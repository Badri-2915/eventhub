// Package admin is a small platform-moderation API: totals, every event, and
// the power to cancel any event (with full refunds). Admins are listed by
// email in ADMIN_EMAILS; there is no separate admin password.
package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/httpx"
	"github.com/eventhub/backend/internal/inventory"
)

type Handler struct {
	store *db.Store
	inv   *inventory.Service
}

func NewHandler(store *db.Store, inv *inventory.Service) *Handler {
	return &Handler{store: store, inv: inv}
}

// Register mounts admin routes; the group must enforce auth + admin.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/overview", h.overview)
	g.GET("/events", h.events)
	g.POST("/events/:id/cancel", h.cancel)
}

func (h *Handler) overview(c *gin.Context) {
	o, err := h.store.AdminOverview(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, o)
}

type eventRow struct {
	ID           uuid.UUID  `json:"id"`
	Title        string     `json:"title"`
	Category     string     `json:"category"`
	Status       string     `json:"status"`
	City         string     `json:"city"`
	StartsAt     *time.Time `json:"starts_at"`
	SeatingMode  string     `json:"seating_mode"`
	HostName     string     `json:"host_name"`
	HostEmail    string     `json:"host_email"`
	Capacity     int32      `json:"capacity"`
	Sold         int32      `json:"sold"`
	RevenuePaise int64      `json:"revenue_paise"`
}

func (h *Handler) events(c *gin.Context) {
	rows, err := h.store.AdminListEvents(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	out := make([]eventRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, eventRow{
			ID: r.ID, Title: r.Title, Category: string(r.Category), Status: string(r.Status), City: r.City,
			StartsAt: r.StartsAt, SeatingMode: string(r.SeatingMode), HostName: r.HostName, HostEmail: r.HostEmail,
			Capacity: r.Capacity, Sold: r.Sold, RevenuePaise: r.RevenuePaise,
		})
	}
	c.JSON(http.StatusOK, gin.H{"events": out})
}

func (h *Handler) cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Abort(c, httpx.ErrNotFound)
		return
	}
	n, err := h.inv.CancelEventAsAdmin(c, id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"refunded_bookings": n})
}
