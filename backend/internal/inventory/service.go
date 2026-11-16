// Package inventory implements holds, confirmation, cancellation and expiry for
// both numbered seats and general-admission counters. Postgres is the single
// source of truth; every check-and-set happens inside one SQL statement.
package inventory

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
	"github.com/eventhub/backend/internal/realtime"
)

type Service struct {
	store    *db.Store
	hub      *realtime.Hub
	holdTTL  time.Duration
	notifier Notifier
}

// Notifier is told about state changes after they commit (emails etc.).
// Implemented by notify.Notifier; nil means "don't notify".
type Notifier interface {
	BookingConfirmed(bookingID uuid.UUID)
	EventCancelled(bookingIDs []uuid.UUID)
}

func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

func NewService(store *db.Store, hub *realtime.Hub, holdTTL time.Duration) *Service {
	return &Service{store: store, hub: hub, holdTTL: holdTTL}
}

type GAItem struct {
	CategoryID uuid.UUID `json:"category_id"`
	Qty        int32     `json:"qty"`
}

// HoldRequest holds either seat ids (NUMBERED) or category quantities (GA).
type HoldRequest struct {
	EventID uuid.UUID   `json:"event_id"`
	SeatIDs []uuid.UUID `json:"seat_ids"`
	Items   []GAItem    `json:"items"`
}

type HoldResult struct {
	BookingID  uuid.UUID `json:"booking_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	TotalPaise int32     `json:"total_paise"`
}

// changes collects realtime updates; they are published only after commit.
type changes struct {
	eventID uuid.UUID
	msg     realtime.Message
}

func (s *Service) publish(ctx context.Context, cs ...changes) {
	for _, c := range cs {
		s.hub.Publish(ctx, c.eventID, c.msg)
	}
}

// Hold atomically reserves the selection for holdTTL. All-or-nothing.
func (s *Service) Hold(ctx context.Context, userID uuid.UUID, req HoldRequest) (HoldResult, error) {
	ev, err := s.store.GetEvent(ctx, req.EventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return HoldResult{}, ErrEventNotFound
	}
	if err != nil {
		return HoldResult{}, err
	}
	if ev.Status != sqlc.EventStatusPUBLISHED || ev.StartsAt == nil || !ev.StartsAt.After(time.Now()) {
		return HoldResult{}, ErrNotOnSale
	}

	seatIDs, items, total, err := normalizeSelection(ev, req)
	if err != nil {
		return HoldResult{}, err
	}
	if total > ev.MaxPerBooking {
		return HoldResult{}, httpx.New(400, "TOO_MANY_TICKETS",
			fmt.Sprintf("You can book at most %d tickets at once", ev.MaxPerBooking))
	}

	var res HoldResult
	var pub []changes
	err = s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		// Re-read under a shared lock: serialises with a host cancelling the event.
		if cur, err := q.GetEventForShare(ctx, ev.ID); err != nil {
			return err
		} else if cur.Status != sqlc.EventStatusPUBLISHED {
			return ErrNotOnSale
		}
		// A new hold replaces the user's previous one for this event.
		prev, err := q.GetPendingBookingForUpdate(ctx, sqlc.GetPendingBookingForUpdateParams{UserID: userID, EventID: ev.ID})
		if err == nil {
			c, err := s.release(ctx, q, ev, prev, sqlc.BookingStatusEXPIRED)
			if err != nil {
				return err
			}
			pub = append(pub, c)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		expires := time.Now().Add(s.holdTTL)
		b, err := q.CreateBooking(ctx, sqlc.CreateBookingParams{UserID: userID, EventID: ev.ID, ExpiresAt: expires})
		if err != nil {
			return err
		}

		var amount int32
		var c changes
		if ev.SeatingMode == sqlc.SeatingModeNUMBERED {
			amount, c, err = holdSeats(ctx, q, ev.ID, b.ID, expires, seatIDs)
		} else {
			amount, c, err = s.holdGA(ctx, q, ev, b.ID, items)
		}
		if err != nil {
			return err
		}
		pub = append(pub, c)
		if err := q.SetBookingTotal(ctx, sqlc.SetBookingTotalParams{ID: b.ID, TotalPaise: amount}); err != nil {
			return err
		}
		res = HoldResult{BookingID: b.ID, ExpiresAt: expires, TotalPaise: amount}
		return nil
	})
	if err != nil {
		return HoldResult{}, mapPgError(err)
	}
	s.publish(ctx, pub...)
	return res, nil
}

func normalizeSelection(ev sqlc.Event, req HoldRequest) ([]uuid.UUID, []GAItem, int32, error) {
	if ev.SeatingMode == sqlc.SeatingModeNUMBERED {
		if len(req.Items) > 0 {
			return nil, nil, 0, ErrWrongMode
		}
		ids := slices.Clone(req.SeatIDs)
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		ids = slices.Compact(ids)
		if len(ids) == 0 {
			return nil, nil, 0, ErrEmptySelection
		}
		return ids, nil, int32(len(ids)), nil
	}

	if len(req.SeatIDs) > 0 {
		return nil, nil, 0, ErrWrongMode
	}
	merged := map[uuid.UUID]int32{}
	var total int32
	for _, it := range req.Items {
		if it.Qty < 0 {
			return nil, nil, 0, httpx.BadRequest("Quantity cannot be negative")
		}
		if it.Qty > 0 {
			merged[it.CategoryID] += it.Qty
			total += it.Qty
		}
	}
	if total == 0 {
		return nil, nil, 0, ErrEmptySelection
	}
	items := make([]GAItem, 0, len(merged))
	for id, qty := range merged {
		items = append(items, GAItem{CategoryID: id, Qty: qty})
	}
	// Fixed lock order across transactions prevents deadlocks.
	slices.SortFunc(items, func(a, b GAItem) int { return slices.Compare(a.CategoryID[:], b.CategoryID[:]) })
	return nil, items, total, nil
}

func holdSeats(ctx context.Context, q *sqlc.Queries, eventID, bookingID uuid.UUID, expires time.Time, seatIDs []uuid.UUID) (int32, changes, error) {
	held, err := q.HoldSeats(ctx, sqlc.HoldSeatsParams{
		BookingID: uuid.NullUUID{UUID: bookingID, Valid: true}, ExpiresAt: &expires,
		EventID: eventID, SeatIds: seatIDs,
	})
	if err != nil {
		return 0, changes{}, err
	}
	if len(held) != len(seatIDs) {
		got := map[uuid.UUID]bool{}
		for _, h := range held {
			got[h.ID] = true
		}
		var taken []uuid.UUID
		for _, id := range seatIDs {
			if !got[id] {
				taken = append(taken, id)
			}
		}
		return 0, changes{}, ErrSeatsTaken.With(map[string]any{"seat_ids": taken})
	}

	cats, err := q.ListCategories(ctx, eventID)
	if err != nil {
		return 0, changes{}, err
	}
	price := map[uuid.UUID]int32{}
	for _, c := range cats {
		price[c.ID] = c.PricePaise
	}

	var amount int32
	msg := realtime.Message{}
	for _, h := range held {
		p := price[h.CategoryID]
		amount += p
		if err := q.InsertBookingItem(ctx, sqlc.InsertBookingItemParams{
			BookingID: bookingID, CategoryID: h.CategoryID,
			SeatID: uuid.NullUUID{UUID: h.ID, Valid: true}, Quantity: 1, UnitPricePaise: p,
		}); err != nil {
			return 0, changes{}, err
		}
		msg.Seats = append(msg.Seats, realtime.SeatChange{ID: h.ID, Status: string(sqlc.SeatStatusHELD)})
	}
	return amount, changes{eventID: eventID, msg: msg}, nil
}

func (s *Service) holdGA(ctx context.Context, q *sqlc.Queries, ev sqlc.Event, bookingID uuid.UUID, items []GAItem) (int32, changes, error) {
	// Lazy expiry keeps the counters honest even if the sweeper is behind.
	expired, err := s.expire(ctx, q, &ev.ID, 50)
	if err != nil {
		return 0, changes{}, err
	}

	var amount int32
	msg := realtime.Message{}
	for _, e := range expired {
		msg.Categories = append(msg.Categories, e.msg.Categories...)
	}
	for _, it := range items {
		row, err := q.HoldGA(ctx, sqlc.HoldGAParams{Qty: it.Qty, ID: it.CategoryID, EventID: ev.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			avail, aerr := q.GACategoryAvailable(ctx, sqlc.GACategoryAvailableParams{ID: it.CategoryID, EventID: ev.ID})
			if errors.Is(aerr, pgx.ErrNoRows) {
				return 0, changes{}, httpx.BadRequest("Unknown ticket category")
			}
			msg := fmt.Sprintf("Only %d tickets left in that category", max(avail, 0))
			if avail <= 0 {
				msg = "That category is sold out"
			}
			return 0, changes{}, httpx.New(ErrNotEnough.Status, ErrNotEnough.Code, msg).
				With(map[string]any{"category_id": it.CategoryID, "available": max(avail, 0)})
		}
		if err != nil {
			return 0, changes{}, err
		}
		amount += row.PricePaise * it.Qty
		if err := q.InsertBookingItem(ctx, sqlc.InsertBookingItemParams{
			BookingID: bookingID, CategoryID: it.CategoryID, Quantity: it.Qty, UnitPricePaise: row.PricePaise,
		}); err != nil {
			return 0, changes{}, err
		}
		msg.Categories = append(msg.Categories, realtime.CategoryChange{ID: row.ID, Available: row.Available})
	}
	return amount, changes{eventID: ev.ID, msg: msg}, nil
}

// release returns a booking's inventory and moves it to newStatus.
// The booking row must already be locked by the caller.
func (s *Service) release(ctx context.Context, q *sqlc.Queries, ev sqlc.Event, b sqlc.Booking, newStatus sqlc.BookingStatus) (changes, error) {
	c := changes{eventID: ev.ID}
	if ev.SeatingMode == sqlc.SeatingModeNUMBERED {
		rows, err := q.ReleaseSeats(ctx, uuid.NullUUID{UUID: b.ID, Valid: true})
		if err != nil {
			return c, err
		}
		for _, r := range rows {
			c.msg.Seats = append(c.msg.Seats, realtime.SeatChange{ID: r.ID, Status: string(r.Status)})
		}
	} else {
		items, err := q.ListBookingItems(ctx, b.ID)
		if err != nil {
			return c, err
		}
		for _, it := range items {
			var avail int32
			var id uuid.UUID
			if b.Status == sqlc.BookingStatusCONFIRMED {
				r, err := q.ReleaseGASold(ctx, sqlc.ReleaseGASoldParams{Qty: it.Quantity, ID: it.CategoryID})
				if err != nil {
					return c, err
				}
				id, avail = r.ID, r.Available
			} else {
				r, err := q.ReleaseGAHeld(ctx, sqlc.ReleaseGAHeldParams{Qty: it.Quantity, ID: it.CategoryID})
				if err != nil {
					return c, err
				}
				id, avail = r.ID, r.Available
			}
			c.msg.Categories = append(c.msg.Categories, realtime.CategoryChange{ID: id, Available: avail})
		}
	}
	_, err := q.SetBookingStatus(ctx, sqlc.SetBookingStatusParams{ID: b.ID, Status: newStatus})
	return c, err
}

// expire claims up to limit expired PENDING bookings (optionally for one event)
// and releases them. SKIP LOCKED means concurrent callers never block each other.
func (s *Service) expire(ctx context.Context, q *sqlc.Queries, eventID *uuid.UUID, limit int32) ([]changes, error) {
	p := sqlc.ClaimExpiredBookingsParams{Lim: limit}
	if eventID != nil {
		p.EventID = uuid.NullUUID{UUID: *eventID, Valid: true}
	}
	bookings, err := q.ClaimExpiredBookings(ctx, p)
	if err != nil {
		return nil, err
	}
	var out []changes
	events := map[uuid.UUID]sqlc.Event{}
	for _, b := range bookings {
		ev, ok := events[b.EventID]
		if !ok {
			if ev, err = q.GetEvent(ctx, b.EventID); err != nil {
				return nil, err
			}
			events[b.EventID] = ev
		}
		c, err := s.release(ctx, q, ev, b, sqlc.BookingStatusEXPIRED)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Sweep expires stale holds in the background. Returns how many it released.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	var pub []changes
	err := s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		var err error
		pub, err = s.expire(ctx, q, nil, 200)
		return err
	})
	if err != nil {
		return 0, err
	}
	s.publish(ctx, pub...)
	return len(pub), nil
}

// Confirm simulates payment. Replaying the same Idempotency-Key returns the
// stored outcome instead of charging again.
func (s *Service) Confirm(ctx context.Context, userID, bookingID uuid.UUID, key string, simulateFailure bool) (BookingDTO, error) {
	if key == "" || len(key) > 200 {
		return BookingDTO{}, ErrMissingKey
	}
	var pub []changes
	confirmedNow := false
	err := s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		b, err := q.GetBookingForUpdate(ctx, bookingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBookingNotFound
		}
		if err != nil {
			return err
		}
		if b.UserID != userID {
			return httpx.ErrForbidden
		}
		// Checked after taking the booking lock, so a concurrent retry with the
		// same key waits for the first attempt and then sees its payment.
		if p, err := q.GetPaymentByKey(ctx, key); err == nil {
			if p.BookingID != bookingID {
				return ErrKeyReused
			}
			return nil // replay: return current booking state below
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		switch {
		case b.Status == sqlc.BookingStatusCONFIRMED:
			return ErrAlreadyConfirmed
		case b.Status != sqlc.BookingStatusPENDING || !b.ExpiresAt.After(time.Now()):
			return ErrHoldExpired
		}
		ev, err := q.GetEvent(ctx, b.EventID)
		if err != nil {
			return err
		}

		if simulateFailure {
			if _, err := q.InsertPayment(ctx, sqlc.InsertPaymentParams{
				BookingID: b.ID, Status: sqlc.PaymentStatusFAILED, AmountPaise: b.TotalPaise, IdempotencyKey: key,
			}); err != nil {
				return err
			}
			c, err := s.release(ctx, q, ev, b, sqlc.BookingStatusFAILED)
			pub = append(pub, c)
			return err
		}

		c := changes{eventID: ev.ID}
		items, err := q.ListBookingItems(ctx, b.ID)
		if err != nil {
			return err
		}
		if ev.SeatingMode == sqlc.SeatingModeNUMBERED {
			booked, err := q.BookHeldSeats(ctx, uuid.NullUUID{UUID: b.ID, Valid: true})
			if err != nil {
				return err
			}
			if len(booked) != len(items) {
				return ErrHoldExpired // a seat was taken over after our hold lapsed
			}
			for _, r := range booked {
				c.msg.Seats = append(c.msg.Seats, realtime.SeatChange{ID: r.ID, Status: string(r.Status)})
			}
		} else {
			for _, it := range items {
				r, err := q.ConfirmGA(ctx, sqlc.ConfirmGAParams{Qty: it.Quantity, ID: it.CategoryID})
				if err != nil {
					return err
				}
				c.msg.Categories = append(c.msg.Categories, realtime.CategoryChange{ID: r.ID, Available: r.Available})
			}
		}
		if _, err := q.InsertPayment(ctx, sqlc.InsertPaymentParams{
			BookingID: b.ID, Status: sqlc.PaymentStatusSUCCESS, AmountPaise: b.TotalPaise, IdempotencyKey: key,
		}); err != nil {
			return err
		}
		if _, err := q.ConfirmBooking(ctx, sqlc.ConfirmBookingParams{
			ID: b.ID, TicketCode: pgtype.Text{String: ticketCode(), Valid: true},
		}); err != nil {
			return err
		}
		pub = append(pub, c)
		confirmedNow = true
		return nil
	})
	if err != nil {
		return BookingDTO{}, mapPgError(err)
	}
	s.publish(ctx, pub...)
	if confirmedNow && s.notifier != nil { // not on idempotent replays
		s.notifier.BookingConfirmed(bookingID)
	}
	return s.Get(ctx, userID, bookingID)
}

// ReleaseHold lets a user abandon their hold early (DELETE /holds/:id).
func (s *Service) ReleaseHold(ctx context.Context, userID, bookingID uuid.UUID) error {
	return s.transition(ctx, userID, bookingID, func(b sqlc.Booking, _ sqlc.Event) (sqlc.BookingStatus, error) {
		if b.Status != sqlc.BookingStatusPENDING {
			return "", ErrNotPending
		}
		return sqlc.BookingStatusEXPIRED, nil
	})
}

// Cancel refunds a confirmed booking if the event's cutoff hasn't passed.
func (s *Service) Cancel(ctx context.Context, userID, bookingID uuid.UUID) error {
	return s.transition(ctx, userID, bookingID, func(b sqlc.Booking, ev sqlc.Event) (sqlc.BookingStatus, error) {
		if b.Status != sqlc.BookingStatusCONFIRMED {
			return "", ErrNotCancellable
		}
		if !canCancel(ev, time.Now()) {
			return "", ErrCancelWindow
		}
		return sqlc.BookingStatusCANCELLED, nil
	})
}

func canCancel(ev sqlc.Event, now time.Time) bool {
	if ev.StartsAt == nil {
		return false
	}
	cutoff := ev.StartsAt.Add(-time.Duration(ev.CancelCutoffHours) * time.Hour)
	return now.Before(cutoff)
}

func (s *Service) transition(ctx context.Context, userID, bookingID uuid.UUID,
	decide func(sqlc.Booking, sqlc.Event) (sqlc.BookingStatus, error)) error {
	var pub changes
	err := s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		b, err := q.GetBookingForUpdate(ctx, bookingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBookingNotFound
		}
		if err != nil {
			return err
		}
		if b.UserID != userID {
			return httpx.ErrForbidden
		}
		ev, err := q.GetEvent(ctx, b.EventID)
		if err != nil {
			return err
		}
		next, err := decide(b, ev)
		if err != nil {
			return err
		}
		if next == sqlc.BookingStatusCANCELLED {
			if err := q.RefundPayments(ctx, b.ID); err != nil {
				return err
			}
		}
		pub, err = s.release(ctx, q, ev, b, next)
		return err
	})
	if err != nil {
		return mapPgError(err)
	}
	s.publish(ctx, pub)
	return nil
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I

func ticketCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return "EH-" + string(b)
}
