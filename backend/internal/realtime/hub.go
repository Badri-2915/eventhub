// Package realtime fans out availability changes over Redis pub/sub so every
// API instance can push them to its SSE clients.
package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type SeatChange struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}

type CategoryChange struct {
	ID        uuid.UUID `json:"id"`
	Available int32     `json:"available"`
}

// Message is what browsers receive. Seats for NUMBERED events, categories for GA.
type Message struct {
	Seats      []SeatChange     `json:"seats,omitempty"`
	Categories []CategoryChange `json:"categories,omitempty"`
}

func (m Message) empty() bool { return len(m.Seats) == 0 && len(m.Categories) == 0 }

// Hub is nil-safe: a nil *Hub publishes nothing (useful in tests).
type Hub struct {
	rdb *redis.Client
}

func NewHub(rdb *redis.Client) *Hub { return &Hub{rdb: rdb} }

func channel(eventID uuid.UUID) string { return "event:" + eventID.String() }

// Publish is best-effort: Postgres is the source of truth and clients also poll.
func (h *Hub) Publish(ctx context.Context, eventID uuid.UUID, msg Message) {
	if h == nil || msg.empty() {
		return
	}
	b, _ := json.Marshal(msg)
	if err := h.rdb.Publish(ctx, channel(eventID), b).Err(); err != nil {
		slog.Warn("realtime publish failed", "event", eventID, "err", err)
	}
}

// Stream returns the SSE handler for GET /api/events/:id/stream. The stream is
// public, so browsers on allowedOrigin may also open it cross-origin (useful
// when the site proxy would buffer long-lived responses).
func (h *Hub) Stream(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
		}
		h.stream(c)
	}
}

func (h *Hub) stream(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	sub := h.rdb.Subscribe(c, channel(eventID))
	defer sub.Close()
	if _, err := sub.Receive(c); err != nil { // wait for subscription confirmation
		c.Status(http.StatusServiceUnavailable)
		return
	}

	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	w.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	msgs := sub.Channel()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			w.Flush()
		case m, ok := <-msgs:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", m.Payload)
			w.Flush()
		}
	}
}
