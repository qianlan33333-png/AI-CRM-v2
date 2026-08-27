-- +goose Up
-- Canonical V1 imports are performed only by V2 domain-owned writers.  This
-- journal links each terminal decision back to the immutable 00107 archive;
-- it never contains source payloads, credentials, or executable legacy work.
CREATE TABLE public.v1_domain_import_receipts (
  import_version TEXT NOT NULL CHECK (import_version ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
  archive_run_id TEXT NOT NULL REFERENCES public.v1_archive_runs(run_id) ON DELETE RESTRICT,
  adapter_id TEXT NOT NULL CHECK (adapter_id <> '' AND length(adapter_id) <= 128),
  table_id TEXT NOT NULL CHECK (table_id ~ '^[a-z_][a-z0-9_]*/[a-z_][a-z0-9_]*$' AND length(table_id) <= 127),
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  disposition TEXT NOT NULL CHECK (disposition IN ('import','archive','quarantine')),
  reason TEXT NOT NULL DEFAULT '' CHECK (btrim(reason) = reason AND length(reason) <= 256),
  target_domain TEXT,
  target_table TEXT,
  target_id TEXT,
  target_digest BYTEA,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  verified BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(import_version, archive_run_id, adapter_id, table_id, source_key_digest),
  CHECK (
    (disposition = 'import' AND reason = '' AND target_domain IS NOT NULL
      AND target_table IS NOT NULL AND target_id IS NOT NULL
      AND octet_length(target_digest) = 32 AND verified) OR
    (disposition IN ('archive','quarantine') AND reason <> ''
      AND target_domain IS NULL AND target_table IS NULL AND target_id IS NULL
      AND target_digest IS NULL AND verified)
  ),
  CHECK (target_domain IS NULL OR (btrim(target_domain) = target_domain AND target_domain <> '' AND length(target_domain) <= 128)),
  CHECK (target_table IS NULL OR (target_table ~ '^[a-z_][a-z0-9_]*$' AND length(target_table) <= 63)),
  CHECK (target_id IS NULL OR (btrim(target_id) = target_id AND target_id <> '' AND length(target_id) <= 256))
);
CREATE INDEX v1_domain_import_receipts_run_table
  ON public.v1_domain_import_receipts(import_version, archive_run_id, table_id);

CREATE TABLE public.v1_domain_import_reconciliation_receipts (
  import_version TEXT NOT NULL,
  archive_run_id TEXT NOT NULL REFERENCES public.v1_archive_runs(run_id) ON DELETE RESTRICT,
  selected_source_count BIGINT NOT NULL CHECK (selected_source_count >= 0),
  receipt_count BIGINT NOT NULL CHECK (receipt_count >= 0),
  imported_count BIGINT NOT NULL CHECK (imported_count >= 0),
  archived_count BIGINT NOT NULL CHECK (archived_count >= 0),
  quarantined_count BIGINT NOT NULL CHECK (quarantined_count >= 0),
  verified_count BIGINT NOT NULL CHECK (verified_count >= 0),
  comparison_digest BYTEA NOT NULL CHECK (octet_length(comparison_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(import_version, archive_run_id),
  CHECK (selected_source_count = receipt_count),
  CHECK (receipt_count = imported_count + archived_count + quarantined_count),
  CHECK (receipt_count = verified_count)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_v1_domain_import_receipt_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.v1_domain_import_reconciliation_receipts
    WHERE import_version = NEW.import_version AND archive_run_id = NEW.archive_run_id
  ) THEN
    RAISE EXCEPTION 'V1 domain import is already reconciled' USING ERRCODE='55000';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM public.v1_archive_records
    WHERE run_id = NEW.archive_run_id AND adapter_id = NEW.adapter_id
      AND table_id = NEW.table_id AND source_key_digest = NEW.source_key_digest
      AND payload_digest = NEW.payload_digest
  ) THEN
    RAISE EXCEPTION 'V1 domain import source does not match archive' USING ERRCODE='23503';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_v1_domain_import_reconcile_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  actual_receipts BIGINT;
  actual_imported BIGINT;
  actual_archived BIGINT;
  actual_quarantined BIGINT;
  actual_verified BIGINT;
BEGIN
  SELECT count(*),
         count(*) FILTER (WHERE disposition='import'),
         count(*) FILTER (WHERE disposition='archive'),
         count(*) FILTER (WHERE disposition='quarantine'),
         count(*) FILTER (WHERE verified)
  INTO actual_receipts, actual_imported, actual_archived, actual_quarantined, actual_verified
  FROM public.v1_domain_import_receipts
  WHERE import_version=NEW.import_version AND archive_run_id=NEW.archive_run_id;
  IF ROW(NEW.receipt_count,NEW.imported_count,NEW.archived_count,NEW.quarantined_count,NEW.verified_count)
     IS DISTINCT FROM ROW(actual_receipts,actual_imported,actual_archived,actual_quarantined,actual_verified) THEN
    RAISE EXCEPTION 'V1 domain import reconciliation counts do not match receipts' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER v1_domain_import_receipt_insert_guard BEFORE INSERT ON public.v1_domain_import_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_v1_domain_import_receipt_guard();
CREATE TRIGGER v1_domain_import_receipt_change_guard BEFORE UPDATE OR DELETE ON public.v1_domain_import_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_data_migration_reject_change();
CREATE TRIGGER v1_domain_import_reconcile_insert_guard BEFORE INSERT ON public.v1_domain_import_reconciliation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_v1_domain_import_reconcile_guard();
CREATE TRIGGER v1_domain_import_reconcile_change_guard BEFORE UPDATE OR DELETE ON public.v1_domain_import_reconciliation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_data_migration_reject_change();

-- +goose Down
LOCK TABLE public.v1_domain_import_reconciliation_receipts, public.v1_domain_import_receipts
  IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.v1_domain_import_receipts)
     OR EXISTS (SELECT 1 FROM public.v1_domain_import_reconciliation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back populated V1 domain import journal' USING ERRCODE='55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER v1_domain_import_reconcile_change_guard ON public.v1_domain_import_reconciliation_receipts;
DROP TRIGGER v1_domain_import_reconcile_insert_guard ON public.v1_domain_import_reconciliation_receipts;
DROP TRIGGER v1_domain_import_receipt_change_guard ON public.v1_domain_import_receipts;
DROP TRIGGER v1_domain_import_receipt_insert_guard ON public.v1_domain_import_receipts;
DROP FUNCTION public.aicrm_v1_domain_import_reconcile_guard();
DROP FUNCTION public.aicrm_v1_domain_import_receipt_guard();
DROP TABLE public.v1_domain_import_reconciliation_receipts;
DROP TABLE public.v1_domain_import_receipts;
