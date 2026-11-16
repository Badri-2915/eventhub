package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	"github.com/eventhub/backend/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Pool.Close()

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	if !cfg.IsDev() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.SetTrustedProxies(nil) //nolint:errcheck

	api := r.Group("/api")
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
		return err
	}
	eventsHandler := events.NewHandler(events.NewService(store), cloud)
	eventsHandler.RegisterPublic(api)
	api.GET("/events/:id/stream", hub.Stream(cfg.FrontendOrigin))

	sender, mailVia, err := mail.New(cfg.Mail)
	if err != nil {
		return err
	}
	slog.Info("email delivery", "via", mailVia)

	inv := inventory.NewService(store, hub, cfg.HoldDuration)
	inv.SetNotifier(notify.New(store, sender, cfg.FrontendOrigin))
	ticketsHandler := tickets.NewHandler(tickets.NewService(store, cfg.FrontendOrigin))
	ticketsHandler.RegisterPublic(api)
	attendee := api.Group("", auth.RequireAuth(tokens))
	invHandler := inventory.NewHandler(inv)
	invHandler.Register(attendee, ratelimit.PerUser(rdb, "hold", cfg.HoldRatePerMin, time.Minute))
	go worker.RunSweeper(ctx, inv, 15*time.Second)

	host := api.Group("/host", auth.RequireAuth(tokens), auth.RequireHost(store))
	eventsHandler.RegisterHost(host)
	invHandler.RegisterHost(host)
	ticketsHandler.RegisterHost(host)

	adminGroup := api.Group("/admin", auth.RequireAuth(tokens), auth.RequireAdmin(store, cfg.IsAdmin))
	admin.NewHandler(store, inv).Register(adminGroup)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx) //nolint:errcheck
	}()

	slog.Info("api listening", "port", cfg.Port, "env", cfg.Env)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func errString(err error) string {
	if err != nil {
		return err.Error()
	}
	return "ok"
}
