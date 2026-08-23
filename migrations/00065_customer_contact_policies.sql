-- +goose Up
-- Contact owns the local customer-contact suppression policy. The schema is
-- deliberately OneID-only and contains no phone, unionid, provider, outbound,
-- task, target, or delivery fact.
CREATE TABLE public.customer_contact_policies (
  customer_id BIGINT PRIMARY KEY REFERENCES public.customers(id) ON DELETE RESTRICT,
  reason_code TEXT NOT NULL,
  suppressed_until TIMESTAMPTZ,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT customer_contact_policies_reason CHECK (
    reason_code IN ('manual_opt_out', 'compliance_hold', 'operator_hold')
  ),
  CONSTRAINT customer_contact_policies_version CHECK (version >= 1),
  CONSTRAINT customer_contact_policies_timestamps CHECK (updated_at >= created_at)
);

CREATE TABLE public.customer_contact_policy_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'reserved',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT customer_contact_policy_receipts_operation CHECK (
    operation IN ('customer_contact_policy.set', 'customer_contact_policy.clear')
  ),
  CONSTRAINT customer_contact_policy_receipts_actor CHECK (
    actor_scope ~ '^customer_contact_policy:actor:[1-9][0-9]*$'
  ),
  CONSTRAINT customer_contact_policy_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT customer_contact_policy_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT customer_contact_policy_receipts_state CHECK (state IN ('reserved', 'completed')),
  CONSTRAINT customer_contact_policy_receipts_completion CHECK (
    (state = 'reserved' AND result_snapshot IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_customer_contact_policy_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.customer_contact_policy_operation_receipts
    WHERE id = NEW.id AND state = 'completed'
      AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'customer contact policy receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER customer_contact_policy_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.customer_contact_policy_operation_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_customer_contact_policy_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_customer_contact_policy_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed customer contact policy receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed'
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid customer contact policy receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER customer_contact_policy_receipts_transition
BEFORE UPDATE OR DELETE ON public.customer_contact_policy_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_customer_contact_policy_receipt_transition_valid();

COMMENT ON TABLE public.customer_contact_policies IS
  'Contact-owned local suppression policy keyed only by canonical customer OneID.';

-- +goose Down
LOCK TABLE public.customer_contact_policies,
  public.customer_contact_policy_operation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.customer_contact_policies)
     OR EXISTS (SELECT 1 FROM public.customer_contact_policy_operation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back customer contact policy facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER customer_contact_policy_receipts_transition ON public.customer_contact_policy_operation_receipts;
DROP FUNCTION public.aicrm_customer_contact_policy_receipt_transition_valid();
DROP TRIGGER customer_contact_policy_receipts_complete_before_commit ON public.customer_contact_policy_operation_receipts;
DROP FUNCTION public.aicrm_customer_contact_policy_receipt_complete_before_commit();
DROP TABLE public.customer_contact_policy_operation_receipts;
DROP TABLE public.customer_contact_policies;
