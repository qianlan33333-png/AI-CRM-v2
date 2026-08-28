-- +goose Up
-- HXC-owned immutable history. No current sender, task, identity, or Provider links.
CREATE TABLE hxc_v1_sender_config_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  private_digest BYTEA NOT NULL CHECK (octet_length(private_digest) = 32),
  priority BIGINT NOT NULL,
  original_is_active BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE hxc_v1_send_record_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  private_digest BYTEA NOT NULL CHECK (octet_length(private_digest) = 32),
  task_type TEXT NOT NULL,
  original_status TEXT NOT NULL,
  selected_count BIGINT NOT NULL,
  eligible_count BIGINT NOT NULL,
  sent_count BIGINT NOT NULL,
  skipped_count BIGINT NOT NULL,
  planned_count BIGINT NOT NULL,
  queued_count BIGINT NOT NULL,
  dispatching_count BIGINT NOT NULL,
  succeeded_count BIGINT NOT NULL,
  failed_count BIGINT NOT NULL,
  blocked_count BIGINT NOT NULL,
  cancelled_count BIGINT NOT NULL,
  image_count BIGINT NOT NULL,
  include_do_not_disturb BOOLEAN NOT NULL,
  target_source TEXT NOT NULL,
  target_source_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL,
  last_status_sync_at TIMESTAMPTZ,
  last_refreshed_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE hxc_v1_send_record_history;
DROP TABLE hxc_v1_sender_config_history;
