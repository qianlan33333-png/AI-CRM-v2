-- +goose Up

CREATE TABLE segment_v1_legacy_marketing_states (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  external_userid_digest BYTEA NOT NULL CHECK (octet_length(external_userid_digest) = 32),
  scenario_key TEXT NOT NULL,
  marketing_phase TEXT NOT NULL,
  phase_label TEXT NOT NULL,
  phase_reason TEXT NOT NULL,
  lifecycle_status TEXT NOT NULL,
  last_batch_source_id BIGINT,
  last_batch_status TEXT NOT NULL,
  last_batch_window_start TEXT NOT NULL,
  last_batch_window_end TEXT NOT NULL,
  last_trigger_message_at TEXT NOT NULL,
  entered_at TIMESTAMPTZ,
  exited_at TIMESTAMPTZ,
  exit_reason TEXT NOT NULL,
  state_payload_digest BYTEA NOT NULL CHECK (octet_length(state_payload_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE segment_v1_legacy_marketing_values (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  external_userid_digest BYTEA NOT NULL CHECK (octet_length(external_userid_digest) = 32),
  scenario_key TEXT NOT NULL,
  value_segment TEXT NOT NULL,
  segment_label TEXT NOT NULL,
  score BIGINT NOT NULL,
  score_breakdown_digest BYTEA NOT NULL CHECK (octet_length(score_breakdown_digest) = 32),
  state_payload_digest BYTEA NOT NULL CHECK (octet_length(state_payload_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE segment_v1_legacy_marketing_values;
DROP TABLE segment_v1_legacy_marketing_states;
