package tests

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/httpx"
	"github.com/eventhub/backend/internal/inventory"
)

func code(err error) string {
	var e *httpx.Error
	if errors.As(err, &e) {
		return e.Code
	}
	if err != nil {
		return "UNEXPECTED: " + err.Error()
	}
	return ""
}

// 500 different users race for the same seat: exactly one wins.
func TestSeatRace(t *testing.T) {
	ev, seats := publishNumbered(t, 2, 10)
	target := seats["A5"]
	const n = 500
	users := make([]uuid.UUID, n)
	for i := range users {
		users[i] = newUser(t)
	}

	var wins, taken atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := invSvc.Hold(context.Background(), u, inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{target}})
			switch code(err) {
			case "":
				wins.Add(1)
			case "SEATS_TAKEN", "TRY_AGAIN":
				taken.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins.Load() != 1 || taken.Load() != n-1 {
		t.Fatalf("want 1 win / %d rejected, got %d / %d", n-1, wins.Load(), taken.Load())
	}
	var held int
	store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM seats WHERE event_id=$1 AND status='HELD'`, ev.ID).Scan(&held)
	if held != 1 {
		t.Fatalf("want 1 held seat, got %d", held)
	}
}

// Overlapping multi-seat holds: exactly one succeeds and nothing is half-held.
func TestOverlappingHoldsAreAllOrNothing(t *testing.T) {
	for round := 0; round < 30; round++ {
		ev, s := publishNumbered(t, 1, 6)
		x, y := newUser(t), newUser(t)
		reqX := inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{s["A1"], s["A2"], s["A3"]}}
		reqY := inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{s["A3"], s["A4"], s["A5"]}}

		var errX, errY error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, errX = invSvc.Hold(context.Background(), x, reqX) }()
		go func() { defer wg.Done(); _, errY = invSvc.Hold(context.Background(), y, reqY) }()
		wg.Wait()

		if (errX == nil) == (errY == nil) {
			t.Fatalf("round %d: exactly one should win, got x=%v y=%v", round, errX, errY)
		}
		var held int
		store.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM seats WHERE event_id=$1 AND status='HELD'`, ev.ID).Scan(&held)
		if held != 3 {
			t.Fatalf("round %d: want exactly 3 held seats, got %d", round, held)
		}
	}
}

// 300 users × 1 ticket against capacity 100: never oversold.
func TestGANoOversell(t *testing.T) {
	ev, cat := publishGA(t, 100, 10)
	const n = 300
	users := make([]uuid.UUID, n)
	for i := range users {
		users[i] = newUser(t)
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := invSvc.Hold(context.Background(), u, inventory.HoldRequest{
				EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 1}},
			})
			switch code(err) {
			case "":
				wins.Add(1)
			case "NOT_ENOUGH_TICKETS", "TRY_AGAIN":
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	var sold, held, capacity int
	store.Pool.QueryRow(context.Background(),
		`SELECT sold, held, capacity FROM ticket_categories WHERE id=$1`, cat).Scan(&sold, &held, &capacity)
	if wins.Load() != 100 || sold+held != 100 {
		t.Fatalf("want 100 holds and sold+held=100, got wins=%d sold=%d held=%d", wins.Load(), sold, held)
	}
}

// An expired hold frees the seat for someone else; the original confirm then fails.
func TestExpiredHoldIsTakenOver(t *testing.T) {
	ctx := context.Background()
	ev, s := publishNumbered(t, 1, 4)
	a, b := newUser(t), newUser(t)
	holdA, err := invSvc.Hold(ctx, a, inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{s["A1"]}})
	if err != nil {
		t.Fatal(err)
	}
	// Fast-forward A's hold into the past.
	store.Pool.Exec(ctx, `UPDATE bookings SET expires_at = now() - interval '1 second' WHERE id=$1`, holdA.BookingID)
	store.Pool.Exec(ctx, `UPDATE seats SET hold_expires_at = now() - interval '1 second' WHERE booking_id=$1`, holdA.BookingID)

	if _, err := invSvc.Hold(ctx, b, inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{s["A1"]}}); err != nil {
		t.Fatalf("second user should get the expired seat: %v", err)
	}
	if _, err := invSvc.Confirm(ctx, a, holdA.BookingID, uuid.NewString(), false); code(err) != "HOLD_EXPIRED" {
		t.Fatalf("want HOLD_EXPIRED for first user, got %v", err)
	}
}

// Same Idempotency-Key twice → one payment, same result.
func TestConfirmIsIdempotent(t *testing.T) {
	ctx := context.Background()
	ev, cat := publishGA(t, 10, 10)
	u := newUser(t)
	hold, err := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()

	var wg sync.WaitGroup
	results := make([]inventory.BookingDTO, 5)
	errs := make([]error, 5)
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = invSvc.Confirm(ctx, u, hold.BookingID, key, false)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if results[i].Status != "CONFIRMED" || *results[i].TicketCode != *results[0].TicketCode {
			t.Fatalf("attempt %d returned a different result", i)
		}
	}
	var payments, sold, held int
	store.Pool.QueryRow(ctx, `SELECT count(*) FROM payments WHERE booking_id=$1`, hold.BookingID).Scan(&payments)
	store.Pool.QueryRow(ctx, `SELECT sold, held FROM ticket_categories WHERE id=$1`, cat).Scan(&sold, &held)
	if payments != 1 || sold != 2 || held != 0 {
		t.Fatalf("want 1 payment, sold=2, held=0; got %d, %d, %d", payments, sold, held)
	}
}

func TestCannotConfirmSomeoneElsesBooking(t *testing.T) {
	ctx := context.Background()
	ev, s := publishNumbered(t, 1, 2)
	a, b := newUser(t), newUser(t)
	hold, err := invSvc.Hold(ctx, a, inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{s["A1"]}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invSvc.Confirm(ctx, b, hold.BookingID, uuid.NewString(), false); code(err) != "FORBIDDEN" {
		t.Fatalf("want FORBIDDEN, got %v", err)
	}
}

// Failed payment and cancellation both return inventory.
func TestFailureAndCancelReleaseInventory(t *testing.T) {
	ctx := context.Background()
	ev, cat := publishGA(t, 5, 10)
	u := newUser(t)
	avail := func() (sold, held int) {
		store.Pool.QueryRow(ctx, `SELECT sold, held FROM ticket_categories WHERE id=$1`, cat).Scan(&sold, &held)
		return
	}

	h1, _ := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 3}}})
	b1, err := invSvc.Confirm(ctx, u, h1.BookingID, uuid.NewString(), true)
	if err != nil || b1.Status != "FAILED" {
		t.Fatalf("want FAILED booking, got %v %v", b1.Status, err)
	}
	if s, h := avail(); s != 0 || h != 0 {
		t.Fatalf("failed payment must release: sold=%d held=%d", s, h)
	}

	h2, _ := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 3}}})
	if _, err := invSvc.Confirm(ctx, u, h2.BookingID, uuid.NewString(), false); err != nil {
		t.Fatal(err)
	}
	if err := invSvc.Cancel(ctx, u, h2.BookingID); err != nil {
		t.Fatal(err)
	}
	if s, h := avail(); s != 0 || h != 0 {
		t.Fatalf("cancel must release: sold=%d held=%d", s, h)
	}
	var refunded int
	store.Pool.QueryRow(ctx, `SELECT count(*) FROM payments WHERE booking_id=$1 AND status='REFUNDED'`, h2.BookingID).Scan(&refunded)
	if refunded != 1 {
		t.Fatalf("want 1 refunded payment, got %d", refunded)
	}
}

// A user's second hold replaces their first instead of stacking.
func TestNewHoldReplacesPrevious(t *testing.T) {
	ctx := context.Background()
	ev, s := publishNumbered(t, 1, 4)
	u := newUser(t)
	if _, err := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{s["A1"], s["A2"]}}); err != nil {
		t.Fatal(err)
	}
	if _, err := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{s["A3"]}}); err != nil {
		t.Fatal(err)
	}
	var held int
	store.Pool.QueryRow(ctx, `SELECT count(*) FROM seats WHERE event_id=$1 AND status='HELD'`, ev.ID).Scan(&held)
	if held != 1 {
		t.Fatalf("want only the new seat held, got %d", held)
	}
}

// Host cancels while attendees are holding and booking: everything ends up
// released/refunded and no hold survives on the cancelled event.
func TestCancelEventRefundsEveryone(t *testing.T) {
	ctx := context.Background()
	ev, cat := publishGA(t, 50, 10)
	paid := newUser(t)
	h, err := invSvc.Hold(ctx, paid, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invSvc.Confirm(ctx, paid, h.BookingID, uuid.NewString(), false); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = invSvc.Hold(ctx, newUser(t), inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 1}}})
		}()
	}
	if _, err := invSvc.CancelEvent(ctx, ev.HostID, ev.ID); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	var live, sold, held int
	store.Pool.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE event_id=$1 AND status IN ('PENDING','CONFIRMED')`, ev.ID).Scan(&live)
	store.Pool.QueryRow(ctx, `SELECT sold, held FROM ticket_categories WHERE id=$1`, cat).Scan(&sold, &held)
	if live != 0 || sold != 0 || held != 0 {
		t.Fatalf("after cancel want no live bookings and empty counters, got live=%d sold=%d held=%d", live, sold, held)
	}
	if _, err := invSvc.Hold(ctx, newUser(t), inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 1}}}); code(err) != "EVENT_NOT_ON_SALE" {
		t.Fatalf("holds after cancel must fail, got %v", err)
	}
}

func TestDeleteDraftOnly(t *testing.T) {
	ctx := context.Background()
	host := newHost(t)
	draft, err := evSvc.CreateDraft(ctx, host, baseInput("GA"))
	if err != nil {
		t.Fatal(err)
	}
	if err := evSvc.DeleteDraft(ctx, newHost(t), draft.ID); code(err) != "NOT_FOUND" {
		t.Fatalf("another host must not delete it, got %v", err)
	}
	if err := evSvc.DeleteDraft(ctx, host, draft.ID); err != nil {
		t.Fatal(err)
	}
	published, _ := publishGA(t, 10, 5)
	if err := evSvc.DeleteDraft(ctx, published.HostID, published.ID); code(err) != "EVENT_NOT_DRAFT" {
		t.Fatalf("published events can't be deleted, got %v", err)
	}
}
