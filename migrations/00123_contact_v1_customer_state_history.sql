-- +goose Up

CREATE TABLE contact_v1_customer_status_snapshots (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  signup_status TEXT NOT NULL,
  signup_label_name TEXT NOT NULL,
  customer_name_snapshot TEXT NOT NULL,
  owner_userid_snapshot TEXT NOT NULL,
  set_by_userid_digest BYTEA NOT NULL CHECK (octet_length(set_by_userid_digest) = 32),
  set_at TIMESTAMPTZ NOT NULL,
  wecom_tag_sync_status TEXT NOT NULL,
  wecom_tag_sync_error_hash BYTEA NOT NULL CHECK (octet_length(wecom_tag_sync_error_hash) = 32),
  status_flags_digest BYTEA NOT NULL CHECK (octet_length(status_flags_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  unionid TEXT NOT NULL
);

CREATE TABLE contact_v1_customer_status_changes (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  old_signup_status TEXT NOT NULL,
  new_signup_status TEXT NOT NULL,
  old_label_name TEXT NOT NULL,
  new_label_name TEXT NOT NULL,
  customer_name_snapshot TEXT NOT NULL,
  owner_userid_snapshot TEXT NOT NULL,
  set_by_userid_digest BYTEA NOT NULL CHECK (octet_length(set_by_userid_digest) = 32),
  set_at TIMESTAMPTZ NOT NULL,
  wecom_tag_sync_status TEXT NOT NULL,
  wecom_tag_sync_error_hash BYTEA NOT NULL CHECK (octet_length(wecom_tag_sync_error_hash) = 32),
  status_flags_digest BYTEA NOT NULL CHECK (octet_length(status_flags_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL,
  unionid TEXT NOT NULL
);

CREATE TABLE contact_v1_class_term_tag_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  tag_group_name TEXT NOT NULL,
  tag_name TEXT NOT NULL,
  class_term_no INTEGER NOT NULL,
  class_term_label TEXT NOT NULL,
  original_active BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  strategy_source_id TEXT NOT NULL,
  group_source_id TEXT NOT NULL,
  tag_source_id TEXT NOT NULL
);

-- +goose Down
DROP TABLE contact_v1_class_term_tag_history;
DROP TABLE contact_v1_customer_status_changes;
DROP TABLE contact_v1_customer_status_snapshots;
