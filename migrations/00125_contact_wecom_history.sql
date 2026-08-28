-- +goose Up
-- V1 historical facts only: no live identity/owner assignment or callback replay.
CREATE TABLE contact_v1_wecom_event_log_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  corp_id_digest BYTEA NOT NULL CHECK (octet_length(corp_id_digest) = 32),
  event_type TEXT NOT NULL,
  change_type TEXT NOT NULL,
  external_user_id_digest BYTEA NOT NULL CHECK (octet_length(external_user_id_digest) = 32),
  user_id_digest BYTEA NOT NULL CHECK (octet_length(user_id_digest) = 32),
  event_time BIGINT,
  event_key_digest BYTEA NOT NULL CHECK (octet_length(event_key_digest) = 32),
  payload_xml_digest BYTEA NOT NULL CHECK (octet_length(payload_xml_digest) = 32),
  payload_json_digest BYTEA NOT NULL CHECK (octet_length(payload_json_digest) = 32),
  process_status TEXT NOT NULL,
  retry_count INTEGER NOT NULL,
  error_message_digest BYTEA NOT NULL CHECK (octet_length(error_message_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  identity_sync_status TEXT NOT NULL,
  identity_sync_error_code_digest BYTEA NOT NULL CHECK (octet_length(identity_sync_error_code_digest) = 32),
  identity_sync_error_message_digest BYTEA NOT NULL CHECK (octet_length(identity_sync_error_message_digest) = 32),
  identity_sync_response_digest BYTEA NOT NULL CHECK (octet_length(identity_sync_response_digest) = 32)
);

CREATE TABLE contact_v1_wecom_follow_user_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  corp_id_digest BYTEA NOT NULL CHECK (octet_length(corp_id_digest) = 32),
  external_user_id_digest BYTEA NOT NULL CHECK (octet_length(external_user_id_digest) = 32),
  user_id_digest BYTEA NOT NULL CHECK (octet_length(user_id_digest) = 32),
  relation_status TEXT NOT NULL,
  is_primary BOOLEAN NOT NULL,
  remark_digest BYTEA NOT NULL CHECK (octet_length(remark_digest) = 32),
  description_digest BYTEA NOT NULL CHECK (octet_length(description_digest) = 32),
  add_way INTEGER,
  state TEXT NOT NULL,
  oper_user_id_digest BYTEA NOT NULL CHECK (octet_length(oper_user_id_digest) = 32),
  create_time BIGINT,
  raw_follow_user_digest BYTEA NOT NULL CHECK (octet_length(raw_follow_user_digest) = 32),
  first_seen_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE contact_v1_wecom_follow_user_history;
DROP TABLE contact_v1_wecom_event_log_history;
