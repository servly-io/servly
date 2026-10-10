-- Online payments through providers (ADR 0007, ADR 0008): the restaurant's
-- provider account, hosted checkouts, provider events for deduplication, and
-- provider columns on payments.

-- +goose Up
CREATE TABLE payment_accounts (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants (id),
    location_id   TEXT NOT NULL,
    provider      TEXT NOT NULL,
    mode          TEXT NOT NULL CHECK (mode IN ('test', 'live')),
    status        TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    account_ref   TEXT NOT NULL DEFAULT '',
    account_name  TEXT NOT NULL DEFAULT '',
    country       TEXT NOT NULL DEFAULT '',
    table_flow    TEXT NOT NULL CHECK (table_flow IN ('after', 'before', 'staff')),
    pickup_flow   TEXT NOT NULL CHECK (pickup_flow IN ('before', 'either', 'counter')),
    credentials   TEXT NOT NULL DEFAULT '',  -- sealed with the operator's secret key
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, location_id),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);

CREATE TABLE payment_checkouts (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    location_id         TEXT NOT NULL,
    account_id          TEXT NOT NULL,
    public_token        TEXT NOT NULL UNIQUE,
    kind                TEXT NOT NULL CHECK (kind IN ('bill', 'prepay')),
    order_id            TEXT,
    dining_location_id  TEXT,
    idempotency_key     TEXT,
    draft               TEXT NOT NULL DEFAULT '',  -- prepay: the priced order, as JSON
    provider            TEXT NOT NULL,
    provider_ref        TEXT NOT NULL DEFAULT '',
    redirect_url        TEXT NOT NULL DEFAULT '',
    amount_minor        INTEGER NOT NULL CHECK (amount_minor > 0),
    currency            TEXT NOT NULL CHECK (length(currency) = 3),
    status              TEXT NOT NULL CHECK (status IN ('open', 'paid', 'failed', 'expired')),
    attempts            INTEGER NOT NULL DEFAULT 0,
    next_check_at       TEXT NOT NULL,
    expires_at          TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, idempotency_key),
    FOREIGN KEY (tenant_id, account_id) REFERENCES payment_accounts (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);
CREATE INDEX payment_checkouts_due ON payment_checkouts (status, next_check_at);
CREATE INDEX payment_checkouts_ref ON payment_checkouts (account_id, provider_ref);
CREATE INDEX payment_checkouts_order ON payment_checkouts (tenant_id, order_id);

CREATE TABLE payment_events (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants (id),
    account_id   TEXT NOT NULL,
    provider     TEXT NOT NULL,
    event_ref    TEXT NOT NULL,
    type         TEXT NOT NULL,
    received_at  TEXT NOT NULL,
    UNIQUE (provider, event_ref)
);

-- SQLite cannot change a CHECK constraint in place: rebuild payments, which
-- no other table references.
CREATE TABLE payments_new (
    id                   TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL,
    order_id             TEXT NOT NULL,
    method               TEXT NOT NULL CHECK (method IN ('cash', 'external_terminal', 'other', 'online')),
    status               TEXT NOT NULL CHECK (status IN ('pending', 'paid', 'failed', 'partially_refunded', 'refunded')),
    amount_minor         INTEGER NOT NULL CHECK (amount_minor > 0),
    refunded_minor       INTEGER NOT NULL DEFAULT 0 CHECK (refunded_minor >= 0 AND refunded_minor <= amount_minor),
    currency             TEXT NOT NULL CHECK (length(currency) = 3),
    reference            TEXT NOT NULL DEFAULT '',
    recorded_by_user_id  TEXT REFERENCES users (id),
    provider             TEXT NOT NULL DEFAULT '',
    provider_ref         TEXT NOT NULL DEFAULT '',
    checkout_id          TEXT REFERENCES payment_checkouts (id),
    detail               TEXT NOT NULL DEFAULT '',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);
INSERT INTO payments_new (id, tenant_id, order_id, method, status, amount_minor, refunded_minor, currency, reference,
                          recorded_by_user_id, created_at, updated_at)
SELECT id, tenant_id, order_id, method, status, amount_minor, refunded_minor, currency, reference,
       recorded_by_user_id, created_at, updated_at FROM payments;
DROP TABLE payments;
ALTER TABLE payments_new RENAME TO payments;
CREATE INDEX payments_order ON payments (tenant_id, order_id);
-- One payment per provider payment and per checkout, whatever delivers the result.
CREATE UNIQUE INDEX payments_provider_ref ON payments (provider, provider_ref) WHERE provider_ref <> '';
CREATE UNIQUE INDEX payments_checkout ON payments (checkout_id) WHERE checkout_id IS NOT NULL;

-- +goose Down
CREATE TABLE payments_old (
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
-- Online payments cannot be represented without their method; they are dropped.
INSERT INTO payments_old SELECT id, tenant_id, order_id, method, status, amount_minor, refunded_minor, currency, reference,
       recorded_by_user_id, created_at, updated_at FROM payments WHERE method <> 'online';
DROP TABLE payments;
ALTER TABLE payments_old RENAME TO payments;
CREATE INDEX payments_order ON payments (tenant_id, order_id);
DROP TABLE payment_events;
DROP TABLE payment_checkouts;
DROP TABLE payment_accounts;
