-- +goose Up
-- HXC-owned observations only; no executable job, current identity, or Provider link.
-- JSON is stored as source text so typed replay preserves its exact bytes.
CREATE TABLE hxc_v1_chat_job_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  queue_source_id BIGINT,
  member_source_id BIGINT,
  external_contact_id TEXT NOT NULL,
  phone TEXT NOT NULL,
  external_message_id TEXT NOT NULL,
  external_session_id TEXT NOT NULL,
  laohuang_task_id TEXT NOT NULL,
  request_payload_json TEXT NOT NULL,
  accepted_payload_json TEXT NOT NULL,
  callback_payload_json TEXT NOT NULL,
  original_status TEXT NOT NULL,
  reply_text TEXT NOT NULL,
  error_code TEXT NOT NULL,
  error_message TEXT NOT NULL,
  send_channel TEXT NOT NULL,
  send_record_source_id BIGINT,
  send_result_json TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  finished_at_source TEXT NOT NULL
);

-- +goose Down
DROP TABLE hxc_v1_chat_job_history;

