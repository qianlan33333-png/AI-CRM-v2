-- +goose Up
CREATE TABLE public.contact_owner_reassignment_previews (
  id TEXT PRIMARY KEY,
  actor_id BIGINT NOT NULL,
  idempotency_key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  preview_hash BYTEA NOT NULL,
  rows JSONB NOT NULL,
  errors JSONB NOT NULL DEFAULT '[]'::jsonb,
  result JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  executed_at TIMESTAMPTZ,
  CONSTRAINT contact_owner_reassignment_preview_id CHECK (id ~ '^cor_[A-Za-z0-9_-]{22}$'),
  CONSTRAINT contact_owner_reassignment_preview_digest CHECK (octet_length(payload_digest) = 32 AND octet_length(idempotency_key_digest) = 32 AND octet_length(preview_hash) = 32),
  CONSTRAINT contact_owner_reassignment_preview_rows_array CHECK (jsonb_typeof(rows) = 'array' AND jsonb_typeof(errors) = 'array' AND (result IS NULL OR jsonb_typeof(result) = 'array')),
  CONSTRAINT contact_owner_reassignment_preview_expiry CHECK (expires_at > created_at),
  CONSTRAINT contact_owner_reassignment_preview_execution CHECK ((executed_at IS NULL AND result IS NULL) OR (executed_at IS NOT NULL AND result IS NOT NULL)),
  UNIQUE (actor_id, idempotency_key_digest)
);

CREATE TABLE public.contact_owner_reassignment_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL,
  idempotency_key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'reserved',
  result JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT contact_owner_reassignment_receipt_key CHECK (octet_length(idempotency_key_digest) = 32 AND octet_length(payload_digest) = 32),
  CONSTRAINT contact_owner_reassignment_receipt_result_array CHECK (result IS NULL OR jsonb_typeof(result) = 'array'),
  CONSTRAINT contact_owner_reassignment_receipt_state CHECK (state IN ('reserved','completed')),
  CONSTRAINT contact_owner_reassignment_receipt_completion CHECK ((state = 'reserved' AND result IS NULL AND completed_at IS NULL) OR (state = 'completed' AND result IS NOT NULL AND completed_at IS NOT NULL)),
  UNIQUE (actor_id, idempotency_key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_contact_owner_reassignment_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.contact_owner_reassignment_operation_receipts WHERE id = NEW.id AND state = 'completed' AND result IS NOT NULL AND completed_at IS NOT NULL) THEN
    RAISE EXCEPTION 'contact owner reassignment receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER contact_owner_reassignment_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.contact_owner_reassignment_operation_receipts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_contact_owner_reassignment_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_contact_owner_reassignment_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' OR NEW.state <> 'completed' OR NEW.actor_id IS DISTINCT FROM OLD.actor_id OR NEW.idempotency_key_digest IS DISTINCT FROM OLD.idempotency_key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.result IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid contact owner reassignment receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER contact_owner_reassignment_receipts_transition
BEFORE UPDATE OR DELETE ON public.contact_owner_reassignment_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_contact_owner_reassignment_receipt_transition_valid();

-- +goose Down
LOCK TABLE public.contact_owner_reassignment_previews, public.contact_owner_reassignment_operation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.contact_owner_reassignment_previews) OR EXISTS (SELECT 1 FROM public.contact_owner_reassignment_operation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back contact owner reassignment facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER contact_owner_reassignment_receipts_transition ON public.contact_owner_reassignment_operation_receipts;
DROP FUNCTION public.aicrm_contact_owner_reassignment_receipt_transition_valid();
DROP TRIGGER contact_owner_reassignment_receipts_complete_before_commit ON public.contact_owner_reassignment_operation_receipts;
DROP FUNCTION public.aicrm_contact_owner_reassignment_receipt_complete_before_commit();
DROP TABLE public.contact_owner_reassignment_operation_receipts;
DROP TABLE public.contact_owner_reassignment_previews;
