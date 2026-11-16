package inventory

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

var ErrAlreadyCancelled = httpx.New(http.StatusConflict, "EVENT_ALREADY_CANCELLED", "This event is already cancelled")

// CancelEvent cancels a host's own event (see cancelEvent).
func (s *Service) CancelEvent(ctx context.Context, hostID, eventID uuid.UUID) (int, error) {
	return s.cancelEvent(ctx, eventID, func(q *sqlc.Queries) (sqlc.Event, error) {
		return q.LockEventForHost(ctx, sqlc.LockEventForHostParams{ID: eventID, HostID: hostID})
	})
}

// CancelEventAsAdmin cancels any event (platform moderation).
func (s *Service) CancelEventAsAdmin(ctx context.Context, eventID uuid.UUID) (int, error) {
	return s.cancelEvent(ctx, eventID, func(q *sqlc.Queries) (sqlc.Event, error) {
		return q.LockEvent(ctx, eventID)
	})
}

// cancelEvent cancels the event and, in the same transaction, releases every
// hold and refunds every confirmed booking. Returns bookings refunded.
func (s *Service) cancelEvent(ctx context.Context, eventID uuid.UUID, lock func(*sqlc.Queries) (sqlc.Event, error)) (int, error) {
	var pub []changes
	var refunded []uuid.UUID
	err := s.store.InTx(ctx, func(q *sqlc.Queries, _ pgx.Tx) error {
		ev, err := lock(q)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if ev.Status == sqlc.EventStatusCANCELLED {
			return ErrAlreadyCancelled
		}
		// The event row is locked FOR UPDATE; holds take FOR SHARE on it, so
		// no new hold can slip in while we release everything.
		bookings, err := q.ListLiveBookingsForUpdate(ctx, eventID)
		if err != nil {
			return err
		}
		for _, b := range bookings {
			next := sqlc.BookingStatusEXPIRED
			if b.Status == sqlc.BookingStatusCONFIRMED {
				next = sqlc.BookingStatusCANCELLED
				if err := q.RefundPayments(ctx, b.ID); err != nil {
					return err
				}
				refunded = append(refunded, b.ID)
			}
			c, err := s.release(ctx, q, ev, b, next)
			if err != nil {
				return err
			}
			pub = append(pub, c)
		}
		_, err = q.SetEventStatus(ctx, sqlc.SetEventStatusParams{ID: eventID, Status: sqlc.EventStatusCANCELLED})
		return err
	})
	if err != nil {
		return 0, mapPgError(err)
	}
	s.publish(ctx, pub...)
	if len(refunded) > 0 && s.notifier != nil {
		s.notifier.EventCancelled(refunded)
	}
	return len(refunded), nil
}

// RegisterHost mounts host-only inventory routes (group enforces auth + host).
func (h *Handler) RegisterHost(g *gin.RouterGroup) {
	g.POST("/events/:id/cancel", h.cancelEvent)
}

func (h *Handler) cancelEvent(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Abort(c, httpx.ErrNotFound)
		return
	}
	n, err := h.svc.CancelEvent(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"refunded_bookings": n})
}
