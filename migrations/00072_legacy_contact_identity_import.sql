-- +goose Up
-- DM01 is a local, operator-controlled historical import ledger.  It never
-- starts provider work, emits business events, or restores legacy runtime rows.
CREATE TABLE legacy_contact_identity_import_runs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_manifest_sha256 BYTEA NOT NULL CHECK (octet_length(source_manifest_sha256) = 32),
  source_repository_sha TEXT NOT NULL CHECK (source_repository_sha ~ '^[0-9a-f]{40}$'),
  snapshot_id TEXT NOT NULL CHECK (btrim(snapshot_id) <> ''),
  parent_run_id BIGINT REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  mode TEXT NOT NULL CHECK (mode IN ('preflight','full','incremental','reconcile')),
  -- Audit only; each source table has its own fixed upper bound below.
  upper_watermark TIMESTAMPTZ NOT NULL,
  hmac_key_version SMALLINT NOT NULL CHECK (hmac_key_version > 0),
  state TEXT NOT NULL CHECK (state IN ('reserved','preflighted','importing','imported','reconciling','reconciled','failed')),
  lease_token_hmac BYTEA CHECK (lease_token_hmac IS NULL OR octet_length(lease_token_hmac) = 32),
  lease_generation BIGINT NOT NULL DEFAULT 0 CHECK (lease_generation >= 0),
  lease_expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  CHECK ((mode = 'reconcile' AND parent_run_id IS NOT NULL) OR (mode <> 'reconcile' AND parent_run_id IS NULL)),
  UNIQUE (source_manifest_sha256, mode, upper_watermark)
);
CREATE TABLE legacy_contact_identity_import_checkpoints (
  run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  source_table TEXT NOT NULL CHECK (source_table IN ('owner_role_map','crm_user_identity','wecom_external_contact_identity_map','crm_user_identity_merge_audit','crm_user_identity_resolution_queue','admin_wecom_directory_members','contacts','crm_user_identity_conflicts','external_contact_bindings','people','wecom_external_contact_follow_users')),
  -- Emit one final checkpoint only after the full bounded table scan. Row
  -- receipts, not an HMAC ordering cursor, make a resumed scan idempotent.
  final_source_key_hmac BYTEA NOT NULL CHECK (octet_length(final_source_key_hmac) = 32),
  payload_hmac BYTEA NOT NULL CHECK (octet_length(payload_hmac) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  watermark TIMESTAMPTZ,
  upper_source_key_hmac BYTEA CHECK (upper_source_key_hmac IS NULL OR octet_length(upper_source_key_hmac) = 32),
  upper_bound_empty BOOLEAN NOT NULL DEFAULT FALSE,
  CHECK ((upper_bound_empty AND watermark IS NULL AND upper_source_key_hmac IS NULL) OR (NOT upper_bound_empty AND watermark IS NOT NULL AND upper_source_key_hmac IS NOT NULL)),
  PRIMARY KEY (run_id, source_table),
  UNIQUE (run_id, final_source_key_hmac)
);
CREATE TABLE legacy_contact_identity_source_mappings (
  source_table TEXT NOT NULL,
  source_key_hmac BYTEA NOT NULL CHECK (octet_length(source_key_hmac) = 32),
  staff_id BIGINT REFERENCES staff(id),
  customer_id BIGINT REFERENCES customers(id),
  identity_id BIGINT REFERENCES identities(id),
  first_run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  last_run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  payload_hmac BYTEA NOT NULL CHECK (octet_length(payload_hmac) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (source_table, source_key_hmac),
  CHECK (num_nonnulls(staff_id, customer_id, identity_id) = 1),
  CHECK (
    (source_table = 'owner_role_map' AND staff_id IS NOT NULL)
    OR (source_table = 'crm_user_identity' AND customer_id IS NOT NULL)
    OR (source_table = 'wecom_external_contact_identity_map' AND identity_id IS NOT NULL)
  )
);
CREATE TABLE legacy_contact_identity_import_row_receipts (
  run_id BIGINT NOT NULL REFERENCES legacy_contact_identity_import_runs(id) ON DELETE RESTRICT,
  source_table TEXT NOT NULL,
  source_key_hmac BYTEA NOT NULL CHECK (octet_length(source_key_hmac) = 32),
  payload_hmac BYTEA NOT NULL CHECK (octet_length(payload_hmac) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  disposition TEXT NOT NULL CHECK (disposition IN ('imported','quarantined','archived','skipped')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, source_table, source_key_hmac)
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

CREATE FUNCTION legacy_contact_identity_import_run_transition_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.state = OLD.state THEN
    RETURN NEW;
  END IF;
  IF OLD.mode = 'preflight' AND NOT (OLD.state = 'reserved' AND NEW.state IN ('preflighted', 'failed')) THEN
    RAISE EXCEPTION 'invalid DM01 preflight state transition' USING ERRCODE = '55000';
  END IF;
  IF OLD.mode IN ('full', 'incremental') AND NEW.state = 'reconciling' THEN
    RAISE EXCEPTION 'DM01 import mode cannot reconcile in the same run' USING ERRCODE = '55000';
  END IF;
  IF OLD.mode = 'reconcile' AND NOT (OLD.state = 'reserved' AND NEW.state IN ('reconciling', 'failed')) AND NOT (OLD.state = 'reconciling' AND NEW.state IN ('reconciled', 'failed')) THEN
    RAISE EXCEPTION 'DM01 reconcile mode cannot import rows' USING ERRCODE = '55000';
  END IF;
  IF (OLD.state = 'reserved' AND NEW.state IN ('preflighted', 'reconciling', 'failed'))
    OR (OLD.state = 'preflighted' AND NEW.state IN ('importing', 'failed'))
    OR (OLD.state = 'importing' AND NEW.state IN ('imported', 'failed'))
    OR (OLD.state = 'imported' AND NEW.state IN ('reconciling', 'failed'))
    OR (OLD.state = 'reconciling' AND NEW.state IN ('reconciled', 'failed')) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid DM01 run state transition: % -> %', OLD.state, NEW.state USING ERRCODE = '55000';
END $$;
CREATE TRIGGER legacy_contact_identity_import_run_transition_guard
BEFORE UPDATE OF state ON legacy_contact_identity_import_runs
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_run_transition_guard();

CREATE FUNCTION legacy_contact_identity_import_immutable_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'DM01 import fact is immutable' USING ERRCODE = '55000';
END $$;
CREATE TRIGGER legacy_contact_identity_import_checkpoint_immutable
BEFORE UPDATE OR DELETE ON legacy_contact_identity_import_checkpoints
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_immutable_guard();
CREATE TRIGGER legacy_contact_identity_import_row_receipt_immutable
BEFORE UPDATE OR DELETE ON legacy_contact_identity_import_row_receipts
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_immutable_guard();
CREATE TRIGGER legacy_contact_identity_import_receipt_immutable
BEFORE UPDATE OR DELETE ON legacy_contact_identity_import_receipts
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_immutable_guard();
CREATE TRIGGER legacy_contact_identity_historical_archive_immutable
BEFORE UPDATE OR DELETE ON legacy_contact_identity_historical_archives
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_immutable_guard();
CREATE TRIGGER legacy_contact_identity_import_quarantine_immutable
BEFORE UPDATE OR DELETE ON legacy_contact_identity_import_quarantines
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_immutable_guard();

CREATE FUNCTION legacy_contact_identity_source_mapping_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.source_table <> OLD.source_table
    OR NEW.source_key_hmac <> OLD.source_key_hmac
    OR NEW.staff_id IS DISTINCT FROM OLD.staff_id
    OR NEW.customer_id IS DISTINCT FROM OLD.customer_id
    OR NEW.identity_id IS DISTINCT FROM OLD.identity_id
    OR NEW.first_run_id <> OLD.first_run_id THEN
    RAISE EXCEPTION 'DM01 source mapping binding is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER legacy_contact_identity_source_mapping_guard
BEFORE UPDATE ON legacy_contact_identity_source_mappings
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_source_mapping_guard();
CREATE TRIGGER legacy_contact_identity_source_mapping_immutable_delete
BEFORE DELETE ON legacy_contact_identity_source_mappings
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_immutable_guard();

CREATE FUNCTION legacy_contact_identity_import_run_fact_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.source_manifest_sha256 <> OLD.source_manifest_sha256
    OR NEW.source_repository_sha <> OLD.source_repository_sha
    OR NEW.snapshot_id <> OLD.snapshot_id
    OR NEW.mode <> OLD.mode
    OR NEW.upper_watermark <> OLD.upper_watermark
    OR NEW.hmac_key_version <> OLD.hmac_key_version THEN
    RAISE EXCEPTION 'DM01 run fact is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER legacy_contact_identity_import_run_fact_guard
BEFORE UPDATE ON legacy_contact_identity_import_runs
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_import_run_fact_guard();
CREATE FUNCTION legacy_contact_identity_reconcile_parent_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE parent legacy_contact_identity_import_runs;
BEGIN
  IF NEW.mode <> 'reconcile' THEN RETURN NEW; END IF;
  SELECT * INTO parent FROM legacy_contact_identity_import_runs WHERE id = NEW.parent_run_id FOR SHARE;
  IF NOT FOUND OR parent.mode NOT IN ('full', 'incremental') OR parent.state <> 'imported'
    OR parent.source_manifest_sha256 <> NEW.source_manifest_sha256 OR parent.snapshot_id <> NEW.snapshot_id THEN
    RAISE EXCEPTION 'invalid DM01 reconcile parent run' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER legacy_contact_identity_reconcile_parent_guard
BEFORE INSERT ON legacy_contact_identity_import_runs
FOR EACH ROW EXECUTE FUNCTION legacy_contact_identity_reconcile_parent_guard();
-- +goose Down
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM legacy_contact_identity_source_mappings)
    OR EXISTS (SELECT 1 FROM legacy_contact_identity_import_row_receipts)
    OR EXISTS (SELECT 1 FROM legacy_contact_identity_import_quarantines)
    OR EXISTS (SELECT 1 FROM legacy_contact_identity_historical_archives)
    OR EXISTS (SELECT 1 FROM legacy_contact_identity_import_receipts) THEN
    RAISE EXCEPTION '00072 down refused: materialized DM01 import exists' USING ERRCODE = '55000';
  END IF;
END $$;
DROP TRIGGER legacy_contact_identity_import_run_fact_guard ON legacy_contact_identity_import_runs;
DROP TRIGGER legacy_contact_identity_reconcile_parent_guard ON legacy_contact_identity_import_runs;
DROP TRIGGER legacy_contact_identity_source_mapping_immutable_delete ON legacy_contact_identity_source_mappings;
DROP TRIGGER legacy_contact_identity_import_quarantine_immutable ON legacy_contact_identity_import_quarantines;
DROP TRIGGER legacy_contact_identity_source_mapping_guard ON legacy_contact_identity_source_mappings;
DROP TRIGGER legacy_contact_identity_historical_archive_immutable ON legacy_contact_identity_historical_archives;
DROP TRIGGER legacy_contact_identity_import_receipt_immutable ON legacy_contact_identity_import_receipts;
DROP TRIGGER legacy_contact_identity_import_row_receipt_immutable ON legacy_contact_identity_import_row_receipts;
DROP TRIGGER legacy_contact_identity_import_checkpoint_immutable ON legacy_contact_identity_import_checkpoints;
DROP TRIGGER legacy_contact_identity_import_run_transition_guard ON legacy_contact_identity_import_runs;
DROP FUNCTION legacy_contact_identity_source_mapping_guard();
DROP FUNCTION legacy_contact_identity_import_run_fact_guard();
DROP FUNCTION legacy_contact_identity_reconcile_parent_guard();
DROP FUNCTION legacy_contact_identity_import_immutable_guard();
DROP FUNCTION legacy_contact_identity_import_run_transition_guard();
DROP TABLE legacy_contact_identity_import_receipts;
DROP TABLE legacy_contact_identity_import_row_receipts;
DROP TABLE legacy_contact_identity_source_mappings;
DROP TABLE legacy_contact_identity_historical_archives;
DROP TABLE legacy_contact_identity_import_quarantines;
DROP TABLE legacy_contact_identity_import_checkpoints;
DROP TABLE legacy_contact_identity_import_runs;
