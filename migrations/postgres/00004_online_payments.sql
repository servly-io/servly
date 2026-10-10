-- Online payments through providers (ADR 0007, ADR 0008): the restaurant's
-- provider account, hosted checkouts, provider events for deduplication, and
-- provider columns on payments.

-- +goose Up
CREATE TABLE payment_accounts (
    id            uuid PRIMARY KEY,
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    location_id   uuid NOT NULL,
    provider      text NOT NULL,
    mode          text NOT NULL CHECK (mode IN ('test', 'live')),
    status        text NOT NULL CHECK (status IN ('active', 'disabled')),
    account_ref   text NOT NULL DEFAULT '',
    account_name  text NOT NULL DEFAULT '',
    country       text NOT NULL DEFAULT '',
    table_flow    text NOT NULL CHECK (table_flow IN ('after', 'before', 'staff')),
    pickup_flow   text NOT NULL CHECK (pickup_flow IN ('before', 'either', 'counter')),
    credentials   text NOT NULL DEFAULT '',  -- sealed with the operator's secret key
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, location_id),
    FOREIGN KEY (tenant_id, location_id) REFERENCES restaurant_locations (tenant_id, id)
);

CREATE TABLE payment_checkouts (
    id                  uuid PRIMARY KEY,
    tenant_id           uuid NOT NULL,
    location_id         uuid NOT NULL,
    account_id          uuid NOT NULL,
    public_token        text NOT NULL UNIQUE,
    kind                text NOT NULL CHECK (kind IN ('bill', 'prepay')),
    order_id            uuid,
    dining_location_id  uuid,
    idempotency_key     text,
    draft               text NOT NULL DEFAULT '',  -- prepay: the priced order, as JSON
    provider            text NOT NULL,
    provider_ref        text NOT NULL DEFAULT '',
    redirect_url        text NOT NULL DEFAULT '',
    amount_minor        bigint NOT NULL CHECK (amount_minor > 0),
    currency            text NOT NULL CHECK (char_length(currency) = 3),
    status              text NOT NULL CHECK (status IN ('open', 'paid', 'failed', 'expired')),
    attempts            integer NOT NULL DEFAULT 0,
    next_check_at       timestamptz NOT NULL,
    expires_at          timestamptz NOT NULL,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, idempotency_key),
    FOREIGN KEY (tenant_id, account_id) REFERENCES payment_accounts (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id)
);
CREATE INDEX payment_checkouts_due ON payment_checkouts (status, next_check_at);
CREATE INDEX payment_checkouts_ref ON payment_checkouts (account_id, provider_ref);
CREATE INDEX payment_checkouts_order ON payment_checkouts (tenant_id, order_id);

CREATE TABLE payment_events (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL REFERENCES tenants (id),
    account_id   uuid NOT NULL,
    provider     text NOT NULL,
    event_ref    text NOT NULL,
    type         text NOT NULL,
    received_at  timestamptz NOT NULL,
    UNIQUE (provider, event_ref)
);

ALTER TABLE payments DROP CONSTRAINT payments_method_check;
ALTER TABLE payments ADD CONSTRAINT payments_method_check CHECK (method IN ('cash', 'external_terminal', 'other', 'online'));
ALTER TABLE payments ADD COLUMN provider text NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN provider_ref text NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN checkout_id uuid REFERENCES payment_checkouts (id);
ALTER TABLE payments ADD COLUMN detail text NOT NULL DEFAULT '';
-- One payment per provider payment and per checkout, whatever delivers the result.
CREATE UNIQUE INDEX payments_provider_ref ON payments (provider, provider_ref) WHERE provider_ref <> '';
CREATE UNIQUE INDEX payments_checkout ON payments (checkout_id) WHERE checkout_id IS NOT NULL;

-- +goose Down
DROP INDEX payments_checkout;
DROP INDEX payments_provider_ref;
-- Online payments cannot be represented without their method; they are dropped.
DELETE FROM payments WHERE method = 'online';
ALTER TABLE payments DROP COLUMN detail;
ALTER TABLE payments DROP COLUMN checkout_id;
ALTER TABLE payments DROP COLUMN provider_ref;
ALTER TABLE payments DROP COLUMN provider;
ALTER TABLE payments DROP CONSTRAINT payments_method_check;
ALTER TABLE payments ADD CONSTRAINT payments_method_check CHECK (method IN ('cash', 'external_terminal', 'other'));
DROP TABLE payment_events;
DROP TABLE payment_checkouts;
DROP TABLE payment_accounts;
