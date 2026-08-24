-- +goose Up
CREATE TABLE public.internal_event_safe_exports (
  id TEXT PRIMARY KEY CHECK (id ~ '^ese_[0-9a-f]{32}$'),
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  filter_digest BYTEA NOT NULL CHECK (octet_length(filter_digest) = 32),
  watermark TIMESTAMPTZ NOT NULL,
  upper_event_id BIGINT NOT NULL CHECK (upper_event_id >= 0),
  record_count INTEGER NOT NULL CHECK (record_count >= 0 AND record_count <= 10000),
  created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE public.internal_event_safe_export_rows (
  export_id TEXT NOT NULL REFERENCES public.internal_event_safe_exports(id) ON DELETE RESTRICT,
  row_index INTEGER NOT NULL CHECK (row_index > 0 AND row_index <= 10000),
  event_id BIGINT NOT NULL REFERENCES public.event_log(id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  dispatched BOOLEAN NOT NULL,
  consumer TEXT,
  status TEXT,
  attempt_count INTEGER,
  completed_at TIMESTAMPTZ,
  PRIMARY KEY (export_id, row_index),
  UNIQUE (export_id, event_id, consumer),
  CHECK ((consumer IS NULL AND status IS NULL AND attempt_count IS NULL AND completed_at IS NULL)
      OR (consumer IS NOT NULL AND status IN ('pending','processing','completed','final_failed','outcome_unknown')
          AND attempt_count >= 0))
);
CREATE TABLE public.internal_event_safe_export_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  export_id TEXT REFERENCES public.internal_event_safe_exports(id) ON DELETE RESTRICT,
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved','completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  UNIQUE (actor_id, key_digest),
  CHECK ((state='reserved' AND export_id IS NULL AND result_snapshot IS NULL AND completed_at IS NULL)
      OR (state='completed' AND export_id IS NOT NULL AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL))
);
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_internal_event_safe_export_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'internal event safe export facts are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER internal_event_safe_exports_immutable BEFORE UPDATE OR DELETE ON public.internal_event_safe_exports
FOR EACH ROW EXECUTE FUNCTION public.aicrm_internal_event_safe_export_immutable();
CREATE TRIGGER internal_event_safe_export_rows_immutable BEFORE UPDATE OR DELETE ON public.internal_event_safe_export_rows
FOR EACH ROW EXECUTE FUNCTION public.aicrm_internal_event_safe_export_immutable();
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_internal_event_safe_export_row_insert_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  -- Completion takes the same parent-row lock before counting rows.  Taking it
  -- here serializes the final row append with completion under READ COMMITTED.
  PERFORM 1 FROM public.internal_event_safe_exports WHERE id=NEW.export_id FOR KEY SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'unknown internal event safe export' USING ERRCODE = '55000';
  END IF;
  IF EXISTS (SELECT 1 FROM public.internal_event_safe_export_receipts
      WHERE export_id=NEW.export_id AND state='completed') THEN
    RAISE EXCEPTION 'cannot append completed internal event safe export rows' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER internal_event_safe_export_rows_insert_guard BEFORE INSERT ON public.internal_event_safe_export_rows
FOR EACH ROW EXECUTE FUNCTION public.aicrm_internal_event_safe_export_row_insert_guard();
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_internal_event_safe_export_receipt_transition()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  export_count INTEGER;
  export_actor BIGINT;
  export_digest BYTEA;
  export_watermark TIMESTAMPTZ;
  export_created TIMESTAMPTZ;
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed internal event safe export receipt is immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.actor_id IS DISTINCT FROM OLD.actor_id OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.state <> 'completed' OR NEW.export_id IS NULL OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid internal event safe export receipt transition' USING ERRCODE = '55000';
  END IF;
  SELECT actor_id,record_count,filter_digest,watermark,created_at
    INTO export_actor,export_count,export_digest,export_watermark,export_created
  FROM public.internal_event_safe_exports WHERE id=NEW.export_id FOR UPDATE;
  IF NOT FOUND OR export_actor <> OLD.actor_id
     OR (SELECT count(*) FROM public.internal_event_safe_export_rows WHERE export_id=NEW.export_id) <> export_count THEN
    RAISE EXCEPTION 'internal event safe export receipt/header mismatch' USING ERRCODE = '55000';
  END IF;
  IF jsonb_typeof(NEW.result_snapshot) <> 'object'
     OR NOT (NEW.result_snapshot ?& ARRAY['id','record_count','watermark','created_at'])
     OR NEW.result_snapshot - ARRAY['id','record_count','watermark','created_at'] <> '{}'::jsonb
     OR NEW.result_snapshot->>'id' <> NEW.export_id
     OR NEW.result_snapshot->>'record_count' <> export_count::text
     OR (NEW.result_snapshot->>'watermark')::timestamptz <> export_watermark
     OR (NEW.result_snapshot->>'created_at')::timestamptz <> export_created THEN
    RAISE EXCEPTION 'internal event safe export result snapshot mismatch' USING ERRCODE = '55000';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM public.event_log WHERE event_type='events.safe_export_created'
      AND idempotency_key='internal-event-safe-export:' || OLD.id::text
      AND payload->>'export_id'=NEW.export_id AND payload->>'record_count'=export_count::text
      AND payload->>'filter_digest'=encode(export_digest,'hex')) THEN
    RAISE EXCEPTION 'internal event safe export completion requires audit fact' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER internal_event_safe_export_receipts_transition BEFORE UPDATE OR DELETE ON public.internal_event_safe_export_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_internal_event_safe_export_receipt_transition();
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_internal_event_safe_export_receipt_must_complete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.internal_event_safe_export_receipts WHERE id=NEW.id AND state <> 'completed') THEN
    RAISE EXCEPTION 'internal event safe export receipts must complete in creating transaction' USING ERRCODE = '55000';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER internal_event_safe_export_receipts_must_complete
AFTER INSERT OR UPDATE ON public.internal_event_safe_export_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_internal_event_safe_export_receipt_must_complete();
COMMENT ON TABLE public.internal_event_safe_exports IS 'Events-owned actor-bound local event_log/event_deliveries safe snapshots; no payload, PII, River, outbound, provider, or external-delivery claim.';
-- +goose Down
LOCK TABLE public.internal_event_safe_export_receipts, public.internal_event_safe_export_rows, public.internal_event_safe_exports IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.internal_event_safe_exports)
     OR EXISTS (SELECT 1 FROM public.internal_event_safe_export_rows)
     OR EXISTS (SELECT 1 FROM public.internal_event_safe_export_receipts) THEN
    RAISE EXCEPTION 'cannot roll back internal event safe export facts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER internal_event_safe_export_receipts_must_complete ON public.internal_event_safe_export_receipts;
DROP FUNCTION public.aicrm_internal_event_safe_export_receipt_must_complete();
DROP TRIGGER internal_event_safe_export_receipts_transition ON public.internal_event_safe_export_receipts;
DROP FUNCTION public.aicrm_internal_event_safe_export_receipt_transition();
DROP TRIGGER internal_event_safe_export_rows_insert_guard ON public.internal_event_safe_export_rows;
DROP FUNCTION public.aicrm_internal_event_safe_export_row_insert_guard();
DROP TRIGGER internal_event_safe_export_rows_immutable ON public.internal_event_safe_export_rows;
DROP TRIGGER internal_event_safe_exports_immutable ON public.internal_event_safe_exports;
DROP FUNCTION public.aicrm_internal_event_safe_export_immutable();
DROP TABLE public.internal_event_safe_export_receipts;
DROP TABLE public.internal_event_safe_export_rows;
DROP TABLE public.internal_event_safe_exports;
