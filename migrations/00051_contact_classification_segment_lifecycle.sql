-- +goose Up
-- CRM-local classification lifecycle. This migration deliberately does not
-- alter provider identifiers or invoke any provider / sync operation.
ALTER TABLE stages
  ADD COLUMN archived_at TIMESTAMPTZ,
  ADD COLUMN archived_by TEXT,
  ADD CONSTRAINT stages_archive_pair CHECK (
    (archived_at IS NULL AND archived_by IS NULL)
    OR (archived_at IS NOT NULL AND archived_by IS NOT NULL
        AND btrim(archived_by) = archived_by AND archived_by <> ''
        AND char_length(archived_by) <= 200)
  );

CREATE INDEX idx_stages_active_sort
  ON stages (sort_order, id)
  WHERE archived_at IS NULL;

CREATE TABLE public.stage_operation_receipts (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation      TEXT NOT NULL CHECK (operation IN ('create', 'rename', 'reorder', 'archive')),
  actor          TEXT NOT NULL CHECK (btrim(actor) = actor AND actor <> '' AND char_length(actor) <= 200),
  key_digest     BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  state          TEXT NOT NULL DEFAULT 'in_progress' CHECK (state IN ('in_progress', 'completed')),
  result_ids     BIGINT[] NOT NULL DEFAULT '{}',
  created_at     TIMESTAMPTZ NOT NULL,
  completed_at   TIMESTAMPTZ,
  CONSTRAINT stage_operation_receipts_completion CHECK (
    (state = 'in_progress' AND cardinality(result_ids) = 0 AND completed_at IS NULL)
    OR (state = 'completed' AND cardinality(result_ids) >= 1 AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor, key_digest)
);

CREATE TABLE public.tag_catalog_operation_receipts (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation      TEXT NOT NULL CHECK (operation IN ('group_create', 'group_update', 'group_reorder', 'group_archive', 'tag_create', 'tag_update', 'tag_reorder', 'tag_archive')),
  actor          TEXT NOT NULL CHECK (btrim(actor) = actor AND actor <> '' AND char_length(actor) <= 200),
  key_digest     BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  state          TEXT NOT NULL DEFAULT 'in_progress' CHECK (state IN ('in_progress', 'completed')),
  result_ids     BIGINT[] NOT NULL DEFAULT '{}',
  created_at     TIMESTAMPTZ NOT NULL,
  completed_at   TIMESTAMPTZ,
  CONSTRAINT tag_catalog_operation_receipts_completion CHECK (
    (state = 'in_progress' AND cardinality(result_ids) = 0 AND completed_at IS NULL)
    OR (state = 'completed' AND cardinality(result_ids) >= 1 AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor, key_digest)
);

-- A receipt is never allowed to commit reserved: a network outcome can be
-- unknown to a browser, but not half-persisted in the local transaction.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_local_catalog_receipt_complete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.stage_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'stage operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER stage_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.stage_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_local_catalog_receipt_complete();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_local_catalog_receipt_transition()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed local catalog receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor IS DISTINCT FROM OLD.actor OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid local catalog receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER stage_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.stage_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_local_catalog_receipt_transition();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_tag_catalog_receipt_complete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.tag_catalog_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'tag catalog receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER tag_catalog_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.tag_catalog_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_tag_catalog_receipt_complete();

CREATE TRIGGER tag_catalog_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.tag_catalog_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_local_catalog_receipt_transition();

ALTER TABLE segments
  ADD COLUMN lifecycle_status TEXT NOT NULL DEFAULT 'active',
  ADD COLUMN archived_at TIMESTAMPTZ,
  ADD COLUMN archived_by TEXT,
  ADD CONSTRAINT segments_lifecycle_status CHECK (lifecycle_status IN ('active', 'archived')),
  ADD CONSTRAINT segments_archive_pair CHECK (
    (lifecycle_status = 'active' AND archived_at IS NULL AND archived_by IS NULL)
    OR (lifecycle_status = 'archived' AND archived_at IS NOT NULL AND archived_by IS NOT NULL
        AND btrim(archived_by) = archived_by AND archived_by <> ''
        AND char_length(archived_by) <= 200)
  );

CREATE INDEX idx_segments_active_refresh_due
  ON segments (refresh_mode, refresh_status, refreshed_at, id)
  WHERE refresh_mode = 'scheduled' AND lifecycle_status = 'active';

ALTER TABLE segment_operation_receipts
  DROP CONSTRAINT segment_operation_receipts_operation,
  ADD CONSTRAINT segment_operation_receipts_operation CHECK (operation IN ('create', 'update', 'archive'));

-- +goose Down
ALTER TABLE segment_operation_receipts
  DROP CONSTRAINT segment_operation_receipts_operation,
  ADD CONSTRAINT segment_operation_receipts_operation CHECK (operation IN ('create', 'update'));

DROP INDEX idx_segments_active_refresh_due;
ALTER TABLE segments
  DROP CONSTRAINT segments_archive_pair,
  DROP CONSTRAINT segments_lifecycle_status,
  DROP COLUMN archived_by,
  DROP COLUMN archived_at,
  DROP COLUMN lifecycle_status;

DROP TABLE public.stage_operation_receipts;
DROP TABLE public.tag_catalog_operation_receipts;
DROP FUNCTION public.aicrm_tag_catalog_receipt_complete();
DROP FUNCTION public.aicrm_local_catalog_receipt_transition();
DROP FUNCTION public.aicrm_local_catalog_receipt_complete();
DROP INDEX idx_stages_active_sort;
ALTER TABLE stages
  DROP CONSTRAINT stages_archive_pair,
  DROP COLUMN archived_by,
  DROP COLUMN archived_at;
