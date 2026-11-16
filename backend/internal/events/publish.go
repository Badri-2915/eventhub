package events

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

func publishErr(msg string) error {
	return httpx.New(http.StatusUnprocessableEntity, "PUBLISH_INVALID", msg)
}

// Publish validates a draft and, in one transaction, materialises its inventory
// (one row per seat, or GA counters already on the categories) and flips it live.
// After this the layout and capacities are read-only.
func (s *Service) Publish(ctx context.Context, hostID, eventID uuid.UUID) (EventDTO, error) {
	var out EventDTO
	err := s.store.InTx(ctx, func(q *sqlc.Queries, tx pgx.Tx) error {
		ev, err := q.LockEventForHost(ctx, sqlc.LockEventForHostParams{ID: eventID, HostID: hostID})
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if ev.Status != sqlc.EventStatusDRAFT {
			return ErrNotDraft
		}
		cats, err := q.ListCategories(ctx, eventID)
		if err != nil {
			return err
		}
		if err := validateForPublish(ev, cats); err != nil {
			return err
		}

		if ev.SeatingMode == sqlc.SeatingModeNUMBERED {
			if err := insertSeatsFromLayout(ctx, q, ev, cats); err != nil {
				return err
			}
		}
		ev, err = q.PublishEvent(ctx, eventID)
		if err != nil {
			return err
		}
		out = toEventDTO(ev, cats)
		return nil
	})
	return out, err
}

func validateForPublish(ev sqlc.Event, cats []sqlc.TicketCategory) error {
	switch {
	case ev.StartsAt == nil:
		return publishErr("Set a start time")
	case !ev.StartsAt.After(time.Now()):
		return publishErr("Start time must be in the future")
	case ev.VenueName == "" || ev.City == "":
		return publishErr("Add the venue name and city")
	case len(cats) == 0:
		return publishErr("Add at least one ticket category")
	}
	if ev.SeatingMode == sqlc.SeatingModeGA {
		var total int32
		for _, c := range cats {
			if c.Capacity == nil || *c.Capacity < 1 {
				return publishErr(fmt.Sprintf("Set a capacity for %s", c.Name))
			}
			total += *c.Capacity
		}
		if total == 0 {
			return publishErr("Total capacity must be at least 1")
		}
	}
	return nil
}

func insertSeatsFromLayout(ctx context.Context, q *sqlc.Queries, ev sqlc.Event, cats []sqlc.TicketCategory) error {
	layout, err := ParseLayout(ev.Layout)
	if err != nil {
		return publishErr(err.Error())
	}
	known := map[uuid.UUID]bool{}
	for _, c := range cats {
		known[c.ID] = true
	}
	seats, err := GenerateSeats(layout, known, cats[0].ID)
	if err != nil {
		return publishErr(err.Error())
	}
	rows := make([]sqlc.InsertSeatsParams, 0, len(seats))
	for _, st := range seats {
		status := sqlc.SeatStatusAVAILABLE
		if st.Blocked {
			status = sqlc.SeatStatusBLOCKED
		}
		rows = append(rows, sqlc.InsertSeatsParams{
			EventID: ev.ID, CategoryID: st.CategoryID, RowLabel: st.RowLabel, SeatNumber: int32(st.SeatNumber),
			Label: st.Label, GridRow: int32(st.GridRow), GridCol: int32(st.GridCol), Status: status,
		})
	}
	_, err = q.InsertSeats(ctx, rows)
	return err
}
