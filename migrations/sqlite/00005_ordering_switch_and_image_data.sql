-- A restaurant-wide switch for taking guest orders, and uploaded images kept
-- in the database (ADR 0009), so every instance serves them.

-- +goose Up
ALTER TABLE restaurant_locations ADD COLUMN accepting_orders INTEGER NOT NULL DEFAULT 1 CHECK (accepting_orders IN (0, 1));
-- NULL only for images still on disk from before; servly copies them in at start.
ALTER TABLE assets ADD COLUMN data BLOB;

-- +goose Down
ALTER TABLE assets DROP COLUMN data;
ALTER TABLE restaurant_locations DROP COLUMN accepting_orders;
