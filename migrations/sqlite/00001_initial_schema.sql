-- Initial schema, SQLite dialect. Contract: docs/docs/domain-model.md.
--
-- Conventions (SQLite):
--   ids          TEXT, canonical lowercase UUID (36 chars)
--   timestamps   TEXT, UTC, fixed width: 2006-01-02T15:04:05.000000Z
--   booleans     INTEGER 0/1 with a CHECK
--   money        INTEGER minor units; currency is TEXT ISO 4217
--   json         TEXT holding a JSON document
-- The PostgreSQL migration of the same number must describe the same schema.

-- +goose Up

CREATE TABLE tenants (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('active', 'suspended')),
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE users (
    id                 TEXT PRIMARY KEY,
    email              TEXT NOT NULL UNIQUE,
    password_hash      TEXT NOT NULL,
    display_name       TEXT NOT NULL,
    is_platform_admin  INTEGER NOT NULL DEFAULT 0 CHECK (is_platform_admin IN (0, 1)),
    status             TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE TABLE tenant_memberships (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants (id),
    user_id     TEXT NOT NULL REFERENCES users (id),
    role        TEXT NOT NULL CHECK (role IN ('owner', 'manager', 'staff')),
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX tenant_memberships_user ON tenant_memberships (user_id);

CREATE TABLE restaurant_locations (
    id                     TEXT PRIMARY KEY,
    tenant_id              TEXT NOT NULL REFERENCES tenants (id),
    name                   TEXT NOT NULL,
    currency               TEXT NOT NULL CHECK (length(currency) = 3),
    timezone               TEXT NOT NULL,
    order_notes_enabled    INTEGER NOT NULL DEFAULT 1 CHECK (order_notes_enabled IN (0, 1)),
    public_status_enabled  INTEGER NOT NULL DEFAULT 1 CHECK (public_status_enabled IN (0, 1)),
    status                 TEXT NOT NULL CHECK (status IN ('active', 'closed')),
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    UNIQUE (tenant_id, id)
);

CREATE TABLE tenant_domains (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants (id),
    location_id  TEXT,
    hostname     TEXT NOT NULL UNIQUE,
    created_at   TEXT NOT NULL,
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);
CREATE INDEX tenant_domains_tenant ON tenant_domains (tenant_id);

CREATE TABLE sessions (
    token_hash        TEXT PRIMARY KEY,
    user_id           TEXT NOT NULL REFERENCES users (id),
    active_tenant_id  TEXT REFERENCES tenants (id),
    csrf_token        TEXT NOT NULL,
    created_at        TEXT NOT NULL,
    last_seen_at      TEXT NOT NULL,
    expires_at        TEXT NOT NULL
);
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE assets (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants (id),
    storage_key   TEXT NOT NULL UNIQUE,
    content_type  TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL CHECK (size_bytes >= 0),
    sha256        TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    UNIQUE (tenant_id, id)
);

CREATE TABLE menu_categories (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    location_id  TEXT NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    position     INTEGER NOT NULL,
    is_enabled   INTEGER NOT NULL DEFAULT 1 CHECK (is_enabled IN (0, 1)),
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);
CREATE INDEX menu_categories_location ON menu_categories (tenant_id, location_id, position);

CREATE TABLE menu_items (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    category_id     TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    price_minor     INTEGER NOT NULL CHECK (price_minor >= 0),
    image_asset_id  TEXT,
    position        INTEGER NOT NULL,
    is_enabled      INTEGER NOT NULL DEFAULT 1 CHECK (is_enabled IN (0, 1)),
    is_available    INTEGER NOT NULL DEFAULT 1 CHECK (is_available IN (0, 1)),
    archived_at     TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, category_id) REFERENCES menu_categories (tenant_id, id),
    FOREIGN KEY (tenant_id, image_asset_id) REFERENCES assets (tenant_id, id)
);
CREATE INDEX menu_items_category ON menu_items (tenant_id, category_id, position);

CREATE TABLE option_groups (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    item_id     TEXT NOT NULL,
    name        TEXT NOT NULL,
    min_select  INTEGER NOT NULL DEFAULT 0 CHECK (min_select >= 0),
    max_select  INTEGER NOT NULL CHECK (max_select >= 1 AND max_select >= min_select),
    position    INTEGER NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, item_id) REFERENCES menu_items (tenant_id, id)
);
CREATE INDEX option_groups_item ON option_groups (tenant_id, item_id, position);

CREATE TABLE options (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    option_group_id    TEXT NOT NULL,
    name               TEXT NOT NULL,
    price_delta_minor  INTEGER NOT NULL DEFAULT 0,
    position           INTEGER NOT NULL,
    is_available       INTEGER NOT NULL DEFAULT 1 CHECK (is_available IN (0, 1)),
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, option_group_id) REFERENCES option_groups (tenant_id, id)
);
CREATE INDEX options_group ON options (tenant_id, option_group_id, position);

CREATE TABLE dining_locations (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    location_id   TEXT NOT NULL,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('table', 'pickup')),
    public_token  TEXT NOT NULL UNIQUE,
    is_enabled    INTEGER NOT NULL DEFAULT 1 CHECK (is_enabled IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);
CREATE INDEX dining_locations_location ON dining_locations (tenant_id, location_id);

CREATE TABLE order_number_counters (
    tenant_id      TEXT NOT NULL,
    location_id    TEXT NOT NULL,
    business_date  TEXT NOT NULL CHECK (business_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    last_number    INTEGER NOT NULL,
    PRIMARY KEY (tenant_id, location_id, business_date),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);

CREATE TABLE orders (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    location_id         TEXT NOT NULL,
    dining_location_id  TEXT,
    order_number        INTEGER NOT NULL CHECK (order_number > 0),
    business_date       TEXT NOT NULL CHECK (business_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    public_token        TEXT NOT NULL UNIQUE,
    mode                TEXT NOT NULL CHECK (mode IN ('dine_in', 'pickup')),
    status              TEXT NOT NULL CHECK (status IN ('new', 'accepted', 'preparing', 'ready', 'completed', 'cancelled')),
    payment_status      TEXT NOT NULL CHECK (payment_status IN ('unpaid', 'pending', 'paid', 'partially_refunded', 'refunded')),
    currency            TEXT NOT NULL CHECK (length(currency) = 3),
    subtotal_minor      INTEGER NOT NULL CHECK (subtotal_minor >= 0),
    total_minor         INTEGER NOT NULL CHECK (total_minor >= 0),
    notes               TEXT NOT NULL DEFAULT '',
    version             INTEGER NOT NULL DEFAULT 1,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, location_id, business_date, order_number),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id),
    FOREIGN KEY (tenant_id, dining_location_id) REFERENCES dining_locations (tenant_id, id)
);
CREATE INDEX orders_board ON orders (tenant_id, location_id, status, created_at);
CREATE INDEX orders_created ON orders (tenant_id, location_id, created_at);

CREATE TABLE order_items (
    id                   TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL,
    order_id             TEXT NOT NULL,
    menu_item_id         TEXT,
    position             INTEGER NOT NULL,
    name                 TEXT NOT NULL,
    quantity             INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_minor     INTEGER NOT NULL CHECK (unit_price_minor >= 0),
    options_total_minor  INTEGER NOT NULL,
    line_total_minor     INTEGER NOT NULL CHECK (line_total_minor >= 0),
    notes                TEXT NOT NULL DEFAULT '',
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id),
    FOREIGN KEY (tenant_id, menu_item_id) REFERENCES menu_items (tenant_id, id)
);
CREATE INDEX order_items_order ON order_items (tenant_id, order_id, position);

CREATE TABLE order_item_options (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    order_item_id      TEXT NOT NULL,
    option_id          TEXT,
    group_name         TEXT NOT NULL,
    option_name        TEXT NOT NULL,
    price_delta_minor  INTEGER NOT NULL,
    FOREIGN KEY (tenant_id, order_item_id) REFERENCES order_items (tenant_id, id),
    FOREIGN KEY (tenant_id, option_id) REFERENCES options (tenant_id, id)
);
CREATE INDEX order_item_options_item ON order_item_options (tenant_id, order_item_id);

CREATE TABLE order_status_events (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    order_id       TEXT NOT NULL,
    from_status    TEXT,
    to_status      TEXT NOT NULL,
    actor_user_id  TEXT REFERENCES users (id),
    reason         TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);
CREATE INDEX order_status_events_order ON order_status_events (tenant_id, order_id, created_at);

CREATE TABLE payments (
    id                   TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL,
    order_id             TEXT NOT NULL,
    method               TEXT NOT NULL CHECK (method IN ('cash', 'external_terminal', 'other')),
    status               TEXT NOT NULL CHECK (status IN ('pending', 'paid', 'failed', 'partially_refunded', 'refunded')),
    amount_minor         INTEGER NOT NULL CHECK (amount_minor > 0),
    refunded_minor       INTEGER NOT NULL DEFAULT 0 CHECK (refunded_minor >= 0 AND refunded_minor <= amount_minor),
    currency             TEXT NOT NULL CHECK (length(currency) = 3),
    reference            TEXT NOT NULL DEFAULT '',
    recorded_by_user_id  TEXT REFERENCES users (id),
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);
CREATE INDEX payments_order ON payments (tenant_id, order_id);

CREATE TABLE audit_events (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT REFERENCES tenants (id),
    actor_user_id  TEXT REFERENCES users (id),
    action         TEXT NOT NULL,
    entity_type    TEXT NOT NULL,
    entity_id      TEXT NOT NULL,
    data           TEXT NOT NULL DEFAULT '{}',
    created_at     TEXT NOT NULL
);
CREATE INDEX audit_events_tenant ON audit_events (tenant_id, created_at);
CREATE INDEX audit_events_entity ON audit_events (tenant_id, entity_type, entity_id);

CREATE TABLE idempotency_records (
    tenant_id     TEXT NOT NULL REFERENCES tenants (id),
    scope         TEXT NOT NULL,
    key           TEXT NOT NULL,
    request_hash  TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    PRIMARY KEY (tenant_id, scope, key)
);
CREATE INDEX idempotency_records_expires ON idempotency_records (expires_at);

-- +goose Down

DROP TABLE idempotency_records;
DROP TABLE audit_events;
DROP TABLE payments;
DROP TABLE order_status_events;
DROP TABLE order_item_options;
DROP TABLE order_items;
DROP TABLE orders;
DROP TABLE order_number_counters;
DROP TABLE dining_locations;
DROP TABLE options;
DROP TABLE option_groups;
DROP TABLE menu_items;
DROP TABLE menu_categories;
DROP TABLE assets;
DROP TABLE sessions;
DROP TABLE tenant_domains;
DROP TABLE restaurant_locations;
DROP TABLE tenant_memberships;
DROP TABLE users;
DROP TABLE tenants;
