package tests

// API tests: the real router (internal/server) behind httptest, with real
// PostgreSQL and Redis. Each "client" keeps its own cookies, like a browser.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventhub/backend/internal/config"
	"github.com/eventhub/backend/internal/mail"
	"github.com/eventhub/backend/internal/server"
)

const adminEmail = "admin@eventhub.test"

// newAPI starts the full HTTP API. holdsPerMin sets the hold rate limit.
func newAPI(t *testing.T, holdsPerMin int) *httptest.Server {
	t.Helper()
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { rdb.Close() })
	cfg := config.Config{
		Env:            "development", // enables the dev sign-in used by these tests
		JWTSecret:      []byte("test-secret-test-secret-test-secret!"),
		HoldDuration:   5 * time.Minute,
		HoldRatePerMin: holdsPerMin,
		FrontendOrigin: "http://localhost:5173",
		AdminEmails:    []string{adminEmail},
		Mail:           mail.Config{From: "EventHub <tickets@eventhub.test>"},
	}
	r, _, err := server.New(cfg, store, rdb)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

type resp struct {
	status int
	raw    []byte
	body   map[string]any
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL + "/api", http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, headers ...string) resp {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw}
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		json.Unmarshal(raw, &out.body) //nolint:errcheck
	}
	return out
}

func (c *client) login(email, name string) map[string]any {
	c.t.Helper()
	r := c.do("POST", "/auth/dev", map[string]string{"email": email, "name": name})
	if r.status != 200 {
		c.t.Fatalf("dev login: %d %s", r.status, r.raw)
	}
	return r.body["user"].(map[string]any)
}

// get walks a JSON path like "event.categories.0.id".
func (r resp) get(path string) any {
	var cur any = r.body
	for _, p := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[p]
		case []any:
			var i int
			fmt.Sscanf(p, "%d", &i)
			if i >= len(v) {
				return nil
			}
			cur = v[i]
		default:
			return nil
		}
	}
	return cur
}

func (r resp) errCode() string {
	s, _ := r.get("error.code").(string)
	return s
}

func expect(t *testing.T, ok bool, what string, r resp) {
	t.Helper()
	if !ok {
		t.Fatalf("%s: got %d %s", what, r.status, r.raw)
	}
}

func futureStart() string {
	return time.Now().UTC().Add(10*24*time.Hour).Format("2006-01-02") + "T14:00:00Z"
}

// numberedEvent creates and publishes a 2×7 hall: row A has 6 Front seats and
// an aisle; row B has 5 Back seats, an aisle and one blocked cell.
func numberedEvent(t *testing.T, host *client) (eventID string, seats map[string]map[string]any) {
	t.Helper()
	front, back := uuid.NewString(), uuid.NewString()
	cells := [][]string{
		{front, front, front, "a", front, front, front},
		{back, back, back, "a", back, back, "x"},
	}
	r := host.do("POST", "/host/events", map[string]any{
		"title": "API Play " + uuid.NewString()[:6], "category": "THEATRE", "venue_name": "Test Hall",
		"city": "Chennai", "address": "Main Rd", "map_url": "https://maps.app.goo.gl/abc",
		"starts_at": futureStart(), "seating_mode": "NUMBERED",
		"layout": map[string]any{"rows": 2, "cols": 7, "cells": cells},
		"categories": []map[string]any{
			{"id": front, "name": "Front", "color": "#a65d57", "price_paise": 50000},
			{"id": back, "name": "Back", "color": "#5b7fa6", "price_paise": 30000},
		},
	})
	expect(t, r.status == 201, "create numbered draft", r)
	eventID = r.get("event.id").(string)
	p := host.do("POST", "/host/events/"+eventID+"/publish", nil)
	expect(t, p.status == 200 && p.get("event.status") == "PUBLISHED", "publish", p)
	s := host.do("GET", "/events/"+eventID+"/seats", nil)
	seats = map[string]map[string]any{}
	for _, x := range s.get("seats").([]any) {
		m := x.(map[string]any)
		seats[m["label"].(string)] = m
	}
	return eventID, seats
}

func gaEvent(t *testing.T, host *client, capacity int) (eventID, categoryID string) {
	t.Helper()
	r := host.do("POST", "/host/events", map[string]any{
		"title": "API Gig " + uuid.NewString()[:6], "category": "CONCERT", "venue_name": "Club",
		"city": "Chennai", "starts_at": futureStart(), "seating_mode": "GA",
		"categories": []map[string]any{{"name": "Entry", "price_paise": 20000, "capacity": capacity}},
	})
	expect(t, r.status == 201, "create GA draft", r)
	eventID = r.get("event.id").(string)
	categoryID = r.get("event.categories.0.id").(string)
	p := host.do("POST", "/host/events/"+eventID+"/publish", nil)
	expect(t, p.status == 200, "publish GA", p)
	return eventID, categoryID
}

func asHost(t *testing.T, srv *httptest.Server) *client {
	h := newClient(t, srv)
	h.login("host-"+uuid.NewString()[:8]+"@test.dev", "Hema Host")
	r := h.do("POST", "/me/become-host", nil)
	expect(t, r.status == 200 && r.get("user.is_host") == true, "become host", r)
	return h
}

func asUser(t *testing.T, srv *httptest.Server, name string) *client {
	c := newClient(t, srv)
	c.login(strings.ToLower(name)+"-"+uuid.NewString()[:8]+"@test.dev", name)
	return c
}

// ---------------------------------------------------------------------------

func TestAPIHealth(t *testing.T) {
	srv := newAPI(t, 100)
	c := newClient(t, srv)
	if r := c.do("GET", "/ping", nil); r.status != 200 {
		t.Fatalf("GET /ping = %d", r.status)
	}
	if r := c.do("HEAD", "/ping", nil); r.status != 200 {
		t.Fatalf("HEAD /ping = %d (UptimeRobot sends HEAD)", r.status)
	}
	r := c.do("GET", "/healthz", nil)
	expect(t, r.status == 200 && r.get("db") == "ok" && r.get("redis") == "ok", "healthz", r)
}

func TestAPIAuthAndRoles(t *testing.T) {
	srv := newAPI(t, 100)
	anon := newClient(t, srv)
	for _, p := range [][2]string{{"GET", "/me"}, {"POST", "/holds"}, {"GET", "/me/bookings"}, {"GET", "/host/events"}, {"GET", "/admin/overview"}} {
		r := anon.do(p[0], p[1], map[string]any{})
		expect(t, r.status == 401 && r.errCode() == "UNAUTHORIZED", "signed-out "+p[0]+" "+p[1], r)
	}

	user := asUser(t, srv, "Arjun")
	r := user.do("GET", "/host/events", nil)
	expect(t, r.status == 403 && r.errCode() == "NOT_HOST", "attendee opens host dashboard", r)
	r = user.do("GET", "/admin/overview", nil)
	expect(t, r.status == 403, "non-admin opens admin", r)

	admin := newClient(t, srv)
	u := admin.login(adminEmail, "Admin")
	if u["is_admin"] != true {
		t.Fatalf("admin flag from ADMIN_EMAILS missing: %v", u)
	}
	r = admin.do("GET", "/admin/overview", nil)
	expect(t, r.status == 200, "admin overview", r)

	r = user.do("POST", "/auth/logout", nil)
	expect(t, r.status == 204, "logout", r)
	r = user.do("GET", "/me", nil)
	expect(t, r.status == 401, "after logout the session is gone", r)
}

func TestAPIEventValidationAndPublish(t *testing.T) {
	srv := newAPI(t, 100)
	host := asHost(t, srv)
	r := host.do("POST", "/host/events", map[string]any{"title": "bad", "map_url": "javascript:alert(1)"})
	expect(t, r.status == 400, "javascript: map link rejected", r)
	if r.get("error.message") == nil {
		t.Fatalf("errors must carry a human message: %s", r.raw)
	}

	r = host.do("POST", "/host/events", map[string]any{"title": "Draft only"})
	expect(t, r.status == 201, "bare draft is allowed", r)
	draft := r.get("event.id").(string)
	r = host.do("POST", "/host/events/"+draft+"/publish", nil)
	expect(t, r.status == 422 && r.errCode() == "PUBLISH_INVALID", "publishing an incomplete draft", r)

	ev, seats := numberedEvent(t, host)
	if _, ok := seats["A6"]; !ok || seats["B6"]["status"] != "BLOCKED" {
		t.Fatalf("aisle should be skipped in numbering and x blocked: %v", seats)
	}
	if _, ok := seats["A7"]; ok {
		t.Fatal("aisle cell must not become seat A7")
	}
	r = host.do("GET", "/events/"+ev, nil)
	expect(t, r.get("event.map_url") == "https://maps.app.goo.gl/abc", "public event shows the map link", r)

	other := asHost(t, srv)
	r = other.do("GET", "/host/events/"+ev+"/stats", nil)
	expect(t, r.status == 404, "another host cannot see my stats", r)
	r = host.do("DELETE", "/host/events/"+draft, nil)
	expect(t, r.status == 204, "delete a draft", r)
	r = host.do("DELETE", "/host/events/"+ev, nil)
	expect(t, r.status == 409, "a published event cannot be deleted", r)
}

func TestAPIBookingJourney(t *testing.T) {
	srv := newAPI(t, 100)
	host := asHost(t, srv)
	buyer, rival := asUser(t, srv, "Arjun"), asUser(t, srv, "Rahul")
	ev, seats := numberedEvent(t, host)

	h := buyer.do("POST", "/holds", map[string]any{"event_id": ev, "seat_ids": []any{seats["A1"]["id"], seats["A2"]["id"]}})
	expect(t, h.status == 201 && h.get("total_paise") == float64(100000), "hold 2 Front seats = ₹1,000", h)
	booking := h.get("booking_id").(string)

	r := rival.do("POST", "/holds", map[string]any{"event_id": ev, "seat_ids": []any{seats["A2"]["id"]}})
	expect(t, r.status == 409 && r.errCode() == "SEATS_TAKEN", "second buyer for a held seat", r)

	r = buyer.do("POST", "/bookings/"+booking+"/confirm", map[string]any{})
	expect(t, r.status == 400 && r.errCode() == "IDEMPOTENCY_KEY_REQUIRED", "pay without a key", r)

	key := uuid.NewString()
	r = buyer.do("POST", "/bookings/"+booking+"/confirm", map[string]any{}, "Idempotency-Key", key)
	expect(t, r.status == 200 && r.get("booking.status") == "CONFIRMED", "pay", r)
	code := r.get("booking.ticket_code").(string)
	if !strings.HasPrefix(code, "EH-") {
		t.Fatalf("ticket code %q", code)
	}
	r = buyer.do("POST", "/bookings/"+booking+"/confirm", map[string]any{}, "Idempotency-Key", key)
	expect(t, r.status == 200 && r.get("booking.ticket_code") == code, "retrying pay with the same key changes nothing", r)

	h2 := buyer.do("POST", "/holds", map[string]any{"event_id": ev, "seat_ids": []any{seats["B1"]["id"]}})
	expect(t, h2.status == 201, "second hold", h2)
	r = buyer.do("POST", "/bookings/"+h2.get("booking_id").(string)+"/confirm", map[string]any{}, "Idempotency-Key", key)
	expect(t, r.status == 409 && r.errCode() == "IDEMPOTENCY_KEY_REUSED", "a key from another booking", r)

	r = buyer.do("GET", "/me/bookings", nil)
	expect(t, r.status == 200 && strings.Contains(string(r.raw), code), "my tickets lists the booking", r)

	// "Release my seats": the seat is free again for the rival straight away.
	r = buyer.do("DELETE", "/holds/"+h2.get("booking_id").(string), nil)
	expect(t, r.status == 204, "release a hold", r)
	r = rival.do("POST", "/holds", map[string]any{"event_id": ev, "seat_ids": []any{seats["B1"]["id"]}})
	expect(t, r.status == 201, "released seat can be held by someone else", r)

	r = rival.do("GET", "/bookings/"+booking, nil)
	expect(t, r.status == 403, "someone else's ticket", r)
	r = buyer.do("GET", "/tickets/"+code+"/qr.png", nil)
	expect(t, r.status == 200 && bytes.HasPrefix(r.raw, []byte("\x89PNG")), "QR image", r)

	st := host.do("GET", "/host/events/"+ev+"/stats", nil)
	expect(t, st.get("sold") == float64(2) && st.get("revenue_paise") == float64(100000), "host stats", st)
	csv := host.do("GET", "/host/events/"+ev+"/attendees.csv", nil)
	expect(t, csv.status == 200 && bytes.Contains(csv.raw, []byte("Arjun")), "attendee CSV", csv)

	// Door
	r = asHost(t, srv).do("POST", "/host/check-in/"+code, nil)
	expect(t, r.status == 404, "another host's scanner", r)
	r = buyer.do("POST", "/host/check-in/"+code, nil)
	expect(t, r.status == 403, "an attendee cannot check in", r)
	r = host.do("POST", "/host/check-in/"+code, nil)
	expect(t, r.status == 200 && r.get("result") == "CHECKED_IN", "first scan", r)
	r = host.do("POST", "/host/check-in/"+code, nil)
	expect(t, r.get("result") == "ALREADY_CHECKED_IN", "second scan", r)
	r = host.do("POST", "/host/check-in/EH-NOTREAL", nil)
	expect(t, r.status == 404, "fake code", r)

	// Host cancels: everyone refunded, ticket void, no more sales
	r = host.do("POST", "/host/events/"+ev+"/cancel", nil)
	expect(t, r.status == 200 && r.get("refunded_bookings") == float64(1), "host cancels event", r)
	r = buyer.do("GET", "/bookings/"+booking, nil)
	expect(t, r.get("booking.status") == "CANCELLED", "booking cancelled with the event", r)
	r = host.do("POST", "/host/check-in/"+code, nil)
	expect(t, r.status == 410, "cancelled ticket at the door", r)
	r = buyer.do("POST", "/holds", map[string]any{"event_id": ev, "seat_ids": []any{seats["A3"]["id"]}})
	expect(t, r.status == 409 && r.errCode() == "EVENT_NOT_ON_SALE", "no sales after cancel", r)
}

func TestAPIGeneralAdmissionAndRefunds(t *testing.T) {
	srv := newAPI(t, 100)
	host := asHost(t, srv)
	buyer := asUser(t, srv, "Asha")
	ev, cat := gaEvent(t, host, 5)
	item := func(q int) map[string]any {
		return map[string]any{"event_id": ev, "items": []any{map[string]any{"category_id": cat, "qty": q}}}
	}

	h := buyer.do("POST", "/holds", item(3))
	expect(t, h.status == 201, "hold 3 GA tickets", h)
	r := buyer.do("POST", "/bookings/"+h.get("booking_id").(string)+"/confirm", map[string]any{"simulate": "failure"}, "Idempotency-Key", uuid.NewString())
	expect(t, r.get("booking.status") == "FAILED", "failed payment", r)
	e := buyer.do("GET", "/events/"+ev, nil)
	expect(t, e.get("event.categories.0.available") == float64(5), "failed payment gives tickets back", e)

	r = buyer.do("POST", "/holds", item(6))
	expect(t, r.status == 400 || r.status == 409, "more than capacity / per-booking limit", r)

	h = buyer.do("POST", "/holds", item(2))
	b := h.get("booking_id").(string)
	buyer.do("POST", "/bookings/"+b+"/confirm", map[string]any{}, "Idempotency-Key", uuid.NewString())
	r = buyer.do("POST", "/bookings/"+b+"/cancel", nil)
	expect(t, r.status == 200 && r.get("booking.status") == "CANCELLED", "attendee cancels and is refunded", r)
	e = buyer.do("GET", "/events/"+ev, nil)
	expect(t, e.get("event.categories.0.available") == float64(5), "cancelled tickets go back on sale", e)

	admin := newClient(t, srv)
	admin.login(adminEmail, "Admin")
	r = admin.do("POST", "/admin/events/"+ev+"/cancel", nil)
	expect(t, r.status == 200, "admin can cancel any event", r)
}

func TestAPIRateLimit(t *testing.T) {
	srv := newAPI(t, 3) // three holds per user per minute
	host := asHost(t, srv)
	buyer := asUser(t, srv, "Speedy")
	ev, seats := numberedEvent(t, host)
	ids := []string{"A1", "A2", "A3", "A4"}
	for i, label := range ids {
		r := buyer.do("POST", "/holds", map[string]any{"event_id": ev, "seat_ids": []any{seats[label]["id"]}})
		if i < 3 {
			expect(t, r.status == 201, "hold within the limit", r)
		} else {
			expect(t, r.status == 429 && r.errCode() == "RATE_LIMITED", "fourth hold in a minute", r)
		}
	}
	// The limit is per user: someone else is not affected.
	r := asUser(t, srv, "Calm").do("POST", "/holds", map[string]any{"event_id": ev, "seat_ids": []any{seats["B1"]["id"]}})
	expect(t, r.status == 201, "another user is not limited", r)
}
