package events

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

const pageSize = 24

type EventCard struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	Category         string     `json:"category"`
	PosterURL        string     `json:"poster_url"`
	VenueName        string     `json:"venue_name"`
	City             string     `json:"city"`
	StartsAt         *time.Time `json:"starts_at"`
	SeatingMode      string     `json:"seating_mode"`
	MinPricePaise    int32      `json:"min_price_paise"`
	MaxPricePaise    int32      `json:"max_price_paise"`
	MaxDiscount      int32      `json:"max_discount_percent"`
	TotalTickets     int32      `json:"total_tickets"`
	AvailableTickets int32      `json:"available_tickets"`
}

type PublicCategory struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Color           string    `json:"color"`
	PricePaise      int32     `json:"price_paise"`
	ListPricePaise  int32     `json:"list_price_paise"`
	DiscountPercent int32     `json:"discount_percent"`
	Total           int32     `json:"total"`
	Available       int32     `json:"available"`
}

type PublicEvent struct {
	ID                uuid.UUID        `json:"id"`
	Title             string           `json:"title"`
	Description       string           `json:"description"`
	Category          string           `json:"category"`
	PosterURL         string           `json:"poster_url"`
	MapURL            string           `json:"map_url"`
	VenueName         string           `json:"venue_name"`
	City              string           `json:"city"`
	Address           string           `json:"address"`
	StartsAt          *time.Time       `json:"starts_at"`
	EndsAt            *time.Time       `json:"ends_at"`
	SeatingMode       string           `json:"seating_mode"`
	Status            string           `json:"status"`
	MaxPerBooking     int32            `json:"max_per_booking"`
	CancelCutoffHours int32            `json:"cancel_cutoff_hours"`
	OnSale            bool             `json:"on_sale"`
	Grid              *GridSize        `json:"grid"`
	Categories        []PublicCategory `json:"categories"`
}

type GridSize struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

type Seat struct {
	ID         uuid.UUID `json:"id"`
	CategoryID uuid.UUID `json:"category_id"`
	RowLabel   string    `json:"row_label"`
	SeatNumber int32     `json:"seat_number"`
	Label      string    `json:"label"`
	GridRow    int32     `json:"grid_row"`
	GridCol    int32     `json:"grid_col"`
	Status     string    `json:"status"`
}

func (s *Service) ListPublic(ctx context.Context, q, city, category string, page int) ([]EventCard, error) {
	p := sqlc.ListPublicEventsParams{Lim: pageSize, Off: int32(page * pageSize)}
	if q = strings.TrimSpace(q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	if city != "" {
		p.City = pgtype.Text{String: city, Valid: true}
	}
	if category != "" {
		cat := sqlc.EventCategory(category)
		if !cat.Valid() {
			return nil, httpx.BadRequest("Unknown category")
		}
		p.Category = sqlc.NullEventCategory{EventCategory: cat, Valid: true}
	}
	rows, err := s.store.ListPublicEvents(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]EventCard, 0, len(rows))
	for _, r := range rows {
		out = append(out, EventCard{
			ID: r.ID, Title: r.Title, Category: string(r.Category), PosterURL: r.PosterUrl,
			VenueName: r.VenueName, City: r.City, StartsAt: r.StartsAt, SeatingMode: string(r.SeatingMode),
			MinPricePaise: r.MinPricePaise, MaxPricePaise: r.MaxPricePaise, MaxDiscount: r.MaxDiscountPercent,
			TotalTickets: r.TotalTickets, AvailableTickets: r.AvailableTickets,
		})
	}
	return out, nil
}

func (s *Service) GetPublic(ctx context.Context, id uuid.UUID) (PublicEvent, error) {
	ev, err := s.store.GetEvent(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && ev.Status == sqlc.EventStatusDRAFT) {
		return PublicEvent{}, httpx.ErrNotFound
	}
	if err != nil {
		return PublicEvent{}, err
	}
	cats, err := s.store.ListCategories(ctx, id)
	if err != nil {
		return PublicEvent{}, err
	}

	out := PublicEvent{
		ID: ev.ID, Title: ev.Title, Description: ev.Description, Category: string(ev.Category),
		PosterURL: ev.PosterUrl, MapURL: ev.MapUrl, VenueName: ev.VenueName, City: ev.City, Address: ev.Address,
		StartsAt: ev.StartsAt, EndsAt: ev.EndsAt, SeatingMode: string(ev.SeatingMode), Status: string(ev.Status),
		MaxPerBooking: ev.MaxPerBooking, CancelCutoffHours: ev.CancelCutoffHours,
		OnSale:     ev.Status == sqlc.EventStatusPUBLISHED && ev.StartsAt != nil && ev.StartsAt.After(time.Now()),
		Categories: make([]PublicCategory, 0, len(cats)),
	}

	counts := map[uuid.UUID]sqlc.SeatCountsByCategoryRow{}
	if ev.SeatingMode == sqlc.SeatingModeNUMBERED {
		if l, err := ParseLayout(ev.Layout); err == nil {
			out.Grid = &GridSize{Rows: l.Rows, Cols: l.Cols}
		}
		rows, err := s.store.SeatCountsByCategory(ctx, id)
		if err != nil {
			return PublicEvent{}, err
		}
		for _, r := range rows {
			counts[r.CategoryID] = r
		}
	}
	for _, c := range cats {
		pc := PublicCategory{
			ID: c.ID, Name: c.Name, Color: c.Color,
			PricePaise: c.PricePaise, ListPricePaise: c.ListPricePaise, DiscountPercent: c.DiscountPercent,
		}
		if ev.SeatingMode == sqlc.SeatingModeGA {
			if c.Capacity != nil {
				pc.Total = *c.Capacity
				pc.Available = max(*c.Capacity-c.Sold-c.Held, 0)
			}
		} else {
			pc.Total, pc.Available = counts[c.ID].Total, counts[c.ID].Available
			if pc.Total == 0 {
				continue // category not used on the layout
			}
		}
		out.Categories = append(out.Categories, pc)
	}
	return out, nil
}

func (s *Service) Seats(ctx context.Context, id uuid.UUID) ([]Seat, error) {
	rows, err := s.store.ListSeats(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Seat, 0, len(rows))
	for _, r := range rows {
		out = append(out, Seat{
			ID: r.ID, CategoryID: r.CategoryID, RowLabel: r.RowLabel, SeatNumber: r.SeatNumber,
			Label: r.Label, GridRow: r.GridRow, GridCol: r.GridCol, Status: string(r.Status),
		})
	}
	return out, nil
}

// RegisterPublic mounts unauthenticated browse routes.
func (h *Handler) RegisterPublic(g *gin.RouterGroup) {
	g.GET("/events", h.listPublic)
	g.GET("/events/cities", h.cities)
	g.GET("/events/:id", h.getPublic)
	g.GET("/events/:id/seats", h.seats)
}

func (h *Handler) listPublic(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	evs, err := h.svc.ListPublic(c, c.Query("q"), c.Query("city"), c.Query("category"), max(page, 0))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": evs, "page_size": pageSize})
}

func (h *Handler) cities(c *gin.Context) {
	cities, err := h.svc.store.ListPublicCities(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cities": cities})
}

func (h *Handler) getPublic(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	ev, err := h.svc.GetPublic(c, id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"event": ev})
}

func (h *Handler) seats(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	seats, err := h.svc.Seats(c, id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"seats": seats})
}
