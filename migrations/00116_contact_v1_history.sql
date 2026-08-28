-- +goose Up
-- V1 snapshots are deliberately separate from live profiles and owner commands.
CREATE TABLE contact_v1_sidebar_profile_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  customer_id BIGINT CHECK (customer_id > 0),
  source TEXT NOT NULL,
  industry TEXT NOT NULL,
  industry_description TEXT NOT NULL,
  needs_blockers_followup TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32)
);
CREATE INDEX contact_v1_sidebar_profile_history_customer_idx ON contact_v1_sidebar_profile_history (customer_id, id);

CREATE TABLE contact_v1_owner_migration_result_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  scope_type TEXT NOT NULL,
  file_hash TEXT NOT NULL,
  preview_hash TEXT NOT NULL,
  total_rows BIGINT NOT NULL CHECK (total_rows >= 0),
  eligible_count BIGINT NOT NULL CHECK (eligible_count >= 0),
  wecom_success BIGINT NOT NULL CHECK (wecom_success >= 0),
  wecom_failed BIGINT NOT NULL CHECK (wecom_failed >= 0),
  crm_updated BIGINT NOT NULL CHECK (crm_updated >= 0),
  include_wecom_transfer BOOLEAN NOT NULL,
  transfer_welcome_message TEXT NOT NULL,
  session_relation TEXT NOT NULL CHECK (session_relation IN ('resolved', 'unresolved')),
  preview_relation TEXT NOT NULL CHECK (preview_relation IN ('resolved', 'unresolved')),
  created_at TIMESTAMPTZ NOT NULL,
  executed_at TIMESTAMPTZ NOT NULL CHECK (executed_at >= created_at),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32)
);

-- +goose Down
DROP TABLE contact_v1_owner_migration_result_history;
DROP TABLE contact_v1_sidebar_profile_history;
