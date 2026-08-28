-- +goose Up
-- Inert V1 facts; no current Staff, owner, identity or permission mutation.
CREATE TABLE contact_v1_external_binding_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  external_user_id_digest BYTEA NOT NULL CHECK (octet_length(external_user_id_digest) = 32),
  source_person_id BIGINT NOT NULL,
  person_history_id BIGINT REFERENCES contact_v1_deferred_person_history(id),
  identity_id BIGINT REFERENCES identities(id),
  identity_assurance TEXT NOT NULL CHECK (identity_assurance IN ('unresolved', 'declared', 'verified')),
  first_bound_by_user_id_digest BYTEA NOT NULL CHECK (octet_length(first_bound_by_user_id_digest) = 32),
  first_owner_user_id_digest BYTEA NOT NULL CHECK (octet_length(first_owner_user_id_digest) = 32),
  last_owner_user_id_digest BYTEA NOT NULL CHECK (octet_length(last_owner_user_id_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CHECK ((identity_id IS NULL AND identity_assurance = 'unresolved') OR
         (identity_id IS NOT NULL AND identity_assurance IN ('declared', 'verified')))
);

CREATE TABLE contact_v1_directory_member_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  wecom_corp_id_digest BYTEA NOT NULL CHECK (octet_length(wecom_corp_id_digest) = 32),
  corp_id_digest BYTEA NOT NULL CHECK (octet_length(corp_id_digest) = 32),
  wecom_user_id_digest BYTEA NOT NULL CHECK (octet_length(wecom_user_id_digest) = 32),
  corp_attribution TEXT NOT NULL CHECK (corp_attribution IN ('matched', 'unattributable')),
  matched_staff_id BIGINT REFERENCES staff(id),
  display_name TEXT NOT NULL,
  department_ids_digest BYTEA NOT NULL CHECK (octet_length(department_ids_digest) = 32),
  department_name TEXT NOT NULL,
  position TEXT NOT NULL,
  wecom_status INTEGER,
  is_active BOOLEAN NOT NULL,
  synced_at TIMESTAMPTZ NOT NULL,
  raw_payload_digest BYTEA NOT NULL CHECK (octet_length(raw_payload_digest) = 32),
  mobile_digest BYTEA NOT NULL CHECK (octet_length(mobile_digest) = 32),
  avatar_url_digest BYTEA NOT NULL CHECK (octet_length(avatar_url_digest) = 32),
  updated_by_digest BYTEA NOT NULL CHECK (octet_length(updated_by_digest) = 32),
  first_seen_at TIMESTAMPTZ NOT NULL,
  last_synced_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CHECK (corp_attribution = 'matched' OR matched_staff_id IS NULL)
);

-- +goose Down
DROP TABLE contact_v1_directory_member_history;
DROP TABLE contact_v1_external_binding_history;
