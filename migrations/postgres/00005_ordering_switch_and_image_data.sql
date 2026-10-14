-- A restaurant-wide switch for taking guest orders, and uploaded images kept
-- in the database (ADR 0009), so every instance serves them.

-- +goose Up
ALTER TABLE restaurant_locations ADD COLUMN accepting_orders boolean NOT NULL DEFAULT true;
-- NULL only for images still on disk from before; servly copies them in at start.
ALTER TABLE assets ADD COLUMN data bytea;

-- +goose Down
ALTER TABLE assets DROP COLUMN data;
ALTER TABLE restaurant_locations DROP COLUMN accepting_orders;
