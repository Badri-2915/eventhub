CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE event_category AS ENUM ('MOVIE', 'COMEDY', 'CONCERT', 'THEATRE', 'WORKSHOP', 'OTHER');
CREATE TYPE seating_mode   AS ENUM ('NUMBERED', 'GA');
CREATE TYPE event_status   AS ENUM ('DRAFT', 'PUBLISHED', 'CANCELLED', 'COMPLETED');
CREATE TYPE seat_status    AS ENUM ('AVAILABLE', 'HELD', 'BOOKED', 'BLOCKED');
CREATE TYPE booking_status AS ENUM ('PENDING', 'CONFIRMED', 'CANCELLED', 'EXPIRED', 'FAILED');
CREATE TYPE payment_status AS ENUM ('SUCCESS', 'FAILED', 'REFUNDED');

CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    google_sub  text NOT NULL UNIQUE,
    email       text NOT NULL,
    name        text NOT NULL DEFAULT '',
    avatar_url  text NOT NULL DEFAULT '',
    is_host     boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE events (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id             uuid NOT NULL REFERENCES users(id),
    title               text NOT NULL,
    description         text NOT NULL DEFAULT '',
    category            event_category NOT NULL DEFAULT 'OTHER',
    poster_url          text NOT NULL DEFAULT '',
    venue_name          text NOT NULL DEFAULT '',
    city                text NOT NULL DEFAULT '',
    address             text NOT NULL DEFAULT '',
    starts_at           timestamptz,
    ends_at             timestamptz,
    seating_mode        seating_mode NOT NULL DEFAULT 'GA',
    status              event_status NOT NULL DEFAULT 'DRAFT',
    layout              jsonb,
    max_per_booking     int NOT NULL DEFAULT 10 CHECK (max_per_booking BETWEEN 1 AND 50),
    cancel_cutoff_hours int NOT NULL DEFAULT 24 CHECK (cancel_cutoff_hours >= 0),
    created_at          timestamptz NOT NULL DEFAULT now(),
    published_at        timestamptz,
    CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at)
);
CREATE INDEX events_host_idx   ON events (host_id);
CREATE INDEX events_browse_idx ON events (status, starts_at);

CREATE TABLE ticket_categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id    uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name        text NOT NULL,
    color       text NOT NULL DEFAULT '#14b8a6',
    sort_order  int NOT NULL DEFAULT 0,
    price_paise int NOT NULL CHECK (price_paise >= 0),
    capacity    int CHECK (capacity IS NULL OR capacity >= 0),
    sold        int NOT NULL DEFAULT 0 CHECK (sold >= 0),
    held        int NOT NULL DEFAULT 0 CHECK (held >= 0),
    -- DB-level oversell guard: the last line of defence for GA inventory
    CHECK (capacity IS NULL OR sold + held <= capacity)
);
CREATE INDEX ticket_categories_event_idx ON ticket_categories (event_id);

CREATE TABLE bookings (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id),
    event_id     uuid NOT NULL REFERENCES events(id),
    status       booking_status NOT NULL DEFAULT 'PENDING',
    total_paise  int NOT NULL DEFAULT 0 CHECK (total_paise >= 0),
    expires_at   timestamptz NOT NULL,
    ticket_code  text UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    cancelled_at timestamptz
);
-- at most one PENDING booking per user per event
CREATE UNIQUE INDEX bookings_one_pending_idx ON bookings (user_id, event_id) WHERE status = 'PENDING';
CREATE INDEX bookings_user_idx    ON bookings (user_id, created_at DESC);
CREATE INDEX bookings_expiry_idx  ON bookings (expires_at) WHERE status = 'PENDING';

CREATE TABLE seats (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    category_id     uuid NOT NULL REFERENCES ticket_categories(id),
    row_label       text NOT NULL,
    seat_number     int NOT NULL,
    label           text NOT NULL,
    grid_row        int NOT NULL,
    grid_col        int NOT NULL,
    status          seat_status NOT NULL DEFAULT 'AVAILABLE',
    booking_id      uuid REFERENCES bookings(id),
    hold_expires_at timestamptz,
    UNIQUE (event_id, label)
);
CREATE INDEX seats_event_status_idx ON seats (event_id, status);
CREATE INDEX seats_booking_idx      ON seats (booking_id) WHERE booking_id IS NOT NULL;

CREATE TABLE booking_items (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id       uuid NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    category_id      uuid NOT NULL REFERENCES ticket_categories(id),
    seat_id          uuid REFERENCES seats(id),
    quantity         int NOT NULL CHECK (quantity > 0),
    unit_price_paise int NOT NULL CHECK (unit_price_paise >= 0)
);
CREATE INDEX booking_items_booking_idx ON booking_items (booking_id);

CREATE TABLE payments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id      uuid NOT NULL REFERENCES bookings(id),
    provider        text NOT NULL DEFAULT 'SIMULATED',
    status          payment_status NOT NULL,
    amount_paise    int NOT NULL CHECK (amount_paise >= 0),
    idempotency_key text NOT NULL UNIQUE,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payments_booking_idx ON payments (booking_id);
