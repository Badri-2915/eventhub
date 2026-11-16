-- name: GetTicketByCode :one
SELECT b.id, b.status, b.checked_in_at, b.event_id, e.host_id, e.title AS event_title,
    e.starts_at, e.ends_at,
    u.name AS attendee_name, u.email AS attendee_email,
    (SELECT COALESCE(sum(quantity), 0) FROM booking_items WHERE booking_id = b.id)::int AS tickets,
    (SELECT string_agg(CASE WHEN s.label IS NOT NULL THEN s.label ELSE bi.quantity || ' × ' || tc.name END, ', '
                       ORDER BY tc.sort_order, s.grid_row, s.grid_col)
     FROM booking_items bi
     JOIN ticket_categories tc ON tc.id = bi.category_id
     LEFT JOIN seats s ON s.id = bi.seat_id
     WHERE bi.booking_id = b.id)::text AS items
FROM bookings b
JOIN events e ON e.id = b.event_id
JOIN users u ON u.id = b.user_id
WHERE b.ticket_code = $1;

-- name: MarkCheckedIn :one
-- Conditional update: if two staff scan at once, only one wins.
UPDATE bookings SET checked_in_at = now()
WHERE id = $1 AND status = 'CONFIRMED' AND checked_in_at IS NULL
RETURNING checked_in_at;

-- name: UndoCheckIn :one
-- For a mistaken scan at the door; the ticket can then be scanned again.
UPDATE bookings SET checked_in_at = NULL
WHERE id = $1 AND status = 'CONFIRMED' AND checked_in_at IS NOT NULL
RETURNING id;

-- name: CheckInCounts :one
-- Guests (tickets, not bookings) who have arrived vs. all confirmed tickets.
SELECT COALESCE(sum(bi.quantity) FILTER (WHERE b.checked_in_at IS NOT NULL), 0)::int AS arrived,
       COALESCE(sum(bi.quantity), 0)::int AS expected
FROM bookings b
JOIN booking_items bi ON bi.booking_id = b.id
WHERE b.event_id = $1 AND b.status = 'CONFIRMED';
