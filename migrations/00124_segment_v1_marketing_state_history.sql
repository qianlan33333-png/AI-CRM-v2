-- +goose Up

CREATE TABLE segment_v1_marketing_state_snapshots (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  person_source_id BIGINT,
  external_userid_digest BYTEA NOT NULL CHECK (octet_length(external_userid_digest) = 32),
  automation_key TEXT NOT NULL,
  main_stage TEXT NOT NULL,
  sub_stage TEXT NOT NULL,
  activated BOOLEAN NOT NULL,
  converted BOOLEAN NOT NULL,
  eligible_for_conversion BOOLEAN NOT NULL,
  lifecycle_status TEXT NOT NULL,
  last_activation_at TEXT NOT NULL,
  last_conversion_marked_at TEXT NOT NULL,
  last_message_at TEXT NOT NULL,
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

CREATE TABLE segment_v1_marketing_state_changes (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  person_source_id BIGINT,
  batch_source_id BIGINT,
  external_userid_digest BYTEA NOT NULL CHECK (octet_length(external_userid_digest) = 32),
  automation_key TEXT NOT NULL,
  main_stage TEXT NOT NULL,
  sub_stage TEXT NOT NULL,
  activated BOOLEAN NOT NULL,
  converted BOOLEAN NOT NULL,
  eligible_for_conversion BOOLEAN NOT NULL,
  lifecycle_status TEXT NOT NULL,
  last_activation_at TEXT NOT NULL,
  last_conversion_marked_at TEXT NOT NULL,
  last_message_at TEXT NOT NULL,
  exit_reason TEXT NOT NULL,
  change_reason TEXT NOT NULL,
  state_payload_digest BYTEA NOT NULL CHECK (octet_length(state_payload_digest) = 32),
  recorded_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE segment_v1_value_segment_snapshots (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  external_userid_digest BYTEA NOT NULL CHECK (octet_length(external_userid_digest) = 32),
  segment TEXT NOT NULL,
  segment_rank INTEGER NOT NULL,
  score INTEGER NOT NULL,
  scoring_version TEXT NOT NULL,
  submission_source_id BIGINT,
  matched_question_ids_digest BYTEA NOT NULL CHECK (octet_length(matched_question_ids_digest) = 32),
  state_payload_digest BYTEA NOT NULL CHECK (octet_length(state_payload_digest) = 32),
  computed_reason TEXT NOT NULL,
  evaluated_at TIMESTAMPTZ NOT NULL,
  computed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE segment_v1_value_segment_changes (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  external_userid_digest BYTEA NOT NULL CHECK (octet_length(external_userid_digest) = 32),
  segment TEXT NOT NULL,
  segment_rank INTEGER NOT NULL,
  score INTEGER NOT NULL,
  scoring_version TEXT NOT NULL,
  submission_source_id BIGINT,
  matched_question_ids_digest BYTEA NOT NULL CHECK (octet_length(matched_question_ids_digest) = 32),
  state_payload_digest BYTEA NOT NULL CHECK (octet_length(state_payload_digest) = 32),
  change_reason TEXT NOT NULL,
  evaluated_at TIMESTAMPTZ NOT NULL,
  recorded_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE segment_v1_value_segment_changes;
DROP TABLE segment_v1_value_segment_snapshots;
DROP TABLE segment_v1_marketing_state_changes;
DROP TABLE segment_v1_marketing_state_snapshots;
