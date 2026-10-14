-- A restaurant-wide switch for taking guest orders.

-- +goose Up
ALTER TABLE restaurant_locations ADD COLUMN accepting_orders boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE restaurant_locations DROP COLUMN accepting_orders;
