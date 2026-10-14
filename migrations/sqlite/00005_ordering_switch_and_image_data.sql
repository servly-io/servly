-- A restaurant-wide switch for taking guest orders.

-- +goose Up
ALTER TABLE restaurant_locations ADD COLUMN accepting_orders INTEGER NOT NULL DEFAULT 1 CHECK (accepting_orders IN (0, 1));

-- +goose Down
ALTER TABLE restaurant_locations DROP COLUMN accepting_orders;
