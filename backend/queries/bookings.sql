-- name: GetPendingBookingForUpdate :one
SELECT * FROM bookings
WHERE user_id = $1 AND event_id = $2 AND status = 'PENDING'
FOR UPDATE;

-- name: CreateBooking :one
INSERT INTO bookings (user_id, event_id, expires_at) VALUES ($1, $2, $3) RETURNING *;

-- name: SetBookingTotal :exec
UPDATE bookings SET total_paise = $2 WHERE id = $1;

-- name: GetBooking :one
SELECT * FROM bookings WHERE id = $1;

-- name: GetBookingForUpdate :one
SELECT * FROM bookings WHERE id = $1 FOR UPDATE;

-- name: ConfirmBooking :one
UPDATE bookings SET status = 'CONFIRMED', ticket_code = $2, confirmed_at = now()
WHERE id = $1 RETURNING *;

-- name: SetBookingStatus :one
UPDATE bookings SET status = sqlc.arg(status)::booking_status,
    cancelled_at = CASE WHEN sqlc.arg(status)::booking_status = 'CANCELLED' THEN now() ELSE cancelled_at END
WHERE id = sqlc.arg(id) RETURNING *;

-- name: ClaimExpiredBookings :many
-- SKIP LOCKED lets several sweepers (or API instances) share the work safely.
SELECT * FROM bookings
WHERE status = 'PENDING' AND expires_at < now()
  AND (sqlc.narg(event_id)::uuid IS NULL OR event_id = sqlc.narg(event_id))
ORDER BY expires_at
LIMIT sqlc.arg(lim)
FOR UPDATE SKIP LOCKED;

-- name: InsertBookingItem :exec
INSERT INTO booking_items (booking_id, category_id, seat_id, quantity, unit_price_paise)
VALUES ($1, $2, $3, $4, $5);

-- name: ListBookingItems :many
SELECT * FROM booking_items WHERE booking_id = $1;

-- name: ListBookingItemsDetailed :many
SELECT bi.id, bi.category_id, bi.seat_id, bi.quantity, bi.unit_price_paise,
    tc.name AS category_name, tc.color AS category_color,
    COALESCE(s.label, '')::text AS seat_label
FROM booking_items bi
JOIN ticket_categories tc ON tc.id = bi.category_id
LEFT JOIN seats s ON s.id = bi.seat_id
WHERE bi.booking_id = $1
ORDER BY tc.sort_order, s.grid_row, s.grid_col;

-- name: ListUserBookings :many
SELECT b.*, e.title AS event_title, e.venue_name AS event_venue, e.city AS event_city,
    e.starts_at AS event_starts_at, e.poster_url AS event_poster_url, e.category AS event_category,
    (SELECT COALESCE(sum(quantity), 0) FROM booking_items WHERE booking_id = b.id)::int AS ticket_count
FROM bookings b
JOIN events e ON e.id = b.event_id
WHERE b.user_id = $1 AND b.status IN ('PENDING', 'CONFIRMED', 'CANCELLED')
ORDER BY e.starts_at DESC;

-- seats ------------------------------------------------------------------

-- name: HoldSeats :many
-- Lock target rows in id order (avoids deadlocks between overlapping holds),
-- then a single conditional check-and-set: only free or expired seats match.
WITH target AS (
    SELECT t.id FROM seats t
    WHERE t.event_id = sqlc.arg(event_id) AND t.id = ANY(sqlc.arg(seat_ids)::uuid[])
    ORDER BY t.id
    FOR UPDATE
)
UPDATE seats s
SET status = 'HELD', booking_id = sqlc.arg(booking_id), hold_expires_at = sqlc.arg(expires_at)
FROM target
WHERE s.id = target.id
  AND (s.status = 'AVAILABLE' OR (s.status = 'HELD' AND s.hold_expires_at < now()))
RETURNING s.id, s.category_id, s.status;

-- name: BookHeldSeats :many
UPDATE seats SET status = 'BOOKED', hold_expires_at = NULL
WHERE booking_id = $1 AND status = 'HELD'
RETURNING id, status;

-- name: ReleaseSeats :many
UPDATE seats SET status = 'AVAILABLE', booking_id = NULL, hold_expires_at = NULL
WHERE booking_id = $1 AND status IN ('HELD', 'BOOKED')
RETURNING id, status;

-- GA counters ------------------------------------------------------------

-- name: HoldGA :one
UPDATE ticket_categories SET held = held + sqlc.arg(qty)
WHERE id = sqlc.arg(id) AND event_id = sqlc.arg(event_id)
  AND capacity IS NOT NULL AND capacity - sold - held >= sqlc.arg(qty)
RETURNING id, price_paise, (capacity - sold - held)::int AS available;

-- name: GACategoryAvailable :one
SELECT (COALESCE(capacity, 0) - sold - held)::int AS available
FROM ticket_categories WHERE id = $1 AND event_id = $2;

-- name: ConfirmGA :one
UPDATE ticket_categories SET held = held - sqlc.arg(qty), sold = sold + sqlc.arg(qty)
WHERE id = sqlc.arg(id)
RETURNING id, (capacity - sold - held)::int AS available;

-- name: ReleaseGAHeld :one
UPDATE ticket_categories SET held = held - sqlc.arg(qty)
WHERE id = sqlc.arg(id)
RETURNING id, (capacity - sold - held)::int AS available;

-- name: ReleaseGASold :one
UPDATE ticket_categories SET sold = sold - sqlc.arg(qty)
WHERE id = sqlc.arg(id)
RETURNING id, (capacity - sold - held)::int AS available;

-- payments ---------------------------------------------------------------

-- name: GetPaymentByKey :one
SELECT * FROM payments WHERE idempotency_key = $1;

-- name: InsertPayment :one
INSERT INTO payments (booking_id, status, amount_paise, idempotency_key)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: RefundPayments :exec
UPDATE payments SET status = 'REFUNDED' WHERE booking_id = $1 AND status = 'SUCCESS';
