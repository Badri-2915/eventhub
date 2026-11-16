-- name: HostEventSummaries :many
-- Per-event sold / capacity / revenue for the host dashboard.
SELECT e.id,
    (CASE WHEN e.seating_mode = 'GA'
        THEN COALESCE((SELECT sum(capacity) FROM ticket_categories WHERE event_id = e.id), 0)
        ELSE (SELECT count(*) FROM seats WHERE event_id = e.id AND status <> 'BLOCKED')
    END)::int AS capacity,
    (CASE WHEN e.seating_mode = 'GA'
        THEN COALESCE((SELECT sum(sold) FROM ticket_categories WHERE event_id = e.id), 0)
        ELSE (SELECT count(*) FROM seats WHERE event_id = e.id AND status = 'BOOKED')
    END)::int AS sold,
    COALESCE((SELECT sum(total_paise) FROM bookings WHERE event_id = e.id AND status = 'CONFIRMED'), 0)::bigint AS revenue_paise
FROM events e
WHERE e.host_id = $1;

-- name: EventRevenue :one
SELECT COALESCE(sum(total_paise), 0)::bigint AS revenue_paise, count(*)::int AS bookings
FROM bookings WHERE event_id = $1 AND status = 'CONFIRMED';

-- name: SalesByDay :many
SELECT (confirmed_at AT TIME ZONE 'Asia/Kolkata')::date AS day,
    count(*)::int AS bookings,
    COALESCE(sum(total_paise), 0)::bigint AS revenue_paise,
    COALESCE(sum((SELECT sum(quantity) FROM booking_items WHERE booking_id = b.id)), 0)::int AS tickets
FROM bookings b
WHERE event_id = $1 AND status = 'CONFIRMED'
GROUP BY 1 ORDER BY 1;

-- name: ListAttendees :many
SELECT b.id, b.ticket_code, b.total_paise, b.confirmed_at, b.checked_in_at, u.name, u.email,
    (SELECT COALESCE(sum(quantity), 0) FROM booking_items WHERE booking_id = b.id)::int AS tickets,
    (SELECT string_agg(CASE WHEN s.label IS NOT NULL THEN s.label ELSE bi.quantity || ' × ' || tc.name END, ', '
                       ORDER BY tc.sort_order, s.grid_row, s.grid_col)
     FROM booking_items bi
     JOIN ticket_categories tc ON tc.id = bi.category_id
     LEFT JOIN seats s ON s.id = bi.seat_id
     WHERE bi.booking_id = b.id)::text AS items
FROM bookings b
JOIN users u ON u.id = b.user_id
WHERE b.event_id = $1 AND b.status = 'CONFIRMED'
ORDER BY b.confirmed_at DESC;

-- name: ListLiveBookingsForUpdate :many
SELECT * FROM bookings
WHERE event_id = $1 AND status IN ('PENDING', 'CONFIRMED')
ORDER BY id
FOR UPDATE;

-- name: SetEventStatus :one
UPDATE events SET status = $2 WHERE id = $1 RETURNING *;

-- name: SetEventPoster :one
UPDATE events SET poster_url = $3 WHERE id = $1 AND host_id = $2 RETURNING *;
