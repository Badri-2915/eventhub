package events

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

const (
	maxCategories   = 20
	maxLayoutBytes  = 256 * 1024
	maxTitleLen     = 120
	maxDescLen      = 5000
	defaultPerOrder = 10
	defaultCutoffH  = 24
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// CategoryInput is one ticket category as sent by the host wizard. The client
// may supply the id so layout cells can reference categories before saving.
type CategoryInput struct {
	ID         *uuid.UUID `json:"id"`
	Name       string     `json:"name"`
	Color      string     `json:"color"`
	PricePaise int32      `json:"price_paise"`
	Capacity   *int32     `json:"capacity"` // GA only
}

// EventInput is the full draft state; PUT replaces everything.
type EventInput struct {
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	Category          string          `json:"category"`
	PosterURL         string          `json:"poster_url"`
	MapURL            string          `json:"map_url"`
	VenueName         string          `json:"venue_name"`
	City              string          `json:"city"`
	Address           string          `json:"address"`
	StartsAt          *time.Time      `json:"starts_at"`
	EndsAt            *time.Time      `json:"ends_at"`
	SeatingMode       string          `json:"seating_mode"`
	Layout            json.RawMessage `json:"layout"`
	MaxPerBooking     *int32          `json:"max_per_booking"`
	CancelCutoffHours *int32          `json:"cancel_cutoff_hours"`
	Categories        []CategoryInput `json:"categories"`
}

// normalize applies defaults and checks what must hold even for a draft.
// Stricter publish-time checks (dates in future, full layout) live elsewhere.
func (in *EventInput) normalize() error {
	in.Title = strings.TrimSpace(in.Title)
	in.City = strings.TrimSpace(in.City)
	if in.Title == "" {
		return httpx.BadRequest("Title is required")
	}
	if len(in.Title) > maxTitleLen {
		return httpx.BadRequest(fmt.Sprintf("Title must be at most %d characters", maxTitleLen))
	}
	if len(in.Description) > maxDescLen {
		return httpx.BadRequest(fmt.Sprintf("Description must be at most %d characters", maxDescLen))
	}
	if in.PosterURL = strings.TrimSpace(in.PosterURL); in.PosterURL != "" && !strings.HasPrefix(in.PosterURL, "https://") {
		return httpx.BadRequest("Poster must be an https:// image URL")
	}
	if err := validateMapURL(&in.MapURL); err != nil {
		return err
	}
	if in.Category == "" {
		in.Category = string(sqlc.EventCategoryOTHER)
	}
	if !sqlc.EventCategory(in.Category).Valid() {
		return httpx.BadRequest("Unknown category " + in.Category)
	}
	if in.SeatingMode == "" {
		in.SeatingMode = string(sqlc.SeatingModeGA)
	}
	if !sqlc.SeatingMode(in.SeatingMode).Valid() {
		return httpx.BadRequest("seating_mode must be NUMBERED or GA")
	}
	if in.StartsAt != nil && in.EndsAt != nil && !in.EndsAt.After(*in.StartsAt) {
		return httpx.BadRequest("End time must be after start time")
	}
	if in.MaxPerBooking == nil {
		v := int32(defaultPerOrder)
		in.MaxPerBooking = &v
	}
	if *in.MaxPerBooking < 1 || *in.MaxPerBooking > 50 {
		return httpx.BadRequest("max_per_booking must be between 1 and 50")
	}
	if in.CancelCutoffHours == nil {
		v := int32(defaultCutoffH)
		in.CancelCutoffHours = &v
	}
	if *in.CancelCutoffHours < 0 {
		return httpx.BadRequest("cancel_cutoff_hours cannot be negative")
	}

	// Layout only makes sense for numbered seating.
	if in.SeatingMode == string(sqlc.SeatingModeGA) || isJSONNull(in.Layout) {
		in.Layout = nil
	} else {
		if len(in.Layout) > maxLayoutBytes {
			return httpx.BadRequest("Layout is too large")
		}
		if !json.Valid(in.Layout) {
			return httpx.BadRequest("Layout must be valid JSON")
		}
	}

	if len(in.Categories) > maxCategories {
		return httpx.BadRequest(fmt.Sprintf("At most %d ticket categories", maxCategories))
	}
	seen := map[string]bool{}
	for i := range in.Categories {
		cat := &in.Categories[i]
		cat.Name = strings.TrimSpace(cat.Name)
		if cat.Name == "" {
			return httpx.BadRequest("Every ticket category needs a name")
		}
		key := strings.ToLower(cat.Name)
		if seen[key] {
			return httpx.BadRequest("Duplicate category name " + cat.Name)
		}
		seen[key] = true
		if cat.Color == "" {
			cat.Color = "#14b8a6"
		}
		if !hexColor.MatchString(cat.Color) {
			return httpx.BadRequest("Category colour must look like #aabbcc")
		}
		if cat.PricePaise < 0 {
			return httpx.BadRequest("Price cannot be negative")
		}
		if in.SeatingMode == string(sqlc.SeatingModeNUMBERED) {
			cat.Capacity = nil // seats define capacity in numbered mode
		} else if cat.Capacity != nil && *cat.Capacity < 0 {
			return httpx.BadRequest("Capacity cannot be negative")
		}
		if cat.ID == nil {
			id := uuid.New()
			cat.ID = &id
		}
	}
	return nil
}

func isJSONNull(b json.RawMessage) bool {
	s := strings.TrimSpace(string(b))
	return s == "" || s == "null"
}

// CategoryDTO is a ticket category as returned to clients.
type CategoryDTO struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Color      string    `json:"color"`
	SortOrder  int32     `json:"sort_order"`
	PricePaise int32     `json:"price_paise"`
	Capacity   *int32    `json:"capacity"`
	Sold       int32     `json:"sold"`
	Held       int32     `json:"held"`
}

// EventDTO is an event plus its categories as returned to clients.
type EventDTO struct {
	ID                uuid.UUID       `json:"id"`
	HostID            uuid.UUID       `json:"host_id"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	Category          string          `json:"category"`
	PosterURL         string          `json:"poster_url"`
	MapURL            string          `json:"map_url"`
	VenueName         string          `json:"venue_name"`
	City              string          `json:"city"`
	Address           string          `json:"address"`
	StartsAt          *time.Time      `json:"starts_at"`
	EndsAt            *time.Time      `json:"ends_at"`
	SeatingMode       string          `json:"seating_mode"`
	Status            string          `json:"status"`
	Layout            json.RawMessage `json:"layout"`
	MaxPerBooking     int32           `json:"max_per_booking"`
	CancelCutoffHours int32           `json:"cancel_cutoff_hours"`
	CreatedAt         time.Time       `json:"created_at"`
	PublishedAt       *time.Time      `json:"published_at"`
	Categories        []CategoryDTO   `json:"categories"`
	Summary           *EventSummary   `json:"summary,omitempty"`
}

// EventSummary is the host dashboard's at-a-glance sales figure.
type EventSummary struct {
	Capacity     int32 `json:"capacity"`
	Sold         int32 `json:"sold"`
	RevenuePaise int64 `json:"revenue_paise"`
}

func toEventDTO(e sqlc.Event, cats []sqlc.TicketCategory) EventDTO {
	layout := json.RawMessage(e.Layout)
	if len(layout) == 0 {
		layout = json.RawMessage("null")
	}
	dto := EventDTO{
		ID: e.ID, HostID: e.HostID, Title: e.Title, Description: e.Description,
		Category: string(e.Category), PosterURL: e.PosterUrl, MapURL: e.MapUrl, VenueName: e.VenueName,
		City: e.City, Address: e.Address, StartsAt: e.StartsAt, EndsAt: e.EndsAt,
		SeatingMode: string(e.SeatingMode), Status: string(e.Status), Layout: layout,
		MaxPerBooking: e.MaxPerBooking, CancelCutoffHours: e.CancelCutoffHours,
		CreatedAt: e.CreatedAt, PublishedAt: e.PublishedAt,
		Categories: make([]CategoryDTO, 0, len(cats)),
	}
	for _, c := range cats {
		dto.Categories = append(dto.Categories, CategoryDTO{
			ID: c.ID, Name: c.Name, Color: c.Color, SortOrder: c.SortOrder,
			PricePaise: c.PricePaise, Capacity: c.Capacity, Sold: c.Sold, Held: c.Held,
		})
	}
	return dto
}

// validateMapURL accepts an optional https link to the venue (Google Maps,
// Apple Maps, a maps.app.goo.gl short link, …). https-only keeps it safe to
// render as a clickable link.
func validateMapURL(raw *string) error {
	*raw = strings.TrimSpace(*raw)
	if *raw == "" {
		return nil
	}
	u, err := url.Parse(*raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(*raw) > 500 {
		return httpx.BadRequest("Location link must be a full https:// link, e.g. from Google Maps → Share")
	}
	return nil
}
