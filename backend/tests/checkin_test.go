package tests

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/inventory"
	"github.com/eventhub/backend/internal/mail"
	"github.com/eventhub/backend/internal/notify"
	"github.com/eventhub/backend/internal/tickets"
)

// confirmedTicket books and pays for 2 GA tickets, returning the event and ticket code.
func confirmedTicket(t *testing.T) (hostID uuid.UUID, code string) {
	t.Helper()
	ctx := context.Background()
	ev, cat := publishGA(t, 20, 10)
	u := newUser(t)
	h, err := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := invSvc.Confirm(ctx, u, h.BookingID, uuid.NewString(), false)
	if err != nil {
		t.Fatal(err)
	}
	return ev.HostID, *b.TicketCode
}

// Two door staff scan the same ticket at the same moment: one checks it in,
// the other sees "already checked in"; a third scan later says the same.
func TestCheckInOnlyOnce(t *testing.T) {
	ctx := context.Background()
	svc := tickets.NewService(store, "http://localhost:5173")
	host, ticket := confirmedTicket(t)

	results := make([]string, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := svc.CheckIn(ctx, host, ticket)
			if err != nil {
				t.Error(err)
				return
			}
			results[i] = r.Result
		}()
	}
	wg.Wait()
	counts := map[string]int{}
	for _, r := range results {
		counts[r]++
	}
	if counts["CHECKED_IN"] != 1 || counts["ALREADY_CHECKED_IN"] != 1 {
		t.Fatalf("want one CHECKED_IN and one ALREADY_CHECKED_IN, got %v", results)
	}
	if r, _ := svc.CheckIn(ctx, host, ticket); r.Result != "ALREADY_CHECKED_IN" || r.CheckedInAt == nil {
		t.Fatalf("re-scan must report the first check-in time, got %+v", r)
	}
}

func TestCheckInRejectsOtherHostsAndBadCodes(t *testing.T) {
	ctx := context.Background()
	svc := tickets.NewService(store, "http://localhost:5173")
	_, ticket := confirmedTicket(t)
	if _, err := svc.CheckIn(ctx, newHost(t), ticket); code(err) != "TICKET_NOT_FOUND" {
		t.Fatalf("other host must not check in, got %v", err)
	}
	if _, err := svc.CheckIn(ctx, newHost(t), "EH-NOPE"); code(err) != "TICKET_NOT_FOUND" {
		t.Fatalf("bad code, got %v", err)
	}
}

type fakeMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMail) wait(t *testing.T, n int) []mail.Message {
	t.Helper()
	for range 100 {
		f.mu.Lock()
		if len(f.sent) >= n {
			out := append([]mail.Message(nil), f.sent...)
			f.mu.Unlock()
			return out
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("want %d emails, got %d", n, len(f.sent))
	return nil
}

// Confirming emails the buyer (ticket + QR) and the host; a replayed
// confirm (same Idempotency-Key) sends nothing extra; cancelling emails buyers.
func TestEmailsOnConfirmAndCancel(t *testing.T) {
	ctx := context.Background()
	fm := &fakeMail{}
	inv := inventory.NewService(store, nil, 5*time.Minute)
	inv.SetNotifier(notify.New(store, fm, "https://eventhub.test"))

	ev, cat := publishGA(t, 10, 10)
	u := newUser(t)
	h, err := inv.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	b, err := inv.Confirm(ctx, u, h.BookingID, key, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inv.Confirm(ctx, u, h.BookingID, key, false); err != nil {
		t.Fatal(err)
	}
	msgs := fm.wait(t, 2)
	ticket := msgs[0]
	if !strings.HasPrefix(ticket.Subject, "Your tickets") || !strings.Contains(ticket.HTML, "/api/tickets/"+*b.TicketCode+"/qr.png") {
		t.Fatalf("ticket email missing QR: %q", ticket.Subject)
	}
	if !strings.HasPrefix(msgs[1].Subject, "New booking") {
		t.Fatalf("second email should go to the host, got %q", msgs[1].Subject)
	}

	if _, err := inv.CancelEvent(ctx, ev.HostID, ev.ID); err != nil {
		t.Fatal(err)
	}
	msgs = fm.wait(t, 3)
	if len(msgs) != 3 || !strings.HasPrefix(msgs[2].Subject, "Cancelled") {
		t.Fatalf("want a cancellation email and no duplicates, got %d emails", len(msgs))
	}
}

// The door counter, the early flag, undo after a mistaken scan, and the door
// closing once the event is over.
func TestCheckInUndoCountsAndClosing(t *testing.T) {
	ctx := context.Background()
	svc := tickets.NewService(store, "http://localhost:5173")
	host, ticket := confirmedTicket(t) // 2 tickets, event starts in 72h

	r, err := svc.CheckIn(ctx, host, ticket)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != "CHECKED_IN" || !r.Early || r.Arrived != 2 || r.Expected != 2 {
		t.Fatalf("want early check-in with 2/2 arrived, got %+v", r)
	}

	u, err := svc.UndoCheckIn(ctx, host, ticket)
	if err != nil || u.Result != "UNDONE" || u.Arrived != 0 {
		t.Fatalf("undo: want UNDONE with 0 arrived, got %+v %v", u, err)
	}
	if _, err := svc.UndoCheckIn(ctx, host, ticket); code(err) != "NOT_CHECKED_IN" {
		t.Fatalf("second undo must fail, got %v", err)
	}
	if _, err := svc.UndoCheckIn(ctx, newHost(t), ticket); code(err) != "TICKET_NOT_FOUND" {
		t.Fatalf("another host must not undo, got %v", err)
	}

	svc.Now = func() time.Time { return time.Now().Add(72 * time.Hour) } // the event's start
	if r, err := svc.CheckIn(ctx, host, ticket); err != nil || r.Result != "CHECKED_IN" || r.Early {
		t.Fatalf("on the day: want a normal check-in, got %+v %v", r, err)
	}
	svc.Now = func() time.Time { return time.Now().Add(72*time.Hour + 13*time.Hour) } // no end time: closes 12h after start
	if _, err := svc.CheckIn(ctx, host, ticket); code(err) != "CHECK_IN_CLOSED" {
		t.Fatalf("after the event: want CHECK_IN_CLOSED, got %v", err)
	}
}
