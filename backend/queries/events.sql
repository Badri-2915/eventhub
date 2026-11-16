-- name: CreateEvent :one
INSERT INTO events (
    host_id, title, description, category, poster_url, venue_name, city, address,
    starts_at, ends_at, seating_mode, layout, max_per_booking, cancel_cutoff_hours, map_url
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: UpdateDraftEvent :one
UPDATE events SET
    title = $3, description = $4, category = $5, poster_url = $6,
    venue_name = $7, city = $8, address = $9, starts_at = $10, ends_at = $11,
    seating_mode = $12, layout = $13, max_per_booking = $14, cancel_cutoff_hours = $15,
    map_url = $16
WHERE id = $1 AND host_id = $2 AND status = 'DRAFT'
RETURNING *;

-- name: GetEvent :one
SELECT * FROM events WHERE id = $1;

-- name: GetEventForHost :one
SELECT * FROM events WHERE id = $1 AND host_id = $2;

-- name: ListHostEvents :many
SELECT * FROM events WHERE host_id = $1 ORDER BY created_at DESC;

-- name: ListCategories :many
SELECT * FROM ticket_categories WHERE event_id = $1 ORDER BY sort_order, name;

-- name: DeleteCategories :exec
DELETE FROM ticket_categories WHERE event_id = $1;

-- name: InsertCategory :one
INSERT INTO ticket_categories (id, event_id, name, color, sort_order, price_paise, capacity, list_price_paise, discount_percent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateLiveEvent :one
-- Published events: everything except seating mode and layout can change.
UPDATE events SET
    title = $3, description = $4, category = $5, poster_url = $6,
    venue_name = $7, city = $8, address = $9, starts_at = $10, ends_at = $11,
    max_per_booking = $12, cancel_cutoff_hours = $13, map_url = $14
WHERE id = $1 AND host_id = $2 AND status = 'PUBLISHED'
RETURNING *;

-- name: UpdateLiveCategory :one
-- New prices apply to new holds only: booking_items keep the price paid. A GA
-- capacity below sold + held fails the table's CHECK constraint.
UPDATE ticket_categories SET
    name = $3, color = $4, price_paise = $5, list_price_paise = $6, discount_percent = $7,
    capacity = CASE WHEN capacity IS NULL THEN NULL ELSE sqlc.narg(capacity)::int END
WHERE id = $1 AND event_id = $2
RETURNING *;

-- name: LockEventForHost :one
SELECT * FROM events WHERE id = $1 AND host_id = $2 FOR UPDATE;

-- name: PublishEvent :one
UPDATE events SET status = 'PUBLISHED', published_at = now() WHERE id = $1 RETURNING *;

-- name: InsertSeats :copyfrom
INSERT INTO seats (event_id, category_id, row_label, seat_number, label, grid_row, grid_col, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListPublicEvents :many
SELECT e.id, e.title, e.category, e.poster_url, e.venue_name, e.city, e.starts_at, e.seating_mode,
    COALESCE((SELECT min(price_paise) FROM ticket_categories WHERE event_id = e.id), 0)::int AS min_price_paise,
    COALESCE((SELECT max(price_paise) FROM ticket_categories WHERE event_id = e.id), 0)::int AS max_price_paise,
    COALESCE((SELECT max(discount_percent) FROM ticket_categories WHERE event_id = e.id), 0)::int AS max_discount_percent,
    (CASE WHEN e.seating_mode = 'GA'
        THEN COALESCE((SELECT sum(capacity) FROM ticket_categories WHERE event_id = e.id), 0)
        ELSE (SELECT count(*) FROM seats WHERE event_id = e.id AND status <> 'BLOCKED')
    END)::int AS total_tickets,
    (CASE WHEN e.seating_mode = 'GA'
        THEN COALESCE((SELECT sum(capacity - sold - held) FROM ticket_categories WHERE event_id = e.id), 0)
        ELSE (SELECT count(*) FROM seats WHERE event_id = e.id
              AND (status = 'AVAILABLE' OR (status = 'HELD' AND hold_expires_at < now())))
    END)::int AS available_tickets
FROM events e
WHERE e.status = 'PUBLISHED' AND e.starts_at > now()
  AND (sqlc.narg(q)::text IS NULL
       OR e.title ILIKE '%' || sqlc.narg(q) || '%'
       OR e.venue_name ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(city)::text IS NULL OR lower(e.city) = lower(sqlc.narg(city)))
  AND (sqlc.narg(category)::event_category IS NULL OR e.category = sqlc.narg(category))
ORDER BY e.starts_at
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListPublicCities :many
SELECT DISTINCT city FROM events
WHERE status = 'PUBLISHED' AND starts_at > now() AND city <> ''
ORDER BY city;

-- name: SeatCountsByCategory :many
SELECT category_id,
    count(*) FILTER (WHERE status <> 'BLOCKED')::int AS total,
    count(*) FILTER (WHERE status = 'AVAILABLE' OR (status = 'HELD' AND hold_expires_at < now()))::int AS available,
    count(*) FILTER (WHERE status = 'HELD' AND hold_expires_at >= now())::int AS held,
    count(*) FILTER (WHERE status = 'BOOKED')::int AS sold
FROM seats WHERE event_id = $1
GROUP BY category_id;

-- name: ListSeats :many
-- Expired holds are reported as AVAILABLE: the hold query treats them as free.
SELECT id, category_id, row_label, seat_number, label, grid_row, grid_col,
    (CASE WHEN status = 'HELD' AND hold_expires_at < now() THEN 'AVAILABLE' ELSE status END)::seat_status AS status
FROM seats WHERE event_id = $1
ORDER BY grid_row, grid_col;

-- name: GetEventForShare :one
-- Holds take a shared lock: many holds run in parallel, but a host cancelling
-- (FOR UPDATE) waits for them and they wait for it.
SELECT * FROM events WHERE id = $1 FOR SHARE;

-- name: DeleteDraftEvent :execrows
-- Only drafts: they have no seats, bookings or payments yet (categories cascade).
DELETE FROM events WHERE id = $1 AND host_id = $2 AND status = 'DRAFT';

-- name: LockEvent :one
SELECT * FROM events WHERE id = $1 FOR UPDATE;
