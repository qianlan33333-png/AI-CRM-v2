-- +goose Up
-- Source snapshots are separate from current views, usage and access grants.
CREATE TABLE product_v1_member_view_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_view_id BIGINT NOT NULL CHECK (source_view_id > 0),
  source_service_product_id BIGINT NOT NULL CHECK (source_service_product_id > 0),
  product_id BIGINT REFERENCES products(id),
  name TEXT NOT NULL,
  position BIGINT NOT NULL,
  is_default BOOLEAN NOT NULL,
  schema_version SMALLINT NOT NULL,
  config_digest BYTEA NOT NULL CHECK (octet_length(config_digest) = 32),
  version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32)
);
CREATE INDEX product_v1_member_view_history_product_idx ON product_v1_member_view_history (product_id, id);

CREATE TABLE product_v1_member_usage_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  customer_id BIGINT CHECK (customer_id > 0),
  formally_logged_in BOOLEAN NOT NULL,
  has_token_usage BOOLEAN NOT NULL,
  learning_plan_id TEXT NOT NULL,
  learning_plan_current BIGINT CHECK (learning_plan_current >= 0),
  learning_plan_total BIGINT CHECK (learning_plan_total >= 0),
  open_count_7d BIGINT NOT NULL CHECK (open_count_7d >= 0),
  last_open_at TIMESTAMPTZ,
  refreshed_at TIMESTAMPTZ NOT NULL,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  recovery_entry_digest BYTEA NOT NULL CHECK (octet_length(recovery_entry_digest) = 32)
);
CREATE INDEX product_v1_member_usage_history_customer_idx ON product_v1_member_usage_history (customer_id, id);

-- +goose Down
DROP TABLE product_v1_member_usage_history;
DROP TABLE product_v1_member_view_history;
