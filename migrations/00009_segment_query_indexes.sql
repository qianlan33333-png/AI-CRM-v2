-- +goose Up
CREATE INDEX idx_customers_segment_stage
  ON customers (stage_id, id);
CREATE INDEX idx_customers_segment_owner
  ON customers (owner_staff_id, id);
CREATE INDEX idx_customers_segment_channel
  ON customers (channel_id, id);
CREATE INDEX idx_customers_segment_added
  ON customers (added_at, id);
CREATE INDEX idx_customers_segment_interact
  ON customers (last_interact_at, id);
CREATE INDEX idx_customers_segment_deleted
  ON customers (is_deleted, id);

-- +goose Down
DROP INDEX idx_customers_segment_deleted;
DROP INDEX idx_customers_segment_interact;
DROP INDEX idx_customers_segment_added;
DROP INDEX idx_customers_segment_channel;
DROP INDEX idx_customers_segment_owner;
DROP INDEX idx_customers_segment_stage;
