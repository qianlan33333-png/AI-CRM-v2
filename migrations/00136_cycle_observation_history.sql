-- +goose Up
-- Immutable source observations, not current runs, actions or subscriptions.
CREATE TABLE operation_cycle_v1_metric_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  run_source_id BIGINT NOT NULL,
  metric_key TEXT NOT NULL,
  label TEXT NOT NULL,
  numerator DOUBLE PRECISION,
  denominator DOUBLE PRECISION,
  value DOUBLE PRECISION,
  unit TEXT NOT NULL,
  observation_window TEXT NOT NULL,
  data_source TEXT NOT NULL,
  data_quality TEXT NOT NULL,
  -- Preserve the frozen JSON representation, including literal JSON null.
  limitations_json TEXT NOT NULL CHECK (limitations_json::jsonb IS NOT NULL),
  is_causal BOOLEAN NOT NULL,
  value_status TEXT NOT NULL,
  last_snapshot_source_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE operation_cycle_v1_reference_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  run_source_id BIGINT NOT NULL,
  reference_key TEXT NOT NULL,
  reference_type TEXT NOT NULL,
  label TEXT NOT NULL,
  source_system TEXT NOT NULL,
  reference_source_id TEXT NOT NULL,
  href TEXT NOT NULL,
  evidence_hash TEXT NOT NULL,
  data_status TEXT NOT NULL,
  last_snapshot_source_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE operation_cycle_v1_reference_history;
DROP TABLE operation_cycle_v1_metric_history;
