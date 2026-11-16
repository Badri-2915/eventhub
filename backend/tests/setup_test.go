// Package tests runs integration and concurrency tests against a real Postgres
// (docker compose). Set TEST_DATABASE_URL to override the local default.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/events"
	"github.com/eventhub/backend/internal/inventory"
)

var (
	store  *db.Store
	evSvc  *events.Service
	invSvc *inventory.Service
)

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		// A separate database so test data never shows up in the dev app.
		if err := ensureDatabase("postgres://eventhub:eventhub@localhost:5432/eventhub?sslmode=disable", "eventhub_test"); err != nil {
			fmt.Println("create test db (is `make up` running?):", err)
			os.Exit(1)
		}
		url = "postgres://eventhub:eventhub@localhost:5432/eventhub_test?sslmode=disable"
	}
	if err := db.Migrate(url); err != nil {
		fmt.Println("migrate:", err)
		os.Exit(1)
	}
	var err error
	store, err = db.Connect(context.Background(), url+"&pool_max_conns=40")
	if err != nil {
		fmt.Println("connect (is `make up` running?):", err)
		os.Exit(1)
	}
	evSvc = events.NewService(store)
	invSvc = inventory.NewService(store, nil, 5*time.Minute)
	code := m.Run()
	store.Pool.Close()
	os.Exit(code)
}

func ensureDatabase(adminURL, name string) error {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	return err
}

func newUser(t testing.TB) uuid.UUID {
	t.Helper()
	u, err := store.UpsertUserByGoogleSub(context.Background(), sqlc.UpsertUserByGoogleSubParams{
		GoogleSub: "test:" + uuid.NewString(), Email: "t@test.dev", Name: "Test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func newHost(t testing.TB) uuid.UUID {
	t.Helper()
	id := newUser(t)
	if _, err := store.MakeHost(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	return id
}

func baseInput(mode string) events.EventInput {
	start := time.Now().Add(72 * time.Hour)
	return events.EventInput{
		Title: "Test " + uuid.NewString()[:8], Category: "COMEDY", VenueName: "Hall", City: "Testville",
		StartsAt: &start, SeatingMode: mode,
	}
}

// publishGA creates and publishes a GA event with one category of the given capacity.
func publishGA(t testing.TB, capacity int32, maxPer int32) (events.EventDTO, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	host := newHost(t)
	in := baseInput("GA")
	in.MaxPerBooking = &maxPer
	in.Categories = []events.CategoryInput{{Name: "Regular", PricePaise: 49900, Capacity: &capacity}}
	ev, err := evSvc.CreateDraft(ctx, host, in)
	if err != nil {
		t.Fatal(err)
	}
	if ev, err = evSvc.Publish(ctx, host, ev.ID); err != nil {
		t.Fatal(err)
	}
	return ev, ev.Categories[0].ID
}

// publishNumbered creates a rows×cols hall, all one category, and returns seat ids by label.
func publishNumbered(t testing.TB, rows, cols int) (events.EventDTO, map[string]uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	host := newHost(t)
	cat := uuid.New()
	layout := events.Layout{Rows: rows, Cols: cols}
	for r := 0; r < rows; r++ {
		row := make([]string, cols)
		for c := range row {
			row[c] = cat.String()
		}
		layout.Cells = append(layout.Cells, row)
	}
	raw, _ := json.Marshal(layout)
	in := baseInput("NUMBERED")
	in.Layout = raw
	in.Categories = []events.CategoryInput{{ID: &cat, Name: "Standard", PricePaise: 29900}}
	ev, err := evSvc.CreateDraft(ctx, host, in)
	if err != nil {
		t.Fatal(err)
	}
	if ev, err = evSvc.Publish(ctx, host, ev.ID); err != nil {
		t.Fatal(err)
	}
	seats, err := evSvc.Seats(ctx, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]uuid.UUID{}
	for _, s := range seats {
		byLabel[s.Label] = s.ID
	}
	return ev, byLabel
}
