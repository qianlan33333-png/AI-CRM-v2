-- +goose Up
-- Immutable V1 timeline observations. This table never feeds customer_events.
CREATE TABLE contact_v1_customer_timeline_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_time TIMESTAMPTZ NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  source_table TEXT NOT NULL,
  source_value TEXT NOT NULL,
  metadata_json TEXT NOT NULL CHECK (metadata_json::jsonb IS NOT NULL),
  created_at TIMESTAMPTZ NOT NULL,
  unionid TEXT NOT NULL,
  customer_id BIGINT REFERENCES customers(id),
  UNIQUE (source_id)
);
CREATE INDEX contact_v1_customer_timeline_history_event_time
  ON contact_v1_customer_timeline_history (event_time DESC, id DESC);
CREATE INDEX contact_v1_customer_timeline_history_customer
  ON contact_v1_customer_timeline_history (customer_id, event_time DESC, id DESC)
  WHERE customer_id IS NOT NULL;

-- +goose Down
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM contact_v1_customer_timeline_history) THEN
    RAISE EXCEPTION 'Customer timeline history requires snapshot restore, not destructive down migration';
  END IF;
END $$;
DROP TABLE contact_v1_customer_timeline_history;
