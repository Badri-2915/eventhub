package inventory

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register mounts attendee routes; the group must already require auth.
// holdLimiter guards POST /holds against hammering.
func (h *Handler) Register(g *gin.RouterGroup, holdLimiter gin.HandlerFunc) {
	g.POST("/holds", holdLimiter, h.hold)
	g.DELETE("/holds/:id", h.release)
	g.GET("/bookings/:id", h.get)
	g.POST("/bookings/:id/confirm", h.confirm)
	g.POST("/bookings/:id/cancel", h.cancel)
	g.GET("/me/bookings", h.listMine)
}

func (h *Handler) hold(c *gin.Context) {
	var req HoldRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Hold(c, auth.UserID(c), req)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

func (h *Handler) release(c *gin.Context) {
	id, ok := bookingID(c)
	if !ok {
		return
	}
	if err := h.svc.ReleaseHold(c, auth.UserID(c), id); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) confirm(c *gin.Context) {
	id, ok := bookingID(c)
	if !ok {
		return
	}
	var body struct {
		Simulate string `json:"simulate"` // "success" (default) | "failure"
	}
	_ = c.ShouldBindJSON(&body)
	b, err := h.svc.Confirm(c, auth.UserID(c), id, c.GetHeader("Idempotency-Key"), body.Simulate == "failure")
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"booking": b})
}

func (h *Handler) cancel(c *gin.Context) {
	id, ok := bookingID(c)
	if !ok {
		return
	}
	if err := h.svc.Cancel(c, auth.UserID(c), id); err != nil {
		httpx.Abort(c, err)
		return
	}
	b, err := h.svc.Get(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"booking": b})
}

func (h *Handler) get(c *gin.Context) {
	id, ok := bookingID(c)
	if !ok {
		return
	}
	b, err := h.svc.Get(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"booking": b})
}

func (h *Handler) listMine(c *gin.Context) {
	list, err := h.svc.ListMine(c, auth.UserID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"bookings": list})
}

func bookingID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Abort(c, ErrBookingNotFound)
		return uuid.Nil, false
	}
	return id, true
}
