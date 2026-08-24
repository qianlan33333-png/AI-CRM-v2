-- +goose Up
-- Contact owns sidebar profile writes. The receipt stores only a local result
-- snapshot and never contains external identity values or provider facts.
CREATE TABLE public.sidebar_customer_profile_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'reserved',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT sidebar_customer_profile_receipts_actor CHECK (
    actor_scope ~ '^sidebar_customer_profile:actor:[1-9][0-9]*$'
  ),
  CONSTRAINT sidebar_customer_profile_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT sidebar_customer_profile_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT sidebar_customer_profile_receipts_state CHECK (state IN ('reserved', 'completed')),
  CONSTRAINT sidebar_customer_profile_receipts_completion CHECK (
    (state = 'reserved' AND result_snapshot IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_sidebar_customer_profile_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.sidebar_customer_profile_operation_receipts
    WHERE id = NEW.id AND state = 'completed'
      AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'sidebar customer profile receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER sidebar_customer_profile_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.sidebar_customer_profile_operation_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_sidebar_customer_profile_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_sidebar_customer_profile_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed sidebar customer profile receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed'
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid sidebar customer profile receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER sidebar_customer_profile_receipts_transition
BEFORE UPDATE OR DELETE ON public.sidebar_customer_profile_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_sidebar_customer_profile_receipt_transition_valid();

-- +goose Down
LOCK TABLE public.sidebar_customer_profile_operation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.sidebar_customer_profile_operation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back sidebar customer profile receipts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER sidebar_customer_profile_receipts_transition ON public.sidebar_customer_profile_operation_receipts;
DROP FUNCTION public.aicrm_sidebar_customer_profile_receipt_transition_valid();
DROP TRIGGER sidebar_customer_profile_receipts_complete_before_commit ON public.sidebar_customer_profile_operation_receipts;
DROP FUNCTION public.aicrm_sidebar_customer_profile_receipt_complete_before_commit();
DROP TABLE public.sidebar_customer_profile_operation_receipts;
