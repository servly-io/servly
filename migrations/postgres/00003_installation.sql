-- One row once the installation has been claimed: by the first-run setup in
-- the browser, or by `servly tenant create`. The fixed primary key makes the
-- claim atomic: a second claim fails on the key, on either dialect.

-- +goose Up
CREATE TABLE installation (
    id              integer PRIMARY KEY CHECK (id = 1),
    initialized_at  timestamptz NOT NULL
);
-- Installations that already have a restaurant are claimed.
INSERT INTO installation (id, initialized_at) SELECT 1, MIN(created_at) FROM tenants HAVING COUNT(*) > 0;

-- +goose Down
DROP TABLE installation;
