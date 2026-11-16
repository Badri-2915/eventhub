package events

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/auth"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/httpx"
)

type CategoryStats struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Color        string    `json:"color"`
	PricePaise   int32     `json:"price_paise"`
	Total        int32     `json:"total"`
	Sold         int32     `json:"sold"`
	Held         int32     `json:"held"`
	Available    int32     `json:"available"`
	RevenuePaise int64     `json:"revenue_paise"`
}

type DaySales struct {
	Day          string `json:"day"` // YYYY-MM-DD
	Bookings     int32  `json:"bookings"`
	Tickets      int32  `json:"tickets"`
	RevenuePaise int64  `json:"revenue_paise"`
}

type EventStats struct {
	Event        EventDTO        `json:"event"`
	Total        int32           `json:"total"`
	Sold         int32           `json:"sold"`
	Held         int32           `json:"held"`
	Available    int32           `json:"available"`
	Bookings     int32           `json:"bookings"`
	RevenuePaise int64           `json:"revenue_paise"`
	Categories   []CategoryStats `json:"categories"`
	SalesByDay   []DaySales      `json:"sales_by_day"`
}

type Attendee struct {
	BookingID   uuid.UUID  `json:"booking_id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	TicketCode  string     `json:"ticket_code"`
	Tickets     int32      `json:"tickets"`
	Items       string     `json:"items"`
	TotalPaise  int32      `json:"total_paise"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	CheckedInAt *time.Time `json:"checked_in_at"`
}

// Stats returns live sold / held / available per category plus revenue and a
// day-by-day sales series. Ownership is enforced by GetForHost.
func (s *Service) Stats(ctx context.Context, hostID, eventID uuid.UUID) (EventStats, error) {
	ev, err := s.GetForHost(ctx, hostID, eventID)
	if err != nil {
		return EventStats{}, err
	}
	out := EventStats{Event: ev, Categories: []CategoryStats{}, SalesByDay: []DaySales{}}

	counts := map[uuid.UUID]sqlc.SeatCountsByCategoryRow{}
	if ev.SeatingMode == string(sqlc.SeatingModeNUMBERED) {
		rows, err := s.store.SeatCountsByCategory(ctx, eventID)
		if err != nil {
			return EventStats{}, err
		}
		for _, r := range rows {
			counts[r.CategoryID] = r
		}
	}
	for _, c := range ev.Categories {
		cs := CategoryStats{ID: c.ID, Name: c.Name, Color: c.Color, PricePaise: c.PricePaise}
		if ev.SeatingMode == string(sqlc.SeatingModeGA) {
			if c.Capacity != nil {
				cs.Total = *c.Capacity
			}
			cs.Sold, cs.Held = c.Sold, c.Held
			cs.Available = max(cs.Total-cs.Sold-cs.Held, 0)
		} else {
			r := counts[c.ID]
			if r.Total == 0 {
				continue // category not used on the layout
			}
			cs.Total, cs.Sold, cs.Held, cs.Available = r.Total, r.Sold, r.Held, r.Available
		}
		cs.RevenuePaise = int64(cs.Sold) * int64(cs.PricePaise)
		out.Total += cs.Total
		out.Sold += cs.Sold
		out.Held += cs.Held
		out.Available += cs.Available
		out.Categories = append(out.Categories, cs)
	}

	rev, err := s.store.EventRevenue(ctx, eventID)
	if err != nil {
		return EventStats{}, err
	}
	out.RevenuePaise, out.Bookings = rev.RevenuePaise, rev.Bookings

	days, err := s.store.SalesByDay(ctx, eventID)
	if err != nil {
		return EventStats{}, err
	}
	for _, d := range days {
		out.SalesByDay = append(out.SalesByDay, DaySales{
			Day: d.Day.Time.Format("2006-01-02"), Bookings: d.Bookings, Tickets: d.Tickets, RevenuePaise: d.RevenuePaise,
		})
	}
	return out, nil
}

func (s *Service) Attendees(ctx context.Context, hostID, eventID uuid.UUID) ([]Attendee, error) {
	if _, err := s.GetForHost(ctx, hostID, eventID); err != nil {
		return nil, err
	}
	rows, err := s.store.ListAttendees(ctx, eventID)
	if err != nil {
		return nil, err
	}
	out := make([]Attendee, 0, len(rows))
	for _, r := range rows {
		out = append(out, Attendee{
			BookingID: r.ID, Name: r.Name, Email: r.Email, TicketCode: r.TicketCode.String,
			Tickets: r.Tickets, Items: r.Items, TotalPaise: r.TotalPaise, ConfirmedAt: r.ConfirmedAt, CheckedInAt: r.CheckedInAt,
		})
	}
	return out, nil
}

// summaries attaches sold / capacity / revenue to each of a host's events.
func (s *Service) summaries(ctx context.Context, hostID uuid.UUID) (map[uuid.UUID]*EventSummary, error) {
	rows, err := s.store.HostEventSummaries(ctx, hostID)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]*EventSummary, len(rows))
	for _, r := range rows {
		m[r.ID] = &EventSummary{Capacity: r.Capacity, Sold: r.Sold, RevenuePaise: r.RevenuePaise}
	}
	return m, nil
}

func (h *Handler) stats(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	st, err := h.svc.Stats(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handler) attendees(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	list, err := h.svc.Attendees(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"attendees": list})
}

func (h *Handler) attendeesCSV(c *gin.Context) {
	id, ok := pathUUID(c, "id")
	if !ok {
		return
	}
	list, err := h.svc.Attendees(c, auth.UserID(c), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="attendees-%s.csv"`, id.String()[:8]))
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"Name", "Email", "Ticket code", "Tickets", "Seats / items", "Paid (INR)", "Booked at", "Checked in at"})
	for _, a := range list {
		booked := fmtTime(a.ConfirmedAt)
		_ = w.Write([]string{
			csvSafe(a.Name), csvSafe(a.Email), a.TicketCode, strconv.Itoa(int(a.Tickets)), csvSafe(a.Items),
			fmt.Sprintf("%.2f", float64(a.TotalPaise)/100), booked, fmtTime(a.CheckedInAt),
		})
	}
	w.Flush()
}

// csvSafe neutralises spreadsheet formula injection (=, +, -, @ at the start).
func csvSafe(s string) string {
	if s != "" && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@') {
		return "'" + s
	}
	return s
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
