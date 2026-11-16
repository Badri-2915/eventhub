// Package server builds the HTTP API. cmd/api serves it; the API tests drive
// the same router through httptest, so they exercise every real route,
// middleware and error mapping.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/eventhub/backend/internal/admin"
	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/config"
	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/events"
	"github.com/eventhub/backend/internal/inventory"
	"github.com/eventhub/backend/internal/mail"
	"github.com/eventhub/backend/internal/notify"
	"github.com/eventhub/backend/internal/ratelimit"
	"github.com/eventhub/backend/internal/realtime"
	"github.com/eventhub/backend/internal/tickets"
)

// New wires every route. It returns the inventory service too, because the
// caller runs the hold sweeper with it.
func New(cfg config.Config, store *db.Store, rdb *redis.Client) (*gin.Engine, *inventory.Service, error) {
	if !cfg.IsDev() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.SetTrustedProxies(nil) //nolint:errcheck

	api := r.Group("/api")
	// Liveness for uptime pings: touches neither Postgres nor Redis, so an
	// always-on monitor doesn't keep the free-tier database awake.
	// HEAD too: UptimeRobot's free monitors send HEAD requests.
	ping := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	api.GET("/ping", ping)
	api.HEAD("/ping", ping)
	api.GET("/healthz", func(c *gin.Context) {
		dbErr := store.Pool.Ping(c)
		redisErr := rdb.Ping(c).Err()
		status := http.StatusOK
		if dbErr != nil || redisErr != nil {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"db": errString(dbErr), "redis": errString(redisErr)})
	})

	tokens := auth.NewTokens(cfg.JWTSecret)
	auth.NewHandler(cfg, store, tokens).Register(api)

	hub := realtime.NewHub(rdb)
	cloud, err := events.ParseCloudinaryURL(cfg.CloudinaryURL)
	if err != nil {
		return nil, nil, err
	}
	eventsHandler := events.NewHandler(events.NewService(store), cloud)
	eventsHandler.RegisterPublic(api)
	api.GET("/events/:id/stream", hub.Stream(cfg.FrontendOrigin))

	sender, mailVia, err := mail.New(cfg.Mail)
	if err != nil {
		return nil, nil, err
	}
	slog.Info("email delivery", "via", mailVia)

	inv := inventory.NewService(store, hub, cfg.HoldDuration)
	inv.SetNotifier(notify.New(store, sender, cfg.FrontendOrigin))
	ticketsHandler := tickets.NewHandler(tickets.NewService(store, cfg.FrontendOrigin))
	ticketsHandler.RegisterPublic(api)
	attendee := api.Group("", auth.RequireAuth(tokens))
	invHandler := inventory.NewHandler(inv)
	invHandler.Register(attendee, ratelimit.PerUser(rdb, "hold", cfg.HoldRatePerMin, time.Minute))

	host := api.Group("/host", auth.RequireAuth(tokens), auth.RequireHost(store))
	eventsHandler.RegisterHost(host)
	invHandler.RegisterHost(host)
	ticketsHandler.RegisterHost(host)

	adminGroup := api.Group("/admin", auth.RequireAuth(tokens), auth.RequireAdmin(store, cfg.IsAdmin))
	admin.NewHandler(store, inv).Register(adminGroup)
	return r, inv, nil
}

func errString(err error) string {
	if err != nil {
		return err.Error()
	}
	return "ok"
}
