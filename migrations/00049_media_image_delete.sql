-- +goose Up
CREATE TABLE public.media_image_delete_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  business_key TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT media_image_delete_receipts_operation CHECK (operation = 'delete'),
  CONSTRAINT media_image_delete_receipts_actor CHECK (actor_scope ~ '^admin:[1-9][0-9]*$' AND char_length(actor_scope) <= 200),
  CONSTRAINT media_image_delete_receipts_business CHECK (business_key ~ '^[1-9][0-9]*$'),
  CONSTRAINT media_image_delete_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT media_image_delete_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT media_image_delete_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT media_image_delete_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  CONSTRAINT media_image_delete_receipts_operation_actor_key_unique UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_media_image_delete_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.media_image_delete_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'media image delete receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER media_image_delete_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.media_image_delete_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_media_image_delete_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_image_delete_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed media image delete receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.business_key IS DISTINCT FROM OLD.business_key
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid media image delete receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_image_delete_receipts_transition
BEFORE UPDATE OR DELETE ON public.media_image_delete_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_image_delete_receipt_transition_valid();

-- +goose Down
DROP TRIGGER media_image_delete_receipts_transition ON public.media_image_delete_receipts;
DROP TRIGGER media_image_delete_receipts_complete_before_commit ON public.media_image_delete_receipts;
DROP TABLE public.media_image_delete_receipts;
DROP FUNCTION public.aicrm_media_image_delete_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_media_image_delete_receipt();
