// Package notify turns booking events into emails. Sending happens in the
// background after the database transaction commits, so a slow or failing
// email provider never blocks or rolls back a booking.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata" // the distroless image has no zoneinfo; embed it

	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/db"
	"github.com/eventhub/backend/internal/db/sqlc"
	"github.com/eventhub/backend/internal/mail"
)

var ist = mustIST()

func mustIST() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+1800)
	}
	return loc
}

type Notifier struct {
	store   *db.Store
	mail    mail.Sender
	siteURL string
}

func New(store *db.Store, sender mail.Sender, siteURL string) *Notifier {
	return &Notifier{store: store, mail: sender, siteURL: siteURL}
}

// BookingConfirmed emails the attendee their ticket (with QR) and tells the host.
func (n *Notifier) BookingConfirmed(bookingID uuid.UUID) {
	n.async("booking confirmed", func(ctx context.Context) error {
		d, err := n.load(ctx, bookingID)
		if err != nil {
			return err
		}
		if err := n.send(ctx, d.user.Email, "Your tickets: "+d.event.Title, ticketTmpl, d); err != nil {
			return err
		}
		rev, err := n.store.EventRevenue(ctx, d.event.ID)
		if err != nil {
			return err
		}
		d.TotalBookings, d.TotalRevenue = rev.Bookings, inr(rev.RevenuePaise)
		return n.send(ctx, d.host.Email, fmt.Sprintf("New booking: %s × %d", d.event.Title, d.Count), hostTmpl, d)
	})
}

// EventCancelled tells every refunded attendee that the event is off.
func (n *Notifier) EventCancelled(bookingIDs []uuid.UUID) {
	n.async("event cancelled", func(ctx context.Context) error {
		for _, id := range bookingIDs {
			d, err := n.load(ctx, id)
			if err != nil {
				return err
			}
			if err := n.send(ctx, d.user.Email, "Cancelled: "+d.event.Title+" (full refund)", cancelTmpl, d); err != nil {
				slog.Warn("cancellation email failed", "booking", id, "err", err)
			}
		}
		return nil
	})
}

func (n *Notifier) async(what string, fn func(ctx context.Context) error) {
	if n == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := fn(ctx); err != nil {
			slog.Warn("notification failed", "what", what, "err", err)
		}
	}()
}

// ---- data ------------------------------------------------------------------

type emailData struct {
	SiteURL, TicketURL, QRURL, DirectionsURL, EventURL, HostURL string
	Code, Name, When, Venue, Items, Total                       string
	Count                                                       int32
	TotalBookings                                               int32
	TotalRevenue                                                string

	event      sqlc.Event
	user, host sqlc.User
}

func (d emailData) EventTitle() string { return d.event.Title }

func (n *Notifier) load(ctx context.Context, bookingID uuid.UUID) (emailData, error) {
	b, err := n.store.GetBooking(ctx, bookingID)
	if err != nil {
		return emailData{}, err
	}
	ev, err := n.store.GetEvent(ctx, b.EventID)
	if err != nil {
		return emailData{}, err
	}
	user, err := n.store.GetUser(ctx, b.UserID)
	if err != nil {
		return emailData{}, err
	}
	host, err := n.store.GetUser(ctx, ev.HostID)
	if err != nil {
		return emailData{}, err
	}
	items, err := n.store.ListBookingItemsDetailed(ctx, bookingID)
	if err != nil {
		return emailData{}, err
	}

	var parts []string
	var count int32
	for _, it := range items {
		count += it.Quantity
		if it.SeatLabel != "" {
			parts = append(parts, it.SeatLabel+" ("+it.CategoryName+")")
		} else {
			parts = append(parts, fmt.Sprintf("%d × %s", it.Quantity, it.CategoryName))
		}
	}
	when := "Date to be announced"
	if ev.StartsAt != nil {
		when = ev.StartsAt.In(ist).Format("Monday, 2 January 2006 · 3:04 PM")
	}
	directions := ev.MapUrl
	if directions == "" {
		directions = "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(strings.Join(nonEmpty(ev.VenueName, ev.Address, ev.City), ", "))
	}
	return emailData{
		SiteURL:       n.siteURL,
		TicketURL:     fmt.Sprintf("%s/bookings/%s", n.siteURL, b.ID),
		QRURL:         fmt.Sprintf("%s/api/tickets/%s/qr.png", n.siteURL, b.TicketCode.String),
		DirectionsURL: directions,
		EventURL:      fmt.Sprintf("%s/events/%s", n.siteURL, ev.ID),
		HostURL:       fmt.Sprintf("%s/host/events/%s", n.siteURL, ev.ID),
		Code:          b.TicketCode.String,
		Name:          user.Name,
		When:          when,
		Venue:         strings.Join(nonEmpty(ev.VenueName, ev.Address, ev.City), ", "),
		Items:         strings.Join(parts, ", "),
		Total:         inr(int64(b.TotalPaise)),
		Count:         count,
		event:         ev, user: user, host: host,
	}, nil
}

func (n *Notifier) send(ctx context.Context, to, subject string, t *template.Template, d emailData) error {
	var html bytes.Buffer
	if err := t.Execute(&html, d); err != nil {
		return err
	}
	text := fmt.Sprintf("%s\n\n%s\n%s\n%s\nTicket code: %s\n\nView: %s\n", subject, d.event.Title, d.When, d.Venue, d.Code, d.TicketURL)
	return n.mail.Send(ctx, mail.Message{To: to, Subject: subject, HTML: html.String(), Text: text})
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, x)
		}
	}
	return out
}

// inr formats paise with Indian digit grouping: 15961600 → ₹1,59,616.
func inr(paise int64) string {
	r := paise / 100
	s := fmt.Sprint(r)
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		var groups []string
		for len(head) > 2 {
			groups = append([]string{head[len(head)-2:]}, groups...)
			head = head[:len(head)-2]
		}
		if head != "" {
			groups = append([]string{head}, groups...)
		}
		s = strings.Join(groups, ",") + "," + tail
	}
	if p := paise % 100; p != 0 {
		return fmt.Sprintf("₹%s.%02d", s, p)
	}
	return "₹" + s
}
