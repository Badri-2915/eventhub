package tests

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/inventory"
)

// 1,000 buyers each grab two side-by-side seats in a 200-seat hall at the same
// moment. Many pairs overlap (B4+B5 vs B5+B6), which is exactly where a
// careless design oversells, half-books or deadlocks. Checks:
//   - every request either wins both seats or gets SEATS_TAKEN (all or nothing),
//   - no seat is held twice and every held seat belongs to a 2-seat booking,
//   - zero deadlocks (seats are always locked in id order, so TRY_AGAIN never appears).
func TestManyBuyersManySeatsNoDoubleBooking(t *testing.T) {
	const rows, cols, buyers = 10, 20, 1000
	ev, seats := publishNumbered(t, rows, cols)
	users := make([]uuid.UUID, buyers)
	reqs := make([]inventory.HoldRequest, buyers)
	for i := range users {
		users[i] = newUser(t)
		r, c := rand.IntN(rows), rand.IntN(cols-1)
		a := seats[fmt.Sprintf("%c%d", 'A'+r, c+1)]
		b := seats[fmt.Sprintf("%c%d", 'A'+r, c+2)]
		if rand.IntN(2) == 0 { // the order the buyer clicked must not matter
			a, b = b, a
		}
		reqs[i] = inventory.HoldRequest{EventID: ev.ID, SeatIDs: []uuid.UUID{a, b}}
	}

	var wins, taken, deadlocks atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := invSvc.Hold(context.Background(), users[i], reqs[i])
			switch code(err) {
			case "":
				wins.Add(1)
			case "SEATS_TAKEN":
				taken.Add(1)
			case "TRY_AGAIN":
				deadlocks.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if deadlocks.Load() != 0 {
		t.Fatalf("lock ordering should make deadlocks impossible, got %d", deadlocks.Load())
	}
	if wins.Load()+taken.Load() != buyers {
		t.Fatalf("every buyer gets an answer: wins=%d taken=%d", wins.Load(), taken.Load())
	}
	ctx := context.Background()
	var held, bookingsHolding, badBookings int
	store.Pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT booking_id) FROM seats
		WHERE event_id = $1 AND status = 'HELD'`, ev.ID).Scan(&held, &bookingsHolding)
	store.Pool.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT b.id FROM bookings b LEFT JOIN seats s ON s.booking_id = b.id AND s.status = 'HELD'
		WHERE b.event_id = $1 AND b.status = 'PENDING'
		GROUP BY b.id HAVING count(s.id) <> 2) x`, ev.ID).Scan(&badBookings)
	if held != 2*int(wins.Load()) || bookingsHolding != int(wins.Load()) || badBookings != 0 {
		t.Fatalf("want %d held seats in %d bookings of exactly 2; got %d seats, %d bookings, %d partial",
			2*wins.Load(), wins.Load(), held, bookingsHolding, badBookings)
	}
	t.Logf("%d buyers: %d won a pair, %d told 'taken', 0 deadlocks, 0 double-booked seats", buyers, wins.Load(), taken.Load())
}

// 50 duplicate payment requests for the same booking with the same key, all at
// once, while 50 *other* bookings pay at the same time: one payment each.
func TestConcurrentPaymentsAreExactlyOnce(t *testing.T) {
	ctx := context.Background()
	ev, cat := publishGA(t, 500, 10)
	const bookings, dupes = 50, 50
	type b struct {
		user    uuid.UUID
		booking uuid.UUID
		key     string
	}
	all := make([]b, bookings)
	for i := range all {
		u := newUser(t)
		h, err := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 2}}})
		if err != nil {
			t.Fatal(err)
		}
		all[i] = b{u, h.BookingID, uuid.NewString()}
	}
	var wg sync.WaitGroup
	var failures atomic.Int32
	start := make(chan struct{})
	for _, x := range all {
		for range dupes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if res, err := invSvc.Confirm(ctx, x.user, x.booking, x.key, false); err != nil || res.Status != "CONFIRMED" {
					failures.Add(1)
				}
			}()
		}
	}
	close(start)
	wg.Wait()

	var payments, sold, held int
	store.Pool.QueryRow(ctx, `SELECT count(*) FROM payments p JOIN bookings b ON b.id = p.booking_id WHERE b.event_id = $1`, ev.ID).Scan(&payments)
	store.Pool.QueryRow(ctx, `SELECT sold, held FROM ticket_categories WHERE id = $1`, cat).Scan(&sold, &held)
	if failures.Load() != 0 || payments != bookings || sold != 2*bookings || held != 0 {
		t.Fatalf("%d requests: want 0 failures, %d payments, sold=%d, held=0; got %d, %d, %d, %d",
			bookings*dupes, bookings, 2*bookings, failures.Load(), payments, sold, held)
	}
}
