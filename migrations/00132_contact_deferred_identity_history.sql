-- +goose Up
-- Unbound V1 evidence only; no Customer/identity assignment or runtime work.
CREATE TABLE contact_v1_deferred_person_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  mobile_digest BYTEA NOT NULL CHECK (octet_length(mobile_digest) = 32),
  third_party_user_id_digest BYTEA NOT NULL CHECK (octet_length(third_party_user_id_digest) = 32),
  private_digest BYTEA NOT NULL CHECK (octet_length(private_digest) = 32),
  redacted_roots TEXT[] NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE contact_v1_deferred_identity_conflict_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  conflict_type TEXT NOT NULL,
  source_type TEXT NOT NULL,
  status TEXT NOT NULL,
  resolution_status TEXT NOT NULL,
  union_id_digest BYTEA NOT NULL CHECK (octet_length(union_id_digest) = 32),
  candidate_union_id_digest BYTEA NOT NULL CHECK (octet_length(candidate_union_id_digest) = 32),
  external_user_id_digest BYTEA NOT NULL CHECK (octet_length(external_user_id_digest) = 32),
  open_id_digest BYTEA NOT NULL CHECK (octet_length(open_id_digest) = 32),
  mobile_digest BYTEA NOT NULL CHECK (octet_length(mobile_digest) = 32),
  legacy_source_key_digest BYTEA NOT NULL CHECK (octet_length(legacy_source_key_digest) = 32),
  payload_json_digest BYTEA NOT NULL CHECK (octet_length(payload_json_digest) = 32),
  source_payload_json_digest BYTEA NOT NULL CHECK (octet_length(source_payload_json_digest) = 32),
  resolution_note_digest BYTEA NOT NULL CHECK (octet_length(resolution_note_digest) = 32),
  private_digest BYTEA NOT NULL CHECK (octet_length(private_digest) = 32),
  redacted_roots TEXT[] NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  resolved_at TIMESTAMPTZ
);

CREATE TABLE contact_v1_missing_root_identity_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  dm01_run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  dm01_source_key_digest BYTEA NOT NULL CHECK (octet_length(dm01_source_key_digest) = 32),
  dm01_source_hmac_key_version TEXT NOT NULL CHECK (dm01_source_hmac_key_version <> ''),
  quarantine_reason TEXT NOT NULL CHECK (quarantine_reason = 'missing_customer_root'),
  type INTEGER,
  status TEXT NOT NULL,
  corp_id_digest BYTEA NOT NULL CHECK (octet_length(corp_id_digest) = 32),
  external_user_id_digest BYTEA NOT NULL CHECK (octet_length(external_user_id_digest) = 32),
  union_id_digest BYTEA NOT NULL CHECK (octet_length(union_id_digest) = 32),
  open_id_digest BYTEA NOT NULL CHECK (octet_length(open_id_digest) = 32),
  follow_user_id_digest BYTEA NOT NULL CHECK (octet_length(follow_user_id_digest) = 32),
  name_digest BYTEA NOT NULL CHECK (octet_length(name_digest) = 32),
  avatar_digest BYTEA NOT NULL CHECK (octet_length(avatar_digest) = 32),
  gender_digest BYTEA CHECK (gender_digest IS NULL OR octet_length(gender_digest) = 32),
  raw_profile_digest BYTEA NOT NULL CHECK (octet_length(raw_profile_digest) = 32),
  private_digest BYTEA NOT NULL CHECK (octet_length(private_digest) = 32),
  redacted_roots TEXT[] NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE contact_v1_missing_root_identity_history;
DROP TABLE contact_v1_deferred_identity_conflict_history;
DROP TABLE contact_v1_deferred_person_history;
