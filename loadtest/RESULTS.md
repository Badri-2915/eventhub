# Load test results

Run on 15 Nov 2026 on a MacBook (Apple Silicon): one API process (`./bin/api`),
PostgreSQL 16 + Redis 7 in Docker (Colima VM, 2 CPU / 4 GB). k6 v2.4.

| Script | Load | Result |
|---|---|---|
| `race_one_seat.js` | 200 users hold the same seat at once | **1 hold won, 199 × 409 `SEATS_TAKEN`**, 0 errors, p95 140 ms |
| `ga_oversell.js` | 300 users × 1 ticket, capacity 100, hold + pay | **exactly 100 held and 100 paid**, `available == 0`, never oversold, p95 301 ms |
| `booking_load.js` | 200 users for 60 s: view → hold → pay, all on **one** GA counter row | **20,976 confirmed bookings (~350/s), ~1,050 req/s**, 0 % failed; hold p95 **193 ms**, confirm p95 **311 ms** |

Notes
- The sustained test deliberately funnels every purchase through a single
  `ticket_categories` row, the worst case for row-lock contention. Sharding the
  counter into N buckets is the next step if one event ever needs more.
- `http_req_duration max` in the raw output includes teardown, which cancels the
  test event and refunds ~21k bookings in a single transaction.
- Correctness is also covered by Go tests (`make test`), including a 500-way seat race.

Reproduce:

```bash
HOLD_RATE_PER_MIN=1000000 make api   # rate limiter off for load tests
make loadtest
```
