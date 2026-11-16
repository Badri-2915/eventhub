-- name: AdminOverview :one
SELECT
    (SELECT count(*) FROM users)::int AS users,
    (SELECT count(*) FROM users WHERE is_host)::int AS hosts,
    (SELECT count(*) FROM events WHERE status <> 'DRAFT')::int AS events,
    (SELECT count(*) FROM events WHERE status = 'PUBLISHED')::int AS live_events,
    (SELECT count(*) FROM bookings WHERE status = 'CONFIRMED')::int AS bookings,
    (SELECT COALESCE(sum(total_paise), 0) FROM bookings WHERE status = 'CONFIRMED')::bigint AS revenue_paise;

-- name: AdminListEvents :many
SELECT e.id, e.title, e.category, e.status, e.city, e.starts_at, e.seating_mode,
    u.name AS host_name, u.email AS host_email,
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
JOIN users u ON u.id = e.host_id
WHERE e.status <> 'DRAFT'
ORDER BY e.starts_at DESC NULLS LAST
LIMIT 200;
