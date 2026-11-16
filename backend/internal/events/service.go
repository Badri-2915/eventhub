package events

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

var (
	ErrNotDraft    = httpx.New(http.StatusConflict, "EVENT_NOT_DRAFT", "Only draft events can be edited this way")
	ErrNotEditable = httpx.New(http.StatusConflict, "EVENT_NOT_EDITABLE", "Cancelled events can't be edited")
)

type Service struct {
	store *db.Store
}

func NewService(store *db.Store) *Service { return &Service{store: store} }

func (s *Service) CreateDraft(ctx context.Context, hostID uuid.UUID, in EventInput) (EventDTO, error) {
	if err := in.normalize(); err != nil {
		return EventDTO{}, err
	}
	var out EventDTO
	err := s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		ev, err := q.CreateEvent(ctx, sqlc.CreateEventParams{
			HostID: hostID, Title: in.Title, Description: in.Description,
			Category: sqlc.EventCategory(in.Category), PosterUrl: in.PosterURL,
			VenueName: in.VenueName, City: in.City, Address: in.Address,
			StartsAt: in.StartsAt, EndsAt: in.EndsAt,
			SeatingMode: sqlc.SeatingMode(in.SeatingMode), Layout: in.Layout,
			MaxPerBooking: *in.MaxPerBooking, CancelCutoffHours: *in.CancelCutoffHours, MapUrl: in.MapURL,
		})
		if err != nil {
			return err
		}
		cats, err := insertCategories(ctx, q, ev.ID, in.Categories)
		if err != nil {
			return err
		}
		out = toEventDTO(ev, cats)
		return nil
	})
	return out, err
}

// Update saves the host's edits. Drafts can change anything; published events
// can change everything except seating mode, layout and the set of ticket types
// (new prices apply to new bookings only). Cancelled events are read-only.
func (s *Service) Update(ctx context.Context, hostID, eventID uuid.UUID, in EventInput) (EventDTO, error) {
	if err := in.normalize(); err != nil {
		return EventDTO{}, err
	}
	var out EventDTO
	err := s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		// FOR UPDATE: holds take FOR SHARE on the event, so they never see a half-applied edit.
		ev, err := q.LockEventForHost(ctx, sqlc.LockEventForHostParams{ID: eventID, HostID: hostID})
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		switch ev.Status {
		case sqlc.EventStatusDRAFT:
			out, err = updateDraft(ctx, q, hostID, eventID, in)
		case sqlc.EventStatusPUBLISHED:
			out, err = updateLive(ctx, q, ev, in)
		default:
			err = ErrNotEditable
		}
		return err
	})
	return out, err
}

func updateDraft(ctx context.Context, q *sqlc.Queries, hostID, eventID uuid.UUID, in EventInput) (EventDTO, error) {
	ev, err := q.UpdateDraftEvent(ctx, sqlc.UpdateDraftEventParams{
		ID: eventID, HostID: hostID, Title: in.Title, Description: in.Description,
		Category: sqlc.EventCategory(in.Category), PosterUrl: in.PosterURL,
		VenueName: in.VenueName, City: in.City, Address: in.Address,
		StartsAt: in.StartsAt, EndsAt: in.EndsAt,
		SeatingMode: sqlc.SeatingMode(in.SeatingMode), Layout: in.Layout,
		MaxPerBooking: *in.MaxPerBooking, CancelCutoffHours: *in.CancelCutoffHours, MapUrl: in.MapURL,
	})
	if err != nil {
		return EventDTO{}, err
	}
	// Drafts have no seats or bookings yet, so replacing categories is safe.
	if err := q.DeleteCategories(ctx, eventID); err != nil {
		return EventDTO{}, err
	}
	cats, err := insertCategories(ctx, q, eventID, in.Categories)
	if err != nil {
		return EventDTO{}, err
	}
	return toEventDTO(ev, cats), nil
}

func liveEditErr(msg string) error {
	return httpx.New(http.StatusConflict, "LIVE_EDIT_INVALID", msg)
}

func updateLive(ctx context.Context, q *sqlc.Queries, ev sqlc.Event, in EventInput) (EventDTO, error) {
	if in.SeatingMode != string(ev.SeatingMode) {
		return EventDTO{}, liveEditErr("Seating can't change after publishing")
	}
	if in.StartsAt == nil || in.VenueName == "" || in.City == "" {
		return EventDTO{}, liveEditErr("A live event needs a start time, venue and city")
	}
	existing, err := q.ListCategories(ctx, ev.ID)
	if err != nil {
		return EventDTO{}, err
	}
	known := map[uuid.UUID]bool{}
	for _, c := range existing {
		known[c.ID] = true
	}
	if len(in.Categories) != len(existing) {
		return EventDTO{}, liveEditErr("Ticket types can't be added or removed after publishing")
	}
	for _, c := range in.Categories {
		if !known[*c.ID] {
			return EventDTO{}, liveEditErr("Ticket types can't be added or removed after publishing")
		}
		if ev.SeatingMode == sqlc.SeatingModeGA && (c.Capacity == nil || *c.Capacity < 1) {
			return EventDTO{}, liveEditErr("Set a capacity for " + c.Name)
		}
	}

	ev, err = q.UpdateLiveEvent(ctx, sqlc.UpdateLiveEventParams{
		ID: ev.ID, HostID: ev.HostID, Title: in.Title, Description: in.Description,
		Category: sqlc.EventCategory(in.Category), PosterUrl: in.PosterURL,
		VenueName: in.VenueName, City: in.City, Address: in.Address,
		StartsAt: in.StartsAt, EndsAt: in.EndsAt,
		MaxPerBooking: *in.MaxPerBooking, CancelCutoffHours: *in.CancelCutoffHours, MapUrl: in.MapURL,
	})
	if err != nil {
		return EventDTO{}, err
	}
	for _, c := range in.Categories {
		var capacity pgtype.Int4
		if c.Capacity != nil {
			capacity = pgtype.Int4{Int32: *c.Capacity, Valid: true}
		}
		_, err := q.UpdateLiveCategory(ctx, sqlc.UpdateLiveCategoryParams{
			ID: *c.ID, EventID: ev.ID, Name: c.Name, Color: c.Color,
			PricePaise: c.PricePaise, ListPricePaise: c.ListPricePaise, DiscountPercent: c.DiscountPercent,
			Capacity: capacity,
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" { // sold + held <= capacity
			return EventDTO{}, liveEditErr(c.Name + ": capacity can't go below the tickets already sold or on hold")
		}
		if err != nil {
			return EventDTO{}, err
		}
	}
	cats, err := q.ListCategories(ctx, ev.ID)
	if err != nil {
		return EventDTO{}, err
	}
	return toEventDTO(ev, cats), nil
}

func (s *Service) GetForHost(ctx context.Context, hostID, eventID uuid.UUID) (EventDTO, error) {
	ev, err := s.store.GetEventForHost(ctx, sqlc.GetEventForHostParams{ID: eventID, HostID: hostID})
	if errors.Is(err, pgx.ErrNoRows) {
		return EventDTO{}, httpx.ErrNotFound
	}
	if err != nil {
		return EventDTO{}, err
	}
	cats, err := s.store.ListCategories(ctx, eventID)
	if err != nil {
		return EventDTO{}, err
	}
	return toEventDTO(ev, cats), nil
}

func (s *Service) ListForHost(ctx context.Context, hostID uuid.UUID) ([]EventDTO, error) {
	evs, err := s.store.ListHostEvents(ctx, hostID)
	if err != nil {
		return nil, err
	}
	sums, err := s.summaries(ctx, hostID)
	if err != nil {
		return nil, err
	}
	out := make([]EventDTO, 0, len(evs))
	for _, ev := range evs {
		cats, err := s.store.ListCategories(ctx, ev.ID)
		if err != nil {
			return nil, err
		}
		dto := toEventDTO(ev, cats)
		dto.Summary = sums[ev.ID]
		out = append(out, dto)
	}
	return out, nil
}

func insertCategories(ctx context.Context, q *sqlc.Queries, eventID uuid.UUID, in []CategoryInput) ([]sqlc.TicketCategory, error) {
	out := make([]sqlc.TicketCategory, 0, len(in))
	for i, c := range in {
		row, err := q.InsertCategory(ctx, sqlc.InsertCategoryParams{
			ID: *c.ID, EventID: eventID, Name: c.Name, Color: c.Color,
			SortOrder: int32(i), PricePaise: c.PricePaise, Capacity: c.Capacity,
			ListPricePaise: c.ListPricePaise, DiscountPercent: c.DiscountPercent,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// DeleteDraft removes an unpublished event. Published events must be cancelled instead.
func (s *Service) DeleteDraft(ctx context.Context, hostID, eventID uuid.UUID) error {
	n, err := s.store.DeleteDraftEvent(ctx, sqlc.DeleteDraftEventParams{ID: eventID, HostID: hostID})
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err := s.store.GetEventForHost(ctx, sqlc.GetEventForHostParams{ID: eventID, HostID: hostID}); err == nil {
			return ErrNotDraft
		}
		return httpx.ErrNotFound
	}
	return nil
}
