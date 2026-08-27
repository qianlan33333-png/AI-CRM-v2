-- +goose Up
-- V1 rows are retained as encrypted, immutable V2-owned history.  This schema
-- never makes legacy queues, sessions, provider callbacks or payment state
-- executable; domain-specific adapters must create any canonical V2 facts.
CREATE TABLE v1_archive_runs (
  run_id TEXT PRIMARY KEY REFERENCES data_migration_runs(run_id) ON DELETE RESTRICT,
  source_repository_sha TEXT NOT NULL CHECK (source_repository_sha ~ '^[0-9a-f]{40}$'),
  snapshot_digest BYTEA NOT NULL CHECK (octet_length(snapshot_digest) = 32),
  schema_digest BYTEA NOT NULL CHECK (octet_length(schema_digest) = 32),
  policy_digest BYTEA NOT NULL CHECK (octet_length(policy_digest) = 32),
  key_version INTEGER NOT NULL CHECK (key_version > 0),
  table_count INTEGER NOT NULL CHECK (table_count > 0),
  row_count BIGINT NOT NULL CHECK (row_count >= 0),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(snapshot_digest, schema_digest, policy_digest),
  FOREIGN KEY(run_id, lease_generation)
    REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT
);

CREATE TABLE v1_archive_tables (
  run_id TEXT NOT NULL REFERENCES v1_archive_runs(run_id) ON DELETE RESTRICT,
  table_id TEXT NOT NULL CHECK (table_id ~ '^[a-z_][a-z0-9_]*/[a-z_][a-z0-9_]*$' AND length(table_id) <= 127),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  schema_digest BYTEA NOT NULL CHECK (octet_length(schema_digest) = 32),
  row_count BIGINT NOT NULL CHECK (row_count >= 0),
  column_manifest JSONB NOT NULL CHECK (jsonb_typeof(column_manifest) = 'array'),
  pk_columns JSONB NOT NULL CHECK (jsonb_typeof(pk_columns) = 'array'),
  disposition TEXT NOT NULL CHECK (disposition IN ('canonical','archive','rebuild','reset','manual')),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(run_id, table_id),
  UNIQUE(run_id, ordinal),
  FOREIGN KEY(run_id, lease_generation)
    REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT
);

CREATE TABLE v1_archive_records (
  adapter_id TEXT NOT NULL CHECK (adapter_id <> '' AND length(adapter_id) <= 128),
  table_id TEXT NOT NULL CHECK (table_id ~ '^[a-z_][a-z0-9_]*/[a-z_][a-z0-9_]*$' AND length(table_id) <= 127),
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  source_ordinal BIGINT NOT NULL CHECK (source_ordinal > 0),
  run_id TEXT NOT NULL,
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  field_digest BYTEA NOT NULL CHECK (octet_length(field_digest) = 32),
  schema_digest BYTEA NOT NULL CHECK (octet_length(schema_digest) = 32),
  nonce BYTEA NOT NULL CHECK (octet_length(nonce) = 12),
  ciphertext BYTEA NOT NULL CHECK (octet_length(ciphertext) > 16),
  key_version INTEGER NOT NULL CHECK (key_version > 0),
  compression TEXT NOT NULL CHECK (compression IN ('none','gzip')),
  redaction_metadata JSONB NOT NULL DEFAULT '[]'::jsonb
    CHECK (jsonb_typeof(redaction_metadata) = 'array'),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(adapter_id, table_id, source_key_digest),
  UNIQUE(run_id, table_id, source_ordinal),
  FOREIGN KEY(run_id, table_id) REFERENCES v1_archive_tables(run_id, table_id) ON DELETE RESTRICT,
  FOREIGN KEY(run_id, lease_generation)
    REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT,
  FOREIGN KEY(adapter_id, table_id, source_key_digest)
    REFERENCES data_migration_row_receipts(adapter_id, table_id, source_key_digest) ON DELETE RESTRICT
);
CREATE INDEX v1_archive_records_run_table ON v1_archive_records(run_id, table_id);

CREATE TABLE v1_archive_mappings (
  adapter_id TEXT NOT NULL,
  table_id TEXT NOT NULL,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  run_id TEXT NOT NULL,
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  disposition TEXT NOT NULL CHECK (disposition IN ('import','archive','quarantine','skip','rebuild','reset')),
  target_domain TEXT,
  target_table TEXT,
  target_id TEXT,
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(adapter_id, table_id, source_key_digest),
  FOREIGN KEY(run_id, table_id) REFERENCES v1_archive_tables(run_id, table_id) ON DELETE RESTRICT,
  FOREIGN KEY(run_id, lease_generation)
    REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT,
  FOREIGN KEY(adapter_id, table_id, source_key_digest)
    REFERENCES data_migration_row_receipts(adapter_id, table_id, source_key_digest) ON DELETE RESTRICT,
  CHECK (
    (disposition = 'import' AND target_domain IS NOT NULL AND target_table IS NOT NULL AND target_id IS NOT NULL) OR
    (disposition = 'rebuild' AND target_domain IS NOT NULL AND target_table IS NOT NULL) OR
    (disposition NOT IN ('import','rebuild') AND target_domain IS NULL AND target_table IS NULL AND target_id IS NULL)
  ),
  CHECK (target_domain IS NULL OR (target_domain <> '' AND length(target_domain) <= 128)),
  CHECK (target_table IS NULL OR (target_table ~ '^[a-z_][a-z0-9_]*$' AND length(target_table) <= 63)),
  CHECK (target_id IS NULL OR (target_id <> '' AND length(target_id) <= 256))
);
CREATE INDEX v1_archive_mappings_run_table ON v1_archive_mappings(run_id, table_id);

CREATE TABLE v1_archive_reconciliation_receipts (
  run_id TEXT PRIMARY KEY REFERENCES v1_archive_runs(run_id) ON DELETE RESTRICT,
  source_table_count INTEGER NOT NULL CHECK (source_table_count > 0),
  archived_table_count INTEGER NOT NULL CHECK (archived_table_count > 0),
  source_row_count BIGINT NOT NULL CHECK (source_row_count >= 0),
  archive_record_count BIGINT NOT NULL CHECK (archive_record_count >= 0),
  terminal_disposition_count BIGINT NOT NULL CHECK (terminal_disposition_count >= 0),
  canonical_mapping_count BIGINT NOT NULL CHECK (canonical_mapping_count >= 0),
  target_verified_count BIGINT NOT NULL CHECK (target_verified_count >= 0),
  comparison_digest BYTEA NOT NULL CHECK (octet_length(comparison_digest) = 32),
  lease_generation BIGINT NOT NULL,
  lease_fence BYTEA NOT NULL CHECK (octet_length(lease_fence) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY(run_id, lease_generation)
    REFERENCES data_migration_run_leases(run_id, generation) ON DELETE RESTRICT,
  CHECK (source_table_count = archived_table_count),
  CHECK (source_row_count = archive_record_count),
  CHECK (source_row_count = terminal_disposition_count),
  CHECK (canonical_mapping_count = target_verified_count),
  CHECK (canonical_mapping_count <= source_row_count)
);

CREATE TRIGGER v1_archive_run_insert_guard BEFORE INSERT ON v1_archive_runs
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER v1_archive_table_insert_guard BEFORE INSERT ON v1_archive_tables
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER v1_archive_record_insert_guard BEFORE INSERT ON v1_archive_records
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER v1_archive_mapping_insert_guard BEFORE INSERT ON v1_archive_mappings
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();
CREATE TRIGGER v1_archive_reconcile_insert_guard BEFORE INSERT ON v1_archive_reconciliation_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_fenced_insert_guard();

CREATE TRIGGER v1_archive_run_change_guard BEFORE UPDATE OR DELETE ON v1_archive_runs
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER v1_archive_table_change_guard BEFORE UPDATE OR DELETE ON v1_archive_tables
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER v1_archive_record_change_guard BEFORE UPDATE OR DELETE ON v1_archive_records
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER v1_archive_mapping_change_guard BEFORE UPDATE OR DELETE ON v1_archive_mappings
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();
CREATE TRIGGER v1_archive_reconcile_change_guard BEFORE UPDATE OR DELETE ON v1_archive_reconciliation_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_data_migration_reject_change();

-- +goose Down
LOCK TABLE v1_archive_reconciliation_receipts, v1_archive_mappings,
  v1_archive_records, v1_archive_tables, v1_archive_runs IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM v1_archive_runs)
     OR EXISTS (SELECT 1 FROM v1_archive_tables)
     OR EXISTS (SELECT 1 FROM v1_archive_records)
     OR EXISTS (SELECT 1 FROM v1_archive_mappings)
     OR EXISTS (SELECT 1 FROM v1_archive_reconciliation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back populated V1 archive' USING ERRCODE='55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE v1_archive_reconciliation_receipts;
DROP TABLE v1_archive_mappings;
DROP TABLE v1_archive_records;
DROP TABLE v1_archive_tables;
DROP TABLE v1_archive_runs;
