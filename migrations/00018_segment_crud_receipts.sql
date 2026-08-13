-- +goose Up
CREATE TABLE public.segment_operation_receipts (
  id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation             TEXT NOT NULL,
  actor_scope           TEXT NOT NULL,
  key_digest            BYTEA NOT NULL,
  payload_digest        BYTEA NOT NULL,
  state                 TEXT NOT NULL DEFAULT 'in_progress',
  result_segment_id     BIGINT REFERENCES public.segments(id),
  created_at            TIMESTAMPTZ NOT NULL,
  completed_at          TIMESTAMPTZ,
  CONSTRAINT segment_operation_receipts_operation CHECK (operation IN ('create', 'update')),
  CONSTRAINT segment_operation_receipts_actor CHECK (
    btrim(actor_scope) = actor_scope
    AND actor_scope <> ''
    AND char_length(actor_scope) <= 200
  ),
  CONSTRAINT segment_operation_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT segment_operation_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT segment_operation_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT segment_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_segment_id IS NULL AND completed_at IS NULL)
    OR
    (state = 'completed' AND result_segment_id IS NOT NULL AND result_segment_id > 0 AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_segment_receipt()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM public.segment_operation_receipts
    WHERE id = NEW.id
      AND state = 'completed'
      AND result_segment_id IS NOT NULL
      AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'segment operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER segment_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.segment_operation_receipts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_segment_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_segment_receipt_transition_valid()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed segment operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed'
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid segment operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER segment_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.segment_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_segment_receipt_transition_valid();

-- +goose Down
DROP TRIGGER segment_operation_receipts_transition ON public.segment_operation_receipts;
DROP TRIGGER segment_operation_receipts_complete_before_commit ON public.segment_operation_receipts;
DROP TABLE public.segment_operation_receipts;
DROP FUNCTION public.aicrm_segment_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_segment_receipt();
