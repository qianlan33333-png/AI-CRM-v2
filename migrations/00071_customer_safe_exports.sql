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
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed customer safe export receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.actor_id IS DISTINCT FROM OLD.actor_id OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.state <> 'completed' OR NEW.export_id IS NULL OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid customer safe export receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER customer_safe_export_receipts_transition BEFORE UPDATE OR DELETE ON public.customer_safe_export_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_customer_safe_export_receipt_transition_valid();
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
DROP TABLE public.customer_safe_export_receipts;
DROP TABLE public.customer_safe_export_rows;
DROP TABLE public.customer_safe_exports;
