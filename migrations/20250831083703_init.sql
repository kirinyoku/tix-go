-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS venues (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    seating_scheme JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS seats (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    section TEXT NOT NULL,
    row INT NOT NULL,
    number INT NOT NULL,
    UNIQUE (venue_id, section, row, number)
);

CREATE TYPE seat_status AS ENUM ('available', 'held', 'sold');

CREATE TABLE IF NOT EXISTS event_seats (
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    seat_id BIGINT NOT NULL REFERENCES seats(id) ON DELETE CASCADE,
    status seat_status NOT NULL DEFAULT 'available',
    hold_id UUID NULL,
    hold_expires_at TIMESTAMPTZ NULL,
    PRIMARY KEY (event_id, seat_id)
);

CREATE INDEX idx_event_seats_event_status
  ON event_seats(event_id, status);

CREATE INDEX idx_event_seats_hold_expires
  ON event_seats(hold_expires_at);
  
CREATE TABLE IF NOT EXISTS holds (
    id UUID PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    total_cents INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tickets (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    seat_id BIGINT NOT NULL REFERENCES seats(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, seat_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE tickets;
DROP TABLE orders;
DROP TABLE holds;
DROP INDEX IF EXISTS idx_event_seats_hold_expires;
DROP INDEX IF EXISTS idx_event_seats_event_status;
DROP TABLE event_seats;
DROP TABLE seats;
DROP TABLE events;
DROP TABLE venues;
DROP TYPE seat_status;
-- +goose StatementEnd
