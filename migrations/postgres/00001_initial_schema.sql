-- Initial schema, PostgreSQL dialect. Contract: docs/docs/domain-model.md.
--
-- Conventions (PostgreSQL):
--   ids          uuid
--   timestamps   timestamptz
--   booleans     boolean
--   money        bigint minor units; currency is text ISO 4217
--   json         jsonb
-- The SQLite migration of the same number must describe the same schema.

-- +goose Up

CREATE TABLE tenants (
    id          uuid PRIMARY KEY,
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    status      text NOT NULL CHECK (status IN ('active', 'suspended')),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);

CREATE TABLE users (
    id                 uuid PRIMARY KEY,
    email              text NOT NULL UNIQUE,
    password_hash      text NOT NULL,
    display_name       text NOT NULL,
    is_platform_admin  boolean NOT NULL DEFAULT false,
    status             text NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL
);

CREATE TABLE tenant_memberships (
    id          uuid PRIMARY KEY,
    tenant_id   uuid NOT NULL REFERENCES tenants (id),
    user_id     uuid NOT NULL REFERENCES users (id),
    role        text NOT NULL CHECK (role IN ('owner', 'manager', 'staff')),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX tenant_memberships_user ON tenant_memberships (user_id);

CREATE TABLE restaurant_locations (
    id                     uuid PRIMARY KEY,
    tenant_id              uuid NOT NULL REFERENCES tenants (id),
    name                   text NOT NULL,
    currency               text NOT NULL CHECK (char_length(currency) = 3),
    timezone               text NOT NULL,
    order_notes_enabled    boolean NOT NULL DEFAULT true,
    public_status_enabled  boolean NOT NULL DEFAULT true,
    status                 text NOT NULL CHECK (status IN ('active', 'closed')),
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL,
    UNIQUE (tenant_id, id)
);

CREATE TABLE tenant_domains (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL REFERENCES tenants (id),
    location_id  uuid,
    hostname     text NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL,
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);
CREATE INDEX tenant_domains_tenant ON tenant_domains (tenant_id);

CREATE TABLE sessions (
    token_hash        text PRIMARY KEY,
    user_id           uuid NOT NULL REFERENCES users (id),
    active_tenant_id  uuid REFERENCES tenants (id),
    csrf_token        text NOT NULL,
    created_at        timestamptz NOT NULL,
    last_seen_at      timestamptz NOT NULL,
    expires_at        timestamptz NOT NULL
);
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE assets (
    id            uuid PRIMARY KEY,
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    storage_key   text NOT NULL UNIQUE,
    content_type  text NOT NULL,
    size_bytes    bigint NOT NULL CHECK (size_bytes >= 0),
    sha256        text NOT NULL,
    created_at    timestamptz NOT NULL,
    UNIQUE (tenant_id, id)
);

CREATE TABLE menu_categories (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL,
    location_id  uuid NOT NULL,
    name         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    position     integer NOT NULL,
    is_enabled   boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);
CREATE INDEX menu_categories_location ON menu_categories (tenant_id, location_id, position);

CREATE TABLE menu_items (
    id              uuid PRIMARY KEY,
    tenant_id       uuid NOT NULL,
    category_id     uuid NOT NULL,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    price_minor     bigint NOT NULL CHECK (price_minor >= 0),
    image_asset_id  uuid,
    position        integer NOT NULL,
    is_enabled      boolean NOT NULL DEFAULT true,
    is_available    boolean NOT NULL DEFAULT true,
    archived_at     timestamptz,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, category_id) REFERENCES menu_categories (tenant_id, id),
    FOREIGN KEY (tenant_id, image_asset_id) REFERENCES assets (tenant_id, id)
);
CREATE INDEX menu_items_category ON menu_items (tenant_id, category_id, position);

CREATE TABLE option_groups (
    id          uuid PRIMARY KEY,
    tenant_id   uuid NOT NULL,
    item_id     uuid NOT NULL,
    name        text NOT NULL,
    min_select  integer NOT NULL DEFAULT 0 CHECK (min_select >= 0),
    max_select  integer NOT NULL CHECK (max_select >= 1 AND max_select >= min_select),
    position    integer NOT NULL,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, item_id) REFERENCES menu_items (tenant_id, id)
);
CREATE INDEX option_groups_item ON option_groups (tenant_id, item_id, position);

CREATE TABLE options (
    id                 uuid PRIMARY KEY,
    tenant_id          uuid NOT NULL,
    option_group_id    uuid NOT NULL,
    name               text NOT NULL,
    price_delta_minor  bigint NOT NULL DEFAULT 0,
    position           integer NOT NULL,
    is_available       boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, option_group_id) REFERENCES option_groups (tenant_id, id)
);
CREATE INDEX options_group ON options (tenant_id, option_group_id, position);

CREATE TABLE dining_locations (
    id            uuid PRIMARY KEY,
    tenant_id     uuid NOT NULL,
    location_id   uuid NOT NULL,
    name          text NOT NULL,
    kind          text NOT NULL CHECK (kind IN ('table', 'pickup')),
    public_token  text NOT NULL UNIQUE,
    is_enabled    boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);
CREATE INDEX dining_locations_location ON dining_locations (tenant_id, location_id);

CREATE TABLE order_number_counters (
    tenant_id      uuid NOT NULL,
    location_id    uuid NOT NULL,
    business_date  text NOT NULL CHECK (business_date ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'),
    last_number    integer NOT NULL,
    PRIMARY KEY (tenant_id, location_id, business_date),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);

CREATE TABLE orders (
    id                  uuid PRIMARY KEY,
    tenant_id           uuid NOT NULL,
    location_id         uuid NOT NULL,
    dining_location_id  uuid,
    order_number        integer NOT NULL CHECK (order_number > 0),
    business_date       text NOT NULL CHECK (business_date ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'),
    public_token        text NOT NULL UNIQUE,
    mode                text NOT NULL CHECK (mode IN ('dine_in', 'pickup')),
    status              text NOT NULL CHECK (status IN ('new', 'accepted', 'preparing', 'ready', 'completed', 'cancelled')),
    payment_status      text NOT NULL CHECK (payment_status IN ('unpaid', 'pending', 'paid', 'partially_refunded', 'refunded')),
    currency            text NOT NULL CHECK (char_length(currency) = 3),
    subtotal_minor      bigint NOT NULL CHECK (subtotal_minor >= 0),
    total_minor         bigint NOT NULL CHECK (total_minor >= 0),
    notes               text NOT NULL DEFAULT '',
    version             bigint NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, location_id, business_date, order_number),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id),
    FOREIGN KEY (tenant_id, dining_location_id) REFERENCES dining_locations (tenant_id, id)
);
CREATE INDEX orders_board ON orders (tenant_id, location_id, status, created_at);
CREATE INDEX orders_created ON orders (tenant_id, location_id, created_at);

CREATE TABLE order_items (
    id                   uuid PRIMARY KEY,
    tenant_id            uuid NOT NULL,
    order_id             uuid NOT NULL,
    menu_item_id         uuid,
    position             integer NOT NULL,
    name                 text NOT NULL,
    quantity             integer NOT NULL CHECK (quantity > 0),
    unit_price_minor     bigint NOT NULL CHECK (unit_price_minor >= 0),
    options_total_minor  bigint NOT NULL,
    line_total_minor     bigint NOT NULL CHECK (line_total_minor >= 0),
    notes                text NOT NULL DEFAULT '',
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id),
    FOREIGN KEY (tenant_id, menu_item_id) REFERENCES menu_items (tenant_id, id)
);
CREATE INDEX order_items_order ON order_items (tenant_id, order_id, position);

CREATE TABLE order_item_options (
    id                 uuid PRIMARY KEY,
    tenant_id          uuid NOT NULL,
    order_item_id      uuid NOT NULL,
    option_id          uuid,
    group_name         text NOT NULL,
    option_name        text NOT NULL,
    price_delta_minor  bigint NOT NULL,
    FOREIGN KEY (tenant_id, order_item_id) REFERENCES order_items (tenant_id, id),
    FOREIGN KEY (tenant_id, option_id) REFERENCES options (tenant_id, id)
);
CREATE INDEX order_item_options_item ON order_item_options (tenant_id, order_item_id);

CREATE TABLE order_status_events (
    id             uuid PRIMARY KEY,
    tenant_id      uuid NOT NULL,
    order_id       uuid NOT NULL,
    from_status    text,
    to_status      text NOT NULL,
    actor_user_id  uuid REFERENCES users (id),
    reason         text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL,
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);
CREATE INDEX order_status_events_order ON order_status_events (tenant_id, order_id, created_at);

CREATE TABLE payments (
    id                   uuid PRIMARY KEY,
    tenant_id            uuid NOT NULL,
    order_id             uuid NOT NULL,
    method               text NOT NULL CHECK (method IN ('cash', 'external_terminal', 'other')),
    status               text NOT NULL CHECK (status IN ('pending', 'paid', 'failed', 'partially_refunded', 'refunded')),
    amount_minor         bigint NOT NULL CHECK (amount_minor > 0),
    refunded_minor       bigint NOT NULL DEFAULT 0 CHECK (refunded_minor >= 0 AND refunded_minor <= amount_minor),
    currency             text NOT NULL CHECK (char_length(currency) = 3),
    reference            text NOT NULL DEFAULT '',
    recorded_by_user_id  uuid REFERENCES users (id),
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);
CREATE INDEX payments_order ON payments (tenant_id, order_id);

CREATE TABLE audit_events (
    id             uuid PRIMARY KEY,
    tenant_id      uuid REFERENCES tenants (id),
    actor_user_id  uuid REFERENCES users (id),
    action         text NOT NULL,
    entity_type    text NOT NULL,
    entity_id      uuid NOT NULL,
    data           jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL
);
CREATE INDEX audit_events_tenant ON audit_events (tenant_id, created_at);
CREATE INDEX audit_events_entity ON audit_events (tenant_id, entity_type, entity_id);

CREATE TABLE idempotency_records (
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    scope         text NOT NULL,
    key           text NOT NULL,
    request_hash  text NOT NULL,
    resource_id   uuid NOT NULL,
    created_at    timestamptz NOT NULL,
    expires_at    timestamptz NOT NULL,
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
