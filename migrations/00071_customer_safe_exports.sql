-- +goose Up
CREATE TABLE public.customer_safe_exports (
  id TEXT PRIMARY KEY CHECK (id ~ '^cse_[0-9a-f]{32}$'),
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  owner_scope_staff_id BIGINT REFERENCES public.staff(id) ON DELETE RESTRICT,
  filter_digest BYTEA NOT NULL CHECK (octet_length(filter_digest) = 32),
  watermark TIMESTAMPTZ NOT NULL,
  record_count INTEGER NOT NULL CHECK (record_count >= 0 AND record_count <= 10000),
  created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE public.customer_safe_export_rows (
  export_id TEXT NOT NULL REFERENCES public.customer_safe_exports(id) ON DELETE RESTRICT,
  row_index INTEGER NOT NULL CHECK (row_index > 0 AND row_index <= 10000),
  customer_id BIGINT NOT NULL REFERENCES public.customers(id) ON DELETE RESTRICT,
  display_name TEXT NOT NULL,
  owner_staff_id BIGINT REFERENCES public.staff(id) ON DELETE RESTRICT,
  stage_id BIGINT REFERENCES public.stages(id) ON DELETE RESTRICT,
  channel_id BIGINT REFERENCES public.channels(id) ON DELETE RESTRICT,
  added_at TIMESTAMPTZ,
  last_interact_at TIMESTAMPTZ,
  PRIMARY KEY (export_id, row_index),
  UNIQUE (export_id, customer_id)
);
CREATE TABLE public.customer_safe_export_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  export_id TEXT REFERENCES public.customer_safe_exports(id) ON DELETE RESTRICT,
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved','completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  UNIQUE (actor_id, key_digest),
  CHECK ((state='reserved' AND export_id IS NULL AND result_snapshot IS NULL AND completed_at IS NULL)
      OR (state='completed' AND export_id IS NOT NULL AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL))
);
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_customer_safe_export_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  export_actor_id BIGINT;
  export_record_count INTEGER;
  export_filter_digest BYTEA;
  export_watermark TIMESTAMPTZ;
  export_created_at TIMESTAMPTZ;
  snapshot_row_count INTEGER;
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed customer safe export receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.actor_id IS DISTINCT FROM OLD.actor_id OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.state <> 'completed' OR NEW.export_id IS NULL OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid customer safe export receipt transition' USING ERRCODE = '55000';
  END IF;
  SELECT actor_id,record_count,filter_digest,watermark,created_at
  INTO export_actor_id,export_record_count,export_filter_digest,export_watermark,export_created_at
  FROM public.customer_safe_exports
  WHERE id = NEW.export_id
  FOR UPDATE;
  IF NOT FOUND OR export_actor_id <> OLD.actor_id THEN
    RAISE EXCEPTION 'customer safe export receipt does not match its actor-bound snapshot' USING ERRCODE = '55000';
  END IF;
  SELECT count(*) INTO snapshot_row_count
  FROM public.customer_safe_export_rows
  WHERE export_id = NEW.export_id;
  IF snapshot_row_count <> export_record_count THEN
    RAISE EXCEPTION 'customer safe export snapshot row count does not match header' USING ERRCODE = '55000';
  END IF;
  IF jsonb_typeof(NEW.result_snapshot) <> 'object'
     OR NOT (NEW.result_snapshot ?& ARRAY['id','record_count','watermark','created_at'])
     OR NEW.result_snapshot - ARRAY['id','record_count','watermark','created_at'] <> '{}'::jsonb
     OR NEW.result_snapshot ->> 'id' <> NEW.export_id
     OR jsonb_typeof(NEW.result_snapshot -> 'record_count') <> 'number'
     OR NEW.result_snapshot ->> 'record_count' <> export_record_count::text
     OR jsonb_typeof(NEW.result_snapshot -> 'watermark') <> 'string'
     OR jsonb_typeof(NEW.result_snapshot -> 'created_at') <> 'string' THEN
    RAISE EXCEPTION 'customer safe export result snapshot does not match header' USING ERRCODE = '55000';
  END IF;
  BEGIN
    IF (NEW.result_snapshot ->> 'watermark')::timestamptz <> export_watermark
       OR (NEW.result_snapshot ->> 'created_at')::timestamptz <> export_created_at THEN
      RAISE EXCEPTION 'customer safe export result snapshot does not match header' USING ERRCODE = '55000';
    END IF;
  EXCEPTION WHEN others THEN
    RAISE EXCEPTION 'customer safe export result snapshot does not match header' USING ERRCODE = '55000';
  END;
  IF NOT EXISTS (
    SELECT 1
    FROM public.event_log
    WHERE event_type = 'customer.safe_export_created'
      AND idempotency_key = 'customer-safe-export:' || OLD.id::text
      AND payload ->> 'export_id' = NEW.export_id
      AND payload ->> 'record_count' = export_record_count::text
      AND payload ->> 'filter_digest' = encode(export_filter_digest,'hex')
  ) THEN
    RAISE EXCEPTION 'customer safe export completion requires its matching event fact' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER customer_safe_export_receipts_transition BEFORE UPDATE OR DELETE ON public.customer_safe_export_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_customer_safe_export_receipt_transition_valid();
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_customer_safe_export_receipt_must_complete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM public.customer_safe_export_receipts
    WHERE id = NEW.id AND state <> 'completed'
  ) THEN
    RAISE EXCEPTION 'customer safe export receipts must complete in their creating transaction' USING ERRCODE = '55000';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER customer_safe_export_receipts_must_complete
AFTER INSERT OR UPDATE ON public.customer_safe_export_receipts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_customer_safe_export_receipt_must_complete();
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_customer_safe_export_snapshot_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'customer safe export snapshots are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER customer_safe_exports_immutable BEFORE UPDATE OR DELETE ON public.customer_safe_exports
FOR EACH ROW EXECUTE FUNCTION public.aicrm_customer_safe_export_snapshot_immutable();
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_customer_safe_export_row_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION 'customer safe export rows are immutable' USING ERRCODE = '55000';
  END IF;
  PERFORM 1
  FROM public.customer_safe_exports
  WHERE id = NEW.export_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'customer safe export snapshot does not exist' USING ERRCODE = '55000';
  END IF;
  IF EXISTS (
    SELECT 1 FROM public.customer_safe_export_receipts
    WHERE export_id = NEW.export_id AND state = 'completed'
  ) THEN
    RAISE EXCEPTION 'cannot append completed customer safe export rows' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER customer_safe_export_rows_transition BEFORE INSERT OR UPDATE OR DELETE ON public.customer_safe_export_rows
FOR EACH ROW EXECUTE FUNCTION public.aicrm_customer_safe_export_row_transition_valid();
COMMENT ON TABLE public.customer_safe_exports IS 'Contact-owned actor-bound local customer export snapshots; no provider, external identity, mobile, or outbound fact.';
-- +goose Down
LOCK TABLE public.customer_safe_export_receipts, public.customer_safe_export_rows, public.customer_safe_exports IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.customer_safe_export_receipts) OR EXISTS (SELECT 1 FROM public.customer_safe_export_rows) OR EXISTS (SELECT 1 FROM public.customer_safe_exports) THEN
    RAISE EXCEPTION 'cannot roll back customer safe export facts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER customer_safe_export_receipts_transition ON public.customer_safe_export_receipts;
DROP FUNCTION public.aicrm_customer_safe_export_receipt_transition_valid();
DROP TRIGGER customer_safe_export_receipts_must_complete ON public.customer_safe_export_receipts;
DROP FUNCTION public.aicrm_customer_safe_export_receipt_must_complete();
DROP TRIGGER customer_safe_export_rows_transition ON public.customer_safe_export_rows;
DROP FUNCTION public.aicrm_customer_safe_export_row_transition_valid();
DROP TRIGGER customer_safe_exports_immutable ON public.customer_safe_exports;
DROP FUNCTION public.aicrm_customer_safe_export_snapshot_immutable();
DROP TABLE public.customer_safe_export_receipts;
DROP TABLE public.customer_safe_export_rows;
DROP TABLE public.customer_safe_exports;
