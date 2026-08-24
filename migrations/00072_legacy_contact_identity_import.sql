-- +goose Up
-- DM01 is a local, operator-controlled historical import ledger.  It never
-- starts provider work, emits business events, or restores legacy runtime rows.
CREATE TABLE legacy_contact_identity_import_runs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_manifest_sha256 BYTEA NOT NULL CHECK (octet_length(source_manifest_sha256) = 32),
  source_repository_sha TEXT NOT NULL CHECK (source_repository_sha ~ '^[0-9a-f]{40}$'),
  snapshot_id TEXT NOT NULL CHECK (btrim(snapshot_id) <> ''),
  mode TEXT NOT NULL CHECK (mode IN ('preflight','full','incremental','reconcile')),
  upper_watermark TIMESTAMPTZ NOT NULL,
  hmac_key_version SMALLINT NOT NULL CHECK (hmac_key_version > 0),
  state TEXT NOT NULL CHECK (state IN ('reserved','preflighted','importing','imported','reconciling','reconciled','failed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (source_manifest_sha256, mode, upper_watermark)
);
CREATE TABLE legacy_contact_identity_import_checkpoints (
  run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  source_table TEXT NOT NULL CHECK (source_table IN ('owner_role_map','crm_user_identity','wecom_external_contact_identity_map','crm_user_identity_merge_audit','crm_user_identity_resolution_queue')),
  source_key_hmac BYTEA NOT NULL CHECK (octet_length(source_key_hmac) = 32),
  payload_hmac BYTEA NOT NULL CHECK (octet_length(payload_hmac) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  watermark TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (run_id, source_table),
  UNIQUE (run_id, source_key_hmac)
);
CREATE TABLE legacy_contact_identity_import_quarantines (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  source_table TEXT NOT NULL,
  source_key_hmac BYTEA NOT NULL CHECK (octet_length(source_key_hmac) = 32),
  reason_code TEXT NOT NULL CHECK (btrim(reason_code) <> ''),
  payload_hmac BYTEA NOT NULL CHECK (octet_length(payload_hmac) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (run_id, source_table, source_key_hmac, reason_code)
);
CREATE TABLE legacy_contact_identity_historical_archives (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  source_table TEXT NOT NULL CHECK (source_table IN ('crm_user_identity_merge_audit','crm_user_identity_resolution_queue')),
  source_key_hmac BYTEA NOT NULL CHECK (octet_length(source_key_hmac) = 32),
  payload_hmac BYTEA NOT NULL CHECK (octet_length(payload_hmac) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  archive_nonce BYTEA NOT NULL CHECK (octet_length(archive_nonce) = 12),
  archive_ciphertext BYTEA NOT NULL CHECK (octet_length(archive_ciphertext) > 16),
  archive_key_version SMALLINT NOT NULL CHECK (archive_key_version > 0),
  inactive BOOLEAN NOT NULL DEFAULT TRUE CHECK (inactive),
  archived_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (run_id, source_table, source_key_hmac)
);
CREATE TABLE legacy_contact_identity_import_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  run_id BIGINT NOT NULL UNIQUE REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  result_digest BYTEA NOT NULL CHECK (octet_length(result_digest) = 32),
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose Down
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM legacy_contact_identity_import_runs WHERE state IN ('imported','reconciling','reconciled')) THEN
    RAISE EXCEPTION '00072 down refused: materialized DM01 import exists' USING ERRCODE = '55000';
  END IF;
END $$;
DROP TABLE legacy_contact_identity_import_receipts;
DROP TABLE legacy_contact_identity_historical_archives;
DROP TABLE legacy_contact_identity_import_quarantines;
DROP TABLE legacy_contact_identity_import_checkpoints;
DROP TABLE legacy_contact_identity_import_runs;
