-- Option groups and options removed from an item are archived, not deleted:
-- past orders may reference them (order_item_options.option_id).

-- +goose Up
ALTER TABLE option_groups ADD COLUMN archived_at TEXT;
ALTER TABLE options ADD COLUMN archived_at TEXT;

-- +goose Down
ALTER TABLE options DROP COLUMN archived_at;
ALTER TABLE option_groups DROP COLUMN archived_at;
