// Command seed creates a demo host and a handful of published events
// (numbered and general admission) so the app has something to browse.
// It is idempotent: it does nothing if the demo host already has events.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/config"
	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/events"
)

type cat struct {
	name     string
	color    string
	rupees   int32
	capacity int32 // GA only
}

type demo struct {
	title, category, desc, venue, city, address string
	inDays, hour                                int
	durationH                                   int
	mode                                        string
	cats                                        []cat
	rows, cols                                  int
	aisles                                      []int
	preset                                      string // "cinema" | "stage"
}

var demos = []demo{
	{
		title: "Midnight Matinee: Classics Restored", category: "MOVIE", mode: "NUMBERED",
		desc:  "A newly restored print of a beloved classic, screened the way it was meant to be seen. Arrive early for the short introduction by our film club.",
		venue: "Lakeside Cinema", city: "Bengaluru", address: "Ulsoor Lake Road", inDays: 3, hour: 21, durationH: 3,
		cats: []cat{{"Recliner", "#3B7D66", 650, 0}, {"Executive", "#5B7FA6", 350, 0}, {"Regular", "#B98B2F", 220, 0}},
		rows: 10, cols: 18, aisles: []int{3, 14}, preset: "cinema",
	},
	{
		title: "The Quiet Room: Stand-up Night", category: "COMEDY", mode: "GA",
		desc:  "Four comics, one small room, no hecklers. A relaxed evening of observational stand-up with table seating and a short interval.",
		venue: "The Courtyard", city: "Bengaluru", address: "Indiranagar 12th Main", inDays: 5, hour: 20, durationH: 2,
		cats: []cat{{"Front Row", "#A65D57", 999, 30}, {"Regular", "#3B7D66", 499, 120}},
	},
	{
		title: "Monsoon Strings: Live Quartet", category: "CONCERT", mode: "NUMBERED",
		desc:  "An evening of chamber music inspired by the rains: Debussy, Ravel and new Indian compositions for string quartet.",
		venue: "Harbour Hall", city: "Mumbai", address: "Nariman Point", inDays: 9, hour: 19, durationH: 2,
		cats: []cat{{"Gold", "#B98B2F", 1999, 0}, {"Silver", "#5B7FA6", 1199, 0}, {"Classic", "#3B7D66", 699, 0}},
		rows: 8, cols: 14, aisles: []int{7}, preset: "stage",
	},
	{
		title: "Pottery for Beginners", category: "WORKSHOP", mode: "GA",
		desc:  "A calm, hands-on afternoon at the wheel. All clay, tools and firing included. Take home two pieces of your own.",
		venue: "Kiln & Co. Studio", city: "Pune", address: "Koregaon Park Lane 5", inDays: 12, hour: 15, durationH: 3,
		cats: []cat{{"Studio seat", "#6E8B74", 1499, 16}},
	},
	{
		title: "Hamlet, Reimagined", category: "THEATRE", mode: "NUMBERED",
		desc:  "A spare, modern staging of Shakespeare's tragedy with a cast of six. Performed in English with a 15 minute interval.",
		venue: "Old Fort Playhouse", city: "Delhi", address: "Mandi House", inDays: 15, hour: 19, durationH: 3,
		cats: []cat{{"Stalls Front", "#A65D57", 1499, 0}, {"Stalls", "#5B7FA6", 899, 0}, {"Rear", "#6E8B74", 499, 0}},
		rows: 9, cols: 16, aisles: []int{4, 11}, preset: "stage",
	},
}

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Pool.Close()

	host, err := store.UpsertUserByGoogleSub(ctx, sqlc.UpsertUserByGoogleSubParams{
		GoogleSub: "seed:demo-host", Email: "demo-host@eventhub.dev", Name: "EventHub Demo",
	})
	if err != nil {
		return err
	}
	if _, err := store.MakeHost(ctx, host.ID); err != nil {
		return err
	}
	existing, err := store.ListHostEvents(ctx, host.ID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		slog.Info("demo events already seeded", "count", len(existing))
		return nil
	}

	svc := events.NewService(store)
	for _, d := range demos {
		ev, err := svc.CreateDraft(ctx, host.ID, d.input())
		if err != nil {
			return err
		}
		if _, err := svc.Publish(ctx, host.ID, ev.ID); err != nil {
			return err
		}
		slog.Info("seeded", "title", d.title)
	}
	return nil
}

func (d demo) input() events.EventInput {
	day := time.Now().AddDate(0, 0, d.inDays)
	start := time.Date(day.Year(), day.Month(), day.Day(), d.hour, 0, 0, 0, time.Local)
	end := start.Add(time.Duration(d.durationH) * time.Hour)
	in := events.EventInput{
		Title: d.title, Description: d.desc, Category: d.category, VenueName: d.venue,
		City: d.city, Address: d.address, StartsAt: &start, EndsAt: &end, SeatingMode: d.mode,
	}
	ids := make([]uuid.UUID, len(d.cats))
	for i, c := range d.cats {
		ids[i] = uuid.New()
		ci := events.CategoryInput{ID: &ids[i], Name: c.name, Color: c.color, PricePaise: c.rupees * 100}
		if d.mode == "GA" {
			capacity := c.capacity
			ci.Capacity = &capacity
		}
		in.Categories = append(in.Categories, ci)
	}
	if d.mode == "NUMBERED" {
		in.Layout, _ = json.Marshal(d.layout(ids))
	}
	return in
}

// layout paints rows by preset: stage = front rows most valuable, cinema = back rows.
func (d demo) layout(ids []uuid.UUID) events.Layout {
	l := events.Layout{Rows: d.rows, Cols: d.cols}
	aisle := map[int]bool{}
	for _, a := range d.aisles {
		aisle[a] = true
	}
	for r := 0; r < d.rows; r++ {
		var tier int
		if d.preset == "cinema" {
			switch {
			case r >= d.rows-2:
				tier = 0
			case r < 3:
				tier = 2
			default:
				tier = 1
			}
		} else {
			switch {
			case r < 2:
				tier = 0
			case r < 5:
				tier = 1
			default:
				tier = 2
			}
		}
		tier = min(tier, len(ids)-1)
		row := make([]string, d.cols)
		for c := range row {
			if aisle[c] {
				row[c] = events.CellAisle
			} else {
				row[c] = ids[tier].String()
			}
		}
		l.Cells = append(l.Cells, row)
	}
	// A couple of blocked seats for camera / accessibility space.
	l.Cells[d.rows-1][0], l.Cells[d.rows-1][d.cols-1] = events.CellBlocked, events.CellBlocked
	return l
}
