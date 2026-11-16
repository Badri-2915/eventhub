package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventhub/backend/internal/config"
	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/server"
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
		// The parse error echoes the URL, password included; keep it out of logs.
		return errors.New("REDIS_URL must look like rediss://default:<password>@<host>:6379")
	}
	// Upstash only accepts TLS; a pasted redis:// URL would fail with a bare "EOF".
	if redisOpts.TLSConfig == nil && strings.HasSuffix(strings.Split(redisOpts.Addr, ":")[0], ".upstash.io") {
		redisOpts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: strings.Split(redisOpts.Addr, ":")[0]}
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	r, inv, err := server.New(cfg, store, rdb)
	if err != nil {
		return err
	}
	go worker.RunSweeper(ctx, inv, 15*time.Second)

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
