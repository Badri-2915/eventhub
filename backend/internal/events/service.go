package events

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

var ErrNotDraft = httpx.New(http.StatusConflict, "EVENT_NOT_DRAFT", "Only draft events can be edited this way")

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

func (s *Service) UpdateDraft(ctx context.Context, hostID, eventID uuid.UUID, in EventInput) (EventDTO, error) {
	if err := in.normalize(); err != nil {
		return EventDTO{}, err
	}
	var out EventDTO
	err := s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		ev, err := q.UpdateDraftEvent(ctx, sqlc.UpdateDraftEventParams{
			ID: eventID, HostID: hostID, Title: in.Title, Description: in.Description,
			Category: sqlc.EventCategory(in.Category), PosterUrl: in.PosterURL,
			VenueName: in.VenueName, City: in.City, Address: in.Address,
			StartsAt: in.StartsAt, EndsAt: in.EndsAt,
			SeatingMode: sqlc.SeatingMode(in.SeatingMode), Layout: in.Layout,
			MaxPerBooking: *in.MaxPerBooking, CancelCutoffHours: *in.CancelCutoffHours, MapUrl: in.MapURL,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish "not yours / missing" from "already published".
			if _, getErr := q.GetEventForHost(ctx, sqlc.GetEventForHostParams{ID: eventID, HostID: hostID}); getErr == nil {
				return ErrNotDraft
			}
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Drafts have no seats or bookings yet, so replacing categories is safe.
		if err := q.DeleteCategories(ctx, eventID); err != nil {
			return err
		}
		cats, err := insertCategories(ctx, q, eventID, in.Categories)
		if err != nil {
			return err
		}
		out = toEventDTO(ev, cats)
		return nil
	})
	return out, err
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
