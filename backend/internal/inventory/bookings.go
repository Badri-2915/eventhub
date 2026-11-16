package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

type BookingEvent struct {
	ID                uuid.UUID  `json:"id"`
	Title             string     `json:"title"`
	Category          string     `json:"category"`
	VenueName         string     `json:"venue_name"`
	City              string     `json:"city"`
	Address           string     `json:"address"`
	PosterURL         string     `json:"poster_url"`
	MapURL            string     `json:"map_url"`
	StartsAt          *time.Time `json:"starts_at"`
	SeatingMode       string     `json:"seating_mode"`
	CancelCutoffHours int32      `json:"cancel_cutoff_hours"`
}

type BookingItem struct {
	CategoryID     uuid.UUID  `json:"category_id"`
	CategoryName   string     `json:"category_name"`
	CategoryColor  string     `json:"category_color"`
	SeatID         *uuid.UUID `json:"seat_id"`
	SeatLabel      string     `json:"seat_label"`
	Quantity       int32      `json:"quantity"`
	UnitPricePaise int32      `json:"unit_price_paise"`
}

type BookingDTO struct {
	ID          uuid.UUID     `json:"id"`
	Status      string        `json:"status"`
	TotalPaise  int32         `json:"total_paise"`
	ExpiresAt   time.Time     `json:"expires_at"`
	TicketCode  *string       `json:"ticket_code"`
	CreatedAt   time.Time     `json:"created_at"`
	ConfirmedAt *time.Time    `json:"confirmed_at"`
	CancelledAt *time.Time    `json:"cancelled_at"`
	CheckedInAt *time.Time    `json:"checked_in_at"`
	CanCancel   bool          `json:"can_cancel"`
	Event       BookingEvent  `json:"event"`
	Items       []BookingItem `json:"items"`
}

func (s *Service) Get(ctx context.Context, userID, bookingID uuid.UUID) (BookingDTO, error) {
	b, err := s.store.GetBooking(ctx, bookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingDTO{}, ErrBookingNotFound
	}
	if err != nil {
		return BookingDTO{}, err
	}
	if b.UserID != userID {
		return BookingDTO{}, httpx.ErrForbidden
	}
	ev, err := s.store.GetEvent(ctx, b.EventID)
	if err != nil {
		return BookingDTO{}, err
	}
	rows, err := s.store.ListBookingItemsDetailed(ctx, b.ID)
	if err != nil {
		return BookingDTO{}, err
	}

	dto := BookingDTO{
		ID: b.ID, Status: string(b.Status), TotalPaise: b.TotalPaise, ExpiresAt: b.ExpiresAt,
		CreatedAt: b.CreatedAt, ConfirmedAt: b.ConfirmedAt, CancelledAt: b.CancelledAt, CheckedInAt: b.CheckedInAt,
		CanCancel: b.Status == sqlc.BookingStatusCONFIRMED && canCancel(ev, time.Now()),
		Event: BookingEvent{
			ID: ev.ID, Title: ev.Title, Category: string(ev.Category), VenueName: ev.VenueName,
			City: ev.City, Address: ev.Address, PosterURL: ev.PosterUrl, MapURL: ev.MapUrl, StartsAt: ev.StartsAt,
			SeatingMode: string(ev.SeatingMode), CancelCutoffHours: ev.CancelCutoffHours,
		},
		Items: make([]BookingItem, 0, len(rows)),
	}
	if b.TicketCode.Valid {
		dto.TicketCode = &b.TicketCode.String
	}
	for _, r := range rows {
		it := BookingItem{
			CategoryID: r.CategoryID, CategoryName: r.CategoryName, CategoryColor: r.CategoryColor,
			SeatLabel: r.SeatLabel, Quantity: r.Quantity, UnitPricePaise: r.UnitPricePaise,
		}
		if r.SeatID.Valid {
			id := r.SeatID.UUID
			it.SeatID = &id
		}
		dto.Items = append(dto.Items, it)
	}
	return dto, nil
}

type BookingSummary struct {
	ID            uuid.UUID  `json:"id"`
	Status        string     `json:"status"`
	TotalPaise    int32      `json:"total_paise"`
	TicketCode    *string    `json:"ticket_code"`
	TicketCount   int32      `json:"ticket_count"`
	ExpiresAt     time.Time  `json:"expires_at"`
	EventID       uuid.UUID  `json:"event_id"`
	EventTitle    string     `json:"event_title"`
	EventVenue    string     `json:"event_venue"`
	EventCity     string     `json:"event_city"`
	EventCategory string     `json:"event_category"`
	EventPoster   string     `json:"event_poster_url"`
	EventStartsAt *time.Time `json:"event_starts_at"`
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID) ([]BookingSummary, error) {
	rows, err := s.store.ListUserBookings(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]BookingSummary, 0, len(rows))
	now := time.Now()
	for _, r := range rows {
		if r.Status == sqlc.BookingStatusPENDING && !r.ExpiresAt.After(now) {
			continue // lapsed hold the sweeper hasn't reached yet
		}
		bs := BookingSummary{
			ID: r.ID, Status: string(r.Status), TotalPaise: r.TotalPaise, TicketCount: r.TicketCount,
			ExpiresAt: r.ExpiresAt, EventID: r.EventID, EventTitle: r.EventTitle, EventVenue: r.EventVenue,
			EventCity: r.EventCity, EventCategory: string(r.EventCategory), EventPoster: r.EventPosterUrl,
			EventStartsAt: r.EventStartsAt,
		}
		if r.TicketCode.Valid {
			bs.TicketCode = &r.TicketCode.String
		}
		out = append(out, bs)
	}
	return out, nil
}
