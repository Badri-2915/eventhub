// Package worker runs background jobs inside the API process.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventhub/backend/internal/inventory"
)

// RunSweeper expires stale holds every interval until ctx is cancelled.
// Correctness doesn't depend on it (holds expire lazily in the hold query);
// it keeps counters tidy and pushes "seat freed" updates to viewers.
// After the first run it only touches the database while recent holds exist.
func RunSweeper(ctx context.Context, inv *inventory.Service, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	first := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !first && !inv.HoldsMayBeLive() {
				continue
			}
			first = false
			n, err := inv.Sweep(ctx)
			if err != nil {
				slog.Error("sweeper failed", "err", err)
			} else if n > 0 {
				slog.Info("expired holds released", "count", n)
			}
		}
	}
}
