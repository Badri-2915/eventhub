# EventHub — End-to-End Build Plan

A configurable event ticketing platform. Hosts create events and design their own
seating; attendees browse, hold, and book tickets with first-come-first-served
guarantees and live availability.

Two inventory modes, chosen by the host per event:

| Mode | Used for | What the customer picks | Inventory model |
|---|---|---|---|
| **NUMBERED** | Movies, stage shows, theatre | Specific seats (A5, A6) on a layout | One row per seat in `seats` |
| **GENERAL ADMISSION (GA)** | Comedy clubs, concerts, workshops | Category + quantity ("2 × Regular") | One counter row per category: `capacity / sold / held` |

> Rule: an event has **either** seats **or** GA counters, never both.

---

## 1. Tech stack (decided)

| Layer            | Choice                                                                                                                  | Why                                                            |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| Backend          | **Go 1.23 + Gin**                                                                                                       | Fast, simple routing, goroutines for worker + SSE              |
| DB access        | **pgx v5 + sqlc**                                                                                                       | Type-safe SQL, full control over transactions/locks            |
| Migrations       | **golang-migrate**                                                                                                      | Plain SQL up/down files                                        |
| Database         | **PostgreSQL 16**                                                                                                       | Single source of truth, row locks, `SKIP LOCKED`               |
| Cache / realtime | **Redis 7** (go-redis)                                                                                                  | Pub/sub for live seat updates, rate limiting, event-list cache |
| Auth             | **Google Identity Services** (frontend) + `google.golang.org/api/idtoken` (backend verify) → own JWT in httpOnly cookie | Never trust user info from the frontend                        |
| Frontend         | **React + Vite + TypeScript + Tailwind + TanStack Query + React Router**                                                | Fast dev, good caching of server state                         |
| Images           | **Cloudinary** (signed upload from backend)                                                                             | Free tier, no S3 setup                                         |
| Local dev        | **Docker Compose** (postgres, redis, api)                                                                               | One command up                                                 |
| Deploy           | API → **Render** (Docker), DB → **Neon**, Redis → **Upstash**, Web → **Vercel** with `/api/*` rewrite to Render         | Same-origin → httpOnly cookies work without CORS pain          |
| Testing          | Go `testing` + real Postgres (compose), **k6** load test                                                                | Prove zero double booking                                      |

---

## 2. Users & roles

- Every Google-signed-in user is an **attendee**.
- A user clicks **"Become a host"** once → `users.is_host = true` → host dashboard unlocked.
- Host routes check `is_host` **and** `events.host_id = current_user` (ownership).

---

## 3. Host flow — create & configure an event

A 4-step wizard. Event stays `DRAFT` until published.

### Step 1 — Details
Title, category (`MOVIE | COMEDY | CONCERT | THEATRE | WORKSHOP | OTHER`),
description, poster upload, venue name, city, address, start & end time,
max tickets per booking (default 10), cancellation cutoff (hours before start, default 24).

### Step 2 — Seating mode
`NUMBERED` or `GENERAL_ADMISSION`.

### Step 3a — Ticket categories (both modes)
Host adds categories: name, price, colour.
Example: VIP ₹1,499 (gold) · Premium ₹999 (purple) · Regular ₹499 (teal).

### Step 3b — GA: capacities
| Category | Capacity | Price |
|---|---|---|
| VIP | 20 | ₹1,499 |
| Premium | 30 | ₹999 |
| Regular | 50 | ₹499 |
| **Total** | **100** | |

Customer sees: **"Regular — 50 total · 23 sold · 2 on hold · 25 available"**.
Event card sees: **"100 tickets · 45 booked · 55 available"**.

### Step 3c — NUMBERED: layout designer (grid editor)

The host designs the hall as a grid. **The screen/stage is always drawn at the top**,
row A is the row nearest to it.

**Setup**
1. Enter rows (e.g. 10) × columns (e.g. 16) → blank grid appears, all cells = seat, no category.
2. Pick a **preset** (optional, everything editable afterwards):
   - **Stage show** — front rows most valuable: A–B VIP, C–E Premium, rest Regular.
   - **Cinema** — back rows most valuable: last 2 rows Premium/Recliner, middle Executive, front 3 rows Regular.
   - **Blank** — host paints everything.

**Tools (toolbar)**
| Tool | Effect |
|---|---|
| Paint category (VIP / Premium / Regular …) | Click or drag across cells to assign category |
| Row click (click row letter "C") | Assign the whole row to the selected category |
| Aisle | Turns cell into an empty gap (walkway); not a seat, not numbered |
| Blocked | Seat exists but is never sold (staff, camera, wheelchair space) |
| Eraser | Back to unassigned seat |

**Labelling rules (automatic)**
- Rows lettered A, B, C … from the screen side; empty rows (all aisle) are skipped in lettering.
- Seat numbers count left → right, **aisles skipped**: `A1 A2 A3 [aisle] A4 A5`.

**Live summary panel** next to the grid:
`VIP 32 seats × ₹1,499 · Premium 48 × ₹999 · Regular 64 × ₹499 · Blocked 4 · Total sellable 144 · Max revenue ₹1,59,616`.

**Preview** button renders exactly what customers will see.

**Validation before publish**
- Every seat cell has a category (no unassigned seats).
- At least 1 sellable seat; max 50 rows × 50 cols.
- Every category used has a price > 0 (or explicit free).

The grid is saved as JSON in `events.layout` while in draft (autosave).

### Step 4 — Review & publish
On **Publish**, in one transaction:
- NUMBERED → generate one `seats` row per seat cell (status `AVAILABLE`, or `BLOCKED`).
- GA → `ticket_categories.capacity` already set; `sold = held = 0`.
- `events.status = PUBLISHED`, layout + capacities become **read-only**
  (only description/poster editable afterwards).

---

## 4. Customer flow

1. **Browse** `/` — published upcoming events, search by text, filter by city/category/date.
2. **Event page** — poster, details, price range, availability badge.
3. **Select**
   - NUMBERED: seat map with legend
     🟩 Available · 🟦 Selected · 🟧 Held by someone · ⬜ Booked · ✖ Blocked.
     Category colour shown as seat border; price on hover.
   - GA: category cards with `−  2  +` stepper and live "X available".
4. **Hold** — click "Proceed" → backend atomically holds seats/qty for **5 minutes** → checkout page with countdown.
5. **Pay (simulated)** — "Pay ₹998" (success) / "Simulate failure" button.
6. **Confirmed** — booking page with ticket code (and QR as stretch goal).
7. **My bookings** — upcoming / past / cancelled; cancel button if before cutoff.

Live updates: while a user is on the event page, seats change colour in real time
when others hold/book (SSE).

---

## 5. Data model

```sql
users (
  id uuid PK, google_sub text UNIQUE, email text, name text, avatar_url text,
  is_host bool DEFAULT false, created_at timestamptz
)

events (
  id uuid PK, host_id uuid → users,
  title, description, category, poster_url,
  venue_name, city, address,
  starts_at timestamptz, ends_at timestamptz,
  seating_mode  ENUM('NUMBERED','GA'),
  status        ENUM('DRAFT','PUBLISHED','CANCELLED','COMPLETED'),
  layout jsonb NULL,                -- grid editor state (NUMBERED only)
  max_per_booking int DEFAULT 10,
  cancel_cutoff_hours int DEFAULT 24,
  created_at, published_at
)

ticket_categories (
  id uuid PK, event_id → events,
  name, color, sort_order,
  price_paise int NOT NULL,         -- money in integer paise, never float
  capacity int NULL,                -- GA only
  sold int DEFAULT 0,               -- GA only
  held int DEFAULT 0,               -- GA only
  CHECK (capacity IS NULL OR sold + held <= capacity)   -- DB-level oversell guard
)

seats (                             -- NUMBERED only
  id uuid PK, event_id, category_id,
  row_label text, seat_number int, label text,   -- 'C', 7, 'C7'
  grid_row int, grid_col int,                    -- position for rendering
  status ENUM('AVAILABLE','HELD','BOOKED','BLOCKED'),
  booking_id uuid NULL, hold_expires_at timestamptz NULL,
  UNIQUE (event_id, label)
)
INDEX seats(event_id, status)

bookings (
  id uuid PK, user_id, event_id,
  status ENUM('PENDING','CONFIRMED','CANCELLED','EXPIRED','FAILED'),
  total_paise int, expires_at timestamptz,
  ticket_code text UNIQUE,          -- set on confirm
  created_at, confirmed_at, cancelled_at
)
-- at most one PENDING booking per user per event:
UNIQUE INDEX ON bookings(user_id, event_id) WHERE status = 'PENDING'

booking_items (
  id, booking_id, category_id,
  seat_id uuid NULL,                -- NUMBERED: one item per seat
  quantity int,                     -- GA: qty of that category
  unit_price_paise int              -- price snapshot at booking time
)

payments (
  id, booking_id, provider 'SIMULATED', status ENUM('SUCCESS','FAILED','REFUNDED'),
  amount_paise, idempotency_key text UNIQUE, created_at
)
```

---

## 6. Core concurrency logic (the heart of the project)

### 6.1 Hold — NUMBERED (all-or-nothing, first wins)
```sql
BEGIN;
INSERT INTO bookings (... status='PENDING', expires_at = now() + '5 min') RETURNING id;

UPDATE seats
SET status='HELD', booking_id=$booking, hold_expires_at = now() + interval '5 minutes'
WHERE event_id = $event AND id = ANY($seat_ids)
  AND ( status = 'AVAILABLE'
        OR (status = 'HELD' AND hold_expires_at < now()) )   -- expired hold = free
RETURNING id;

-- rows returned != len(seat_ids)  →  ROLLBACK, HTTP 409 "Some seats were just taken"
INSERT booking_items ...; COMMIT;
```
Why it's correct: the `UPDATE ... WHERE status='AVAILABLE'` is a single atomic
check-and-set; Postgres row locks make the second concurrent transaction re-check
the row after the first commits, find it `HELD`, and match 0 rows.

### 6.2 Hold — GA (counter, no oversell)
```sql
BEGIN;
-- 1. release this category's expired holds first (keeps counter honest)
-- 2. conditional increment
UPDATE ticket_categories
SET held = held + $qty
WHERE id = $category AND capacity - sold - held >= $qty
RETURNING capacity - sold - held AS remaining;
-- 0 rows → ROLLBACK, 409 "Only N tickets left"
INSERT bookings / booking_items ...; COMMIT;
```
Plus the `CHECK (sold + held <= capacity)` constraint as a second safety net.

### 6.3 Confirm (simulated payment, idempotent)
`POST /bookings/:id/confirm` with header `Idempotency-Key`.
```
BEGIN
  if payments has this idempotency_key → return the stored result (no double charge)
  SELECT booking FOR UPDATE
  require status='PENDING' AND expires_at > now() AND user matches   else 410 Gone
  NUMBERED: UPDATE seats SET status='BOOKED' WHERE booking_id=$b AND status='HELD'
            (count must equal items, else hold was taken over → fail)
  GA:       UPDATE ticket_categories SET held=held-qty, sold=sold+qty
  INSERT payments(SUCCESS); booking → CONFIRMED, ticket_code generated
COMMIT → publish update
```
Simulated failure → payment FAILED, booking FAILED, inventory released.

### 6.4 Expiry
- **Lazy (correctness):** NUMBERED hold query treats expired holds as available — works even if the worker is down.
- **Sweeper (tidiness):** goroutine every 15 s:
  `UPDATE bookings SET status='EXPIRED' WHERE status='PENDING' AND expires_at < now() ... FOR UPDATE SKIP LOCKED`
  → release seats / decrement `held` → publish updates.

### 6.5 Cancel
Allowed if `CONFIRMED` and `now() < starts_at - cutoff`.
Seats → `AVAILABLE` / GA `sold -= qty`; payment → `REFUNDED` (simulated).

### 6.6 Abuse limits
- Max `max_per_booking` tickets per hold.
- One `PENDING` booking per user per event (partial unique index).
- Redis token-bucket rate limit on `POST /holds` (e.g. 10/min/user).

---

## 7. Real-time availability

- Every state change publishes to Redis channel `event:{id}`:
  `{"type":"seats","changes":[{"id":"…","status":"HELD"}]}` or
  `{"type":"ga","category_id":"…","available":25}`.
- `GET /events/:id/stream` (SSE) subscribes and forwards to the browser.
- Frontend `useEventStream` hook patches the TanStack Query cache → seats recolour instantly.
- Redis pub/sub means it still works with multiple API instances.
- Fallback: refetch availability every 20 s if SSE disconnects.

---

## 8. API

```
Auth
  POST   /api/auth/google            {id_token} → sets httpOnly JWT cookie
  POST   /api/auth/logout
  GET    /api/me                     POST /api/me/become-host

Public / attendee
  GET    /api/events?q=&city=&category=&from=&to=&page=
  GET    /api/events/:id             details + categories + availability summary
  GET    /api/events/:id/seats       seat map (NUMBERED)
  GET    /api/events/:id/stream      SSE
  POST   /api/holds                  {event_id, seat_ids[]}  |  {event_id, items:[{category_id, qty}]}
  DELETE /api/holds/:booking_id      release early
  POST   /api/bookings/:id/confirm   Idempotency-Key header, {simulate:"success"|"failure"}
  POST   /api/bookings/:id/cancel
  GET    /api/me/bookings            GET /api/bookings/:id

Host
  POST   /api/host/events            create draft
  PUT    /api/host/events/:id        update draft (details, mode, categories, layout)
  POST   /api/host/events/:id/poster upload signature for Cloudinary
  POST   /api/host/events/:id/publish
  POST   /api/host/events/:id/cancel cancels event + refunds all bookings
  GET    /api/host/events            my events
  GET    /api/host/events/:id/stats  sold/held/available per category, revenue, bookings over time
  GET    /api/host/events/:id/attendees(.csv)
```

Errors: JSON `{ "error": { "code": "SEATS_TAKEN", "message": "...", "details": {...} } }`.

---

## 9. Frontend pages

| Route | Page |
|---|---|
| `/` | Event discovery: search, filters, cards with availability badge |
| `/events/:id` | Event detail + **SeatMap** or **GA picker** |
| `/checkout/:bookingId` | Summary, 5:00 countdown, pay / simulate failure |
| `/bookings` | My bookings |
| `/bookings/:id` | Ticket view (code / QR) |
| `/host` | Host dashboard: my events, status, sold % |
| `/host/events/new`, `/host/events/:id/edit` | 4-step wizard incl. **LayoutDesigner** |
| `/host/events/:id` | Live stats, revenue, attendee list, read-only seat map coloured by status |

Key components: `SeatMap` (shared by customer, preview, host stats), `LayoutDesigner`,
`GaPicker`, `Countdown`, `useEventStream`.

---

## 10. Repository layout

```
eventhub/
├── PLAN.md
├── README.md                 # architecture diagram, concurrency explanation, test results
├── docker-compose.yml        # postgres, redis, api
├── backend/
│   ├── cmd/api/main.go
│   ├── internal/
│   │   ├── config/           # env loading
│   │   ├── db/               # pgx pool, sqlc generated code, tx helper
│   │   ├── auth/             # google verify, JWT, middleware
│   │   ├── events/           # handlers + service (create, publish, layout → seats)
│   │   ├── inventory/        # hold / confirm / cancel / release (seats + GA)
│   │   ├── realtime/         # redis pub/sub, SSE handler
│   │   ├── worker/           # hold-expiry sweeper
│   │   ├── ratelimit/
│   │   └── httpx/            # error format, request validation
│   ├── migrations/
│   ├── queries/              # *.sql for sqlc
│   └── tests/                # concurrency + integration tests
├── frontend/
│   └── src/{pages,components,api,hooks,lib}
└── loadtest/
    ├── race_one_seat.js      # k6
    └── ga_oversell.js
```

---

## 11. Testing — proving correctness

| Test | Setup | Pass condition |
|---|---|---|
| Seat race | 1,000 goroutines try to hold seat A5 | exactly **1** success, 999 × 409 |
| Multi-seat overlap | User X holds A1–A3, User Y holds A3–A5 simultaneously | exactly one succeeds, no partial holds |
| GA oversell | capacity 100, 1,000 goroutines × qty 1 | `sold + held == 100`, never > 100 |
| Expiry | hold, fast-forward `expires_at`, another user holds | second user wins; first confirm → 410 |
| Idempotency | confirm twice with same key | 1 payment row, same response |
| Ownership | user B confirms user A's booking | 403 |
| Layout generation | 10×16 grid with 2 aisle columns + 4 blocked | correct labels, 140 seats, 4 BLOCKED |
| Load (k6) | 200 VUs for 60 s on hold/confirm | record p95 latency + req/s for README/resume |

---

## 12. Build schedule (~3 focused days)

### Day 1 — Foundation + host side
- [ ] Compose (postgres, redis), migrations, sqlc, Gin skeleton, config, error format
- [ ] Google login → JWT cookie, `/me`, become-host
- [ ] Host event CRUD (draft), categories
- [ ] Frontend: Vite + Tailwind + router, login, host wizard steps 1–3b
- **Done when:** a host can sign in and save a GA draft event.

### Day 2 — Layout designer + booking engine
- [ ] `LayoutDesigner` (grid, presets, paint, row-click, aisle, blocked, summary, preview)
- [ ] Publish → seat generation / GA counters
- [ ] Hold / confirm / cancel / release (both modes), sweeper
- [ ] Concurrency tests (seat race, GA oversell) green
- **Done when:** two browsers race for the same seat and only one wins.

### Day 3 — Customer UX, realtime, ship
- [ ] Discovery page, event page, `SeatMap`, `GaPicker`, checkout countdown, my bookings
- [ ] SSE + Redis pub/sub live updates
- [ ] Host stats dashboard + attendee CSV
- [ ] Rate limiting, k6 run, seed demo events (1 movie numbered, 1 comedy GA, 1 concert numbered)
- [ ] Deploy (Render + Neon + Upstash + Vercel), README with diagram & test numbers
- **Done when:** live demo link works end to end.

### Stretch (after v1)
QR tickets + host "scan at door" check-in · email confirmation · Razorpay test mode ·
reusable venue layouts (`venues` table) · multiple shows per event · waitlist when sold out.

---

## 13. Environment variables

```
# backend
DATABASE_URL=postgres://eventhub:eventhub@localhost:5432/eventhub?sslmode=disable
REDIS_URL=redis://localhost:6379
GOOGLE_CLIENT_ID=xxxx.apps.googleusercontent.com
JWT_SECRET=<random 32+ bytes>
CLOUDINARY_URL=cloudinary://key:secret@cloud
HOLD_MINUTES=5
FRONTEND_ORIGIN=http://localhost:5173

# frontend
VITE_GOOGLE_CLIENT_ID=xxxx.apps.googleusercontent.com
VITE_API_BASE=/api
```

---

## 14. Interview talking points

1. **Check-then-act race** — why `SELECT` then `UPDATE` double-books, and why a conditional `UPDATE … WHERE status='AVAILABLE'` doesn't.
2. **Two inventory models** — row-per-seat vs counter; why GA doesn't need 100 rows.
3. **DB constraint as last line of defence** — `CHECK (sold + held <= capacity)`.
4. **Single source of truth** — holds live in Postgres, not Redis, to avoid dual-write inconsistency.
5. **Expiry without trusting timers** — lazy expiry in the WHERE clause + sweeper with `SKIP LOCKED`.
6. **Idempotency keys** for payment retries.
7. **Scaling** — SSE fan-out via Redis pub/sub across instances; hot-event contention on one GA counter row and how to shard it (split capacity into N buckets) if needed.

## 15. Resume entry (fill real numbers after k6)

**EventHub | Go, React, PostgreSQL, Redis — Live Demo**
- Built a configurable event-ticketing platform with Google OAuth, a grid-based venue layout designer, and support for both numbered seating and general-admission inventory.
- Guaranteed zero double bookings under N concurrent requests using atomic PostgreSQL check-and-set transactions, DB constraints, and 5-minute expiring holds; added idempotent checkout.
- Delivered real-time seat availability via SSE and Redis pub/sub; load-tested at N req/s (p95 N ms) with k6.
