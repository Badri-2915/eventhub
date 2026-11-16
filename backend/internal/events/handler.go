package events

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/httpx"
)

type Handler struct {
	svc   *Service
	cloud Cloudinary
}

func NewHandler(svc *Service, cloud Cloudinary) *Handler { return &Handler{svc: svc, cloud: cloud} }

// RegisterHost mounts host-only routes; the group must already enforce auth + host.
func (h *Handler) RegisterHost(host *gin.RouterGroup) {
	host.GET("/events", h.listMine)
	host.POST("/events", h.create)
	host.GET("/events/:id", h.getMine)
	host.PUT("/events/:id", h.update)
	host.DELETE("/events/:id", h.deleteDraft)
	host.POST("/events/:id/publish", h.publish)
	host.POST("/events/:id/poster", h.posterSignature)
	host.PUT("/events/:id/poster", h.setPoster)
	host.GET("/events/:id/stats", h.stats)
	host.GET("/events/:id/attendees", h.attendees)
	host.GET("/events/:id/attendees.csv", h.attendeesCSV)
}

func (h *Handler) publish(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	ev, err := h.svc.Publish(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"event": ev})
}

func (h *Handler) listMine(c *gin.Context) {
	evs, err := h.svc.ListForHost(c, auth.UserID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": evs})
}

func (h *Handler) create(c *gin.Context) {
	var in EventInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	ev, err := h.svc.CreateDraft(c, auth.UserID(c), in)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"event": ev})
}

func (h *Handler) getMine(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	ev, err := h.svc.GetForHost(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"event": ev})
}

func (h *Handler) update(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	var in EventInput
	if !httpx.BindJSON(c, &in) {
		return
	}
	ev, err := h.svc.Update(c, auth.UserID(c), id, in)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"event": ev})
}

func pathUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		httpx.Abort(c, httpx.ErrNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) deleteDraft(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteDraft(c, auth.UserID(c), id); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
