-- +goose Up
-- Generic data-migration control facts only. Domain adapters own source reads,
-- target mutations and target verification; this schema never stores raw rows,
-- credentials, DSNs, provider payloads or guessed identity assignments.
CREATE TABLE data_migration_runs (
  run_id TEXT PRIMARY KEY CHECK (run_id <> '' AND length(run_id) <= 128),
  adapter_id TEXT NOT NULL CHECK (adapter_id <> '' AND length(adapter_id) <= 128),
  source_identity TEXT NOT NULL CHECK (source_identity <> '' AND length(source_identity) <= 512),
  source_schema_digest BYTEA NOT NULL CHECK (octet_length(source_schema_digest) = 32),
  manifest_digest BYTEA NOT NULL CHECK (octet_length(manifest_digest) = 32),
  phase TEXT NOT NULL DEFAULT 'running' CHECK (phase IN ('running','completed','reconciled')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  reconciled_at TIMESTAMPTZ,
  CHECK (
    (phase='running' AND completed_at IS NULL AND reconciled_at IS NULL) OR
    (phase='completed' AND completed_at IS NOT NULL AND reconciled_at IS NULL) OR
    (phase='reconciled' AND completed_at IS NOT NULL AND reconciled_at IS NOT NULL)
  )
);

CREATE TABLE data_migration_run_tables (
  run_id TEXT NOT NULL REFERENCES data_migration_runs(run_id) ON DELETE RESTRICT,
  table_id TEXT NOT NULL CHECK (table_id <> '' AND length(table_id) <= 256),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  source_identity TEXT NOT NULL CHECK (source_identity <> '' AND length(source_identity) <= 512),
  schema_digest BYTEA NOT NULL CHECK (octet_length(schema_digest) = 32),
  upper_bound BYTEA,
  upper_bound_empty BOOLEAN NOT NULL,
  cursor TEXT,
  processed BIGINT NOT NULL DEFAULT 0 CHECK (processed >= 0),
  complete BOOLEAN NOT NULL DEFAULT FALSE,
  last_lease_generation BIGINT,
  last_lease_fence BYTEA,
  PRIMARY KEY(run_id, table_id),
  UNIQUE(run_id, ordinal),
  CHECK ((upper_bound_empty AND upper_bound IS NULL) OR (NOT upper_bound_empty AND octet_length(upper_bound) > 0)),
  CHECK ((processed=0 AND cursor IS NULL) OR (processed>0 AND cursor IS NOT NULL)),
  CHECK ((last_lease_generation IS NULL AND last_lease_fence IS NULL) OR
    (last_lease_generation > 0 AND octet_length(last_lease_fence) = 32))
);

CREATE TABLE data_migration_run_leases (
  run_id TEXT NOT NULL REFERENCES data_migration_runs(run_id) ON DELETE RESTRICT,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BYTEA NOT NULL CHECK (octet_length(fence) = 32),
  acquired_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL CHECK (expires_at > acquired_at),
  active BOOLEAN NOT NULL DEFAULT TRUE,
  retired_at TIMESTAMPTZ,
  PRIMARY KEY(run_id, generation),
  UNIQUE(fence),
  CHECK ((active AND retired_at IS NULL) OR (NOT active AND retired_at IS NOT NULL))
);
CREATE UNIQUE INDEX data_migration_one_active_lease_per_run
  ON data_migration_run_leases(run_id) WHERE active;
ALTER TABLE data_migration_run_tables ADD CONSTRAINT data_migration_run_tables_last_lease_fk
  FOREIGN KEY(run_id,last_lease_generation) REFERENCES data_migration_run_leases(run_id,generation) ON DELETE RESTRICT;

CREATE TABLE data_migration_row_receipts (
  adapter_id TEXT NOT NULL,
  table_id TEXT NOT NULL,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  disposition TEXT NOT NULL CHECK (disposition IN ('import','archive','quarantine','skip','rebuild','reset')),
  mapping_digest BYTEA NOT NULL CHECK (octet_length(mapping_digest) = 32),
  policy_digest BYTEA NOT NULL CHECK (octet_length(policy_digest) = 32),
  operation TEXT NOT NULL DEFAULT '' CHECK (length(operation) <= 128),
  mutation_digest BYTEA NOT NULL CHECK (octet_length(mutation_digest) = 32),
  run_id TEXT NOT NULL,
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(adapter_id, table_id, source_key_digest),
  UNIQUE(adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest),
  FOREIGN KEY(run_id, lease_generation) REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT,
  CHECK ((disposition IN ('import','rebuild','reset') AND operation <> '') OR (disposition NOT IN ('import','rebuild','reset') AND operation = ''))
);

CREATE TABLE data_migration_result_receipts (
  run_id TEXT NOT NULL REFERENCES data_migration_runs(run_id) ON DELETE RESTRICT,
  adapter_id TEXT NOT NULL,
  table_id TEXT NOT NULL,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  disposition TEXT NOT NULL CHECK (disposition IN ('import','archive','quarantine','skip','rebuild','reset')),
  mapping_digest BYTEA NOT NULL CHECK (octet_length(mapping_digest) = 32),
  policy_digest BYTEA NOT NULL CHECK (octet_length(policy_digest) = 32),
  operation TEXT NOT NULL DEFAULT '' CHECK (length(operation) <= 128),
  mutation_digest BYTEA NOT NULL CHECK (octet_length(mutation_digest) = 32),
  outcome TEXT NOT NULL CHECK (outcome IN ('import','archive','quarantine','skip','rebuild','reset')),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(run_id, adapter_id, table_id, source_key_digest),
  FOREIGN KEY(adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest)
    REFERENCES data_migration_row_receipts(adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest) ON DELETE RESTRICT,
  FOREIGN KEY(run_id, lease_generation) REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT,
  CHECK (outcome = disposition)
);
CREATE INDEX data_migration_result_receipts_run ON data_migration_result_receipts(run_id, table_id);

CREATE TABLE data_migration_quarantines (
  run_id TEXT NOT NULL REFERENCES data_migration_runs(run_id) ON DELETE RESTRICT,
  adapter_id TEXT NOT NULL,
  table_id TEXT NOT NULL,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  reason TEXT NOT NULL CHECK (reason <> '' AND length(reason) <= 256),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(run_id, adapter_id, table_id, source_key_digest),
  FOREIGN KEY(run_id, lease_generation) REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT
);

CREATE TABLE data_migration_archives (
  run_id TEXT NOT NULL REFERENCES data_migration_runs(run_id) ON DELETE RESTRICT,
  adapter_id TEXT NOT NULL,
  table_id TEXT NOT NULL,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(run_id, adapter_id, table_id, source_key_digest),
  FOREIGN KEY(run_id, lease_generation) REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT
);

CREATE TABLE data_migration_reconciliation_receipts (
  run_id TEXT PRIMARY KEY REFERENCES data_migration_runs(run_id) ON DELETE RESTRICT,
  source_row_count BIGINT NOT NULL CHECK (source_row_count >= 0),
  result_row_count BIGINT NOT NULL CHECK (result_row_count >= 0),
  target_verified_count BIGINT NOT NULL CHECK (target_verified_count >= 0),
  comparison_digest BYTEA NOT NULL CHECK (octet_length(comparison_digest) = 32),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY(run_id, lease_generation) REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT,
  CHECK (source_row_count = result_row_count AND result_row_count = target_verified_count)
);

-- +goose StatementBegin
CREATE FUNCTION aicrm_data_migration_reject_change() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'data migration facts are immutable' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION aicrm_data_migration_active_fence(run_value TEXT, generation_value BIGINT, fence_value BYTEA) RETURNS BOOLEAN
LANGUAGE sql STABLE SET search_path = pg_catalog AS $$
  SELECT EXISTS (
    SELECT 1 FROM public.data_migration_run_leases
    WHERE run_id=run_value AND generation=generation_value AND fence=fence_value
      AND active AND expires_at > statement_timestamp()
  )
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION aicrm_data_migration_run_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.adapter_id IS DISTINCT FROM OLD.adapter_id
     OR NEW.source_identity IS DISTINCT FROM OLD.source_identity
     OR NEW.source_schema_digest IS DISTINCT FROM OLD.source_schema_digest
     OR NEW.manifest_digest IS DISTINCT FROM OLD.manifest_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'data migration run identity is immutable' USING ERRCODE='55000';
  END IF;
  IF OLD.phase='running' AND NEW.phase='completed' AND OLD.completed_at IS NULL
     AND NEW.completed_at IS NOT NULL AND NEW.reconciled_at IS NULL
     AND NOT EXISTS (SELECT 1 FROM public.data_migration_run_tables WHERE run_id=OLD.run_id AND NOT complete)
     AND NOT EXISTS (SELECT 1 FROM public.data_migration_run_leases WHERE run_id=OLD.run_id AND active) THEN
    RETURN NEW;
  END IF;
  IF OLD.phase='completed' AND NEW.phase='reconciled'
     AND NEW.completed_at IS NOT DISTINCT FROM OLD.completed_at
     AND OLD.reconciled_at IS NULL AND NEW.reconciled_at IS NOT NULL
     AND EXISTS (SELECT 1 FROM public.data_migration_reconciliation_receipts WHERE run_id=OLD.run_id)
     AND NOT EXISTS (SELECT 1 FROM public.data_migration_run_leases WHERE run_id=OLD.run_id AND active) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid data migration run transition' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER data_migration_run_update_guard BEFORE UPDATE ON data_migration_runs
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_run_guard();
CREATE TRIGGER data_migration_run_delete_guard BEFORE DELETE ON data_migration_runs
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();

-- +goose StatementBegin
CREATE FUNCTION aicrm_data_migration_checkpoint_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE run_phase TEXT;
BEGIN
  SELECT phase INTO run_phase FROM public.data_migration_runs WHERE run_id=OLD.run_id FOR UPDATE;
  IF run_phase IS DISTINCT FROM 'running'
     OR NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.table_id IS DISTINCT FROM OLD.table_id
     OR NEW.ordinal IS DISTINCT FROM OLD.ordinal OR NEW.source_identity IS DISTINCT FROM OLD.source_identity
     OR NEW.schema_digest IS DISTINCT FROM OLD.schema_digest
     OR NEW.upper_bound IS DISTINCT FROM OLD.upper_bound OR NEW.upper_bound_empty IS DISTINCT FROM OLD.upper_bound_empty
     OR NEW.processed < OLD.processed OR (OLD.complete AND NOT NEW.complete)
     OR (NEW.processed > OLD.processed AND NEW.cursor IS NULL)
     OR (NEW.processed = OLD.processed AND NEW.cursor IS DISTINCT FROM OLD.cursor)
     OR (OLD.complete AND NEW IS DISTINCT FROM OLD)
     OR NOT public.aicrm_data_migration_active_fence(NEW.run_id,NEW.last_lease_generation,NEW.last_lease_fence) THEN
    RAISE EXCEPTION 'invalid data migration checkpoint transition' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER data_migration_checkpoint_update_guard BEFORE UPDATE ON data_migration_run_tables
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_checkpoint_guard();
-- +goose StatementBegin
CREATE FUNCTION aicrm_data_migration_checkpoint_insert_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF (SELECT phase FROM public.data_migration_runs WHERE run_id=NEW.run_id) IS DISTINCT FROM 'running'
     OR EXISTS (SELECT 1 FROM public.data_migration_run_leases WHERE run_id=NEW.run_id)
     OR NEW.ordinal <> (SELECT count(*) FROM public.data_migration_run_tables WHERE run_id=NEW.run_id) THEN
    RAISE EXCEPTION 'invalid data migration checkpoint registration' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER data_migration_checkpoint_insert_guard BEFORE INSERT ON data_migration_run_tables
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_checkpoint_insert_guard();
CREATE TRIGGER data_migration_checkpoint_delete_guard BEFORE DELETE ON data_migration_run_tables
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();

-- +goose StatementBegin
CREATE FUNCTION aicrm_data_migration_lease_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP='INSERT' THEN
    IF NEW.generation <> COALESCE((SELECT max(generation)+1 FROM public.data_migration_run_leases WHERE run_id=NEW.run_id),1)
       OR EXISTS (SELECT 1 FROM public.data_migration_run_leases WHERE run_id=NEW.run_id AND active)
       OR (SELECT phase FROM public.data_migration_runs WHERE run_id=NEW.run_id) NOT IN ('running','completed') THEN
      RAISE EXCEPTION 'invalid or competing data migration lease' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.run_id=OLD.run_id AND NEW.generation=OLD.generation AND NEW.fence=OLD.fence
     AND NEW.acquired_at=OLD.acquired_at AND OLD.active
     AND ((NEW.active AND NEW.retired_at IS NULL AND NEW.expires_at>OLD.expires_at)
       OR (NOT NEW.active AND NEW.retired_at IS NOT NULL AND NEW.expires_at=OLD.expires_at)) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid data migration lease transition' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER data_migration_lease_insert_update_guard BEFORE INSERT OR UPDATE ON data_migration_run_leases
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_lease_guard();
CREATE TRIGGER data_migration_lease_delete_guard BEFORE DELETE ON data_migration_run_leases
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();

-- +goose StatementBegin
CREATE FUNCTION aicrm_data_migration_fenced_insert_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT public.aicrm_data_migration_active_fence(NEW.run_id, NEW.lease_generation, NEW.lease_fence) THEN
    RAISE EXCEPTION 'data migration lease fenced' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER data_migration_row_receipt_insert_guard BEFORE INSERT ON data_migration_row_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER data_migration_result_receipt_insert_guard BEFORE INSERT ON data_migration_result_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER data_migration_quarantine_insert_guard BEFORE INSERT ON data_migration_quarantines
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER data_migration_archive_insert_guard BEFORE INSERT ON data_migration_archives
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER data_migration_reconcile_insert_guard BEFORE INSERT ON data_migration_reconciliation_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();

CREATE TRIGGER data_migration_row_receipt_change_guard BEFORE UPDATE OR DELETE ON data_migration_row_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER data_migration_result_receipt_change_guard BEFORE UPDATE OR DELETE ON data_migration_result_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER data_migration_quarantine_change_guard BEFORE UPDATE OR DELETE ON data_migration_quarantines
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER data_migration_archive_change_guard BEFORE UPDATE OR DELETE ON data_migration_archives
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER data_migration_reconcile_change_guard BEFORE UPDATE OR DELETE ON data_migration_reconciliation_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();

-- +goose Down
LOCK TABLE data_migration_reconciliation_receipts, data_migration_result_receipts,
  data_migration_row_receipts, data_migration_quarantines, data_migration_archives,
  data_migration_run_leases, data_migration_run_tables, data_migration_runs
  IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM data_migration_runs)
     OR EXISTS (SELECT 1 FROM data_migration_run_tables)
     OR EXISTS (SELECT 1 FROM data_migration_run_leases)
     OR EXISTS (SELECT 1 FROM data_migration_row_receipts)
     OR EXISTS (SELECT 1 FROM data_migration_result_receipts)
     OR EXISTS (SELECT 1 FROM data_migration_quarantines)
     OR EXISTS (SELECT 1 FROM data_migration_archives)
     OR EXISTS (SELECT 1 FROM data_migration_reconciliation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back populated data migration harness' USING ERRCODE='55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE data_migration_reconciliation_receipts;
DROP TABLE data_migration_archives;
DROP TABLE data_migration_quarantines;
DROP TABLE data_migration_result_receipts;
DROP TABLE data_migration_row_receipts;
DROP TABLE data_migration_run_tables;
DROP TABLE data_migration_run_leases;
DROP TABLE data_migration_runs;
DROP FUNCTION aicrm_data_migration_fenced_insert_guard();
DROP FUNCTION aicrm_data_migration_lease_guard();
DROP FUNCTION aicrm_data_migration_checkpoint_insert_guard();
DROP FUNCTION aicrm_data_migration_checkpoint_guard();
DROP FUNCTION aicrm_data_migration_run_guard();
DROP FUNCTION aicrm_data_migration_active_fence(TEXT, BIGINT, BYTEA);
DROP FUNCTION aicrm_data_migration_reject_change();
