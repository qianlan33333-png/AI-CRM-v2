-- +goose Up
-- Native v2 product CAS and local entitlement facts. This migration is local
-- only: it neither derives payment outcomes nor calls a provider.
ALTER TABLE public.products
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1,
  ADD CONSTRAINT products_version CHECK (version >= 1);

CREATE TABLE public.product_local_entitlements (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  product_id BIGINT NOT NULL REFERENCES public.products(id) ON DELETE RESTRICT,
  order_id BIGINT NOT NULL REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  customer_id BIGINT NOT NULL REFERENCES public.customers(id) ON DELETE RESTRICT,
  state TEXT NOT NULL DEFAULT 'active',
  version BIGINT NOT NULL DEFAULT 1,
  granted_by BIGINT NOT NULL,
  granted_at TIMESTAMPTZ NOT NULL,
  revoked_by BIGINT,
  revoked_at TIMESTAMPTZ,
  CONSTRAINT product_local_entitlements_order UNIQUE (order_id),
  CONSTRAINT product_local_entitlements_product CHECK (product_id > 0),
  CONSTRAINT product_local_entitlements_order_id CHECK (order_id > 0),
  CONSTRAINT product_local_entitlements_customer CHECK (customer_id > 0),
  CONSTRAINT product_local_entitlements_state CHECK (state IN ('active', 'revoked')),
  CONSTRAINT product_local_entitlements_version CHECK (version >= 1),
  CONSTRAINT product_local_entitlements_granted_by CHECK (granted_by > 0),
  CONSTRAINT product_local_entitlements_revoked_by CHECK (revoked_by IS NULL OR revoked_by > 0),
  CONSTRAINT product_local_entitlements_lifecycle CHECK (
    (state = 'active' AND revoked_by IS NULL AND revoked_at IS NULL) OR
    (state = 'revoked' AND revoked_by IS NOT NULL AND revoked_at IS NOT NULL)
  )
);
CREATE INDEX product_local_entitlements_product_id_idx
  ON public.product_local_entitlements (product_id, id DESC);

CREATE TABLE public.entitlement_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT entitlement_operation_receipts_operation CHECK (operation IN ('grant', 'revoke')),
  CONSTRAINT entitlement_operation_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT entitlement_operation_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT entitlement_operation_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT entitlement_operation_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT entitlement_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_entitlement_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.entitlement_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'entitlement operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER entitlement_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.entitlement_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_entitlement_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_entitlement_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed entitlement operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid entitlement operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER entitlement_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.entitlement_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_entitlement_receipt_transition_valid();

ALTER TABLE public.product_operation_receipts
  DROP CONSTRAINT product_operation_receipts_operation,
  ADD CONSTRAINT product_operation_receipts_operation CHECK (
    operation IN ('create', 'update')
  );

-- +goose Down
-- The pre-00050 schema cannot represent update receipts. Never erase completed
-- idempotency/audit facts to make a rollback fit the older constraint: require
-- an explicit operator decision instead.
-- Serialize the check with every Product/Entitlement writer. These locks are
-- held by the migration transaction through the subsequent DDL, so a writer
-- cannot commit a new fact after validation but before its table is dropped.
LOCK TABLE public.product_operation_receipts,
  public.product_local_entitlements,
  public.entitlement_operation_receipts,
  public.products
IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.product_operation_receipts WHERE operation = 'update'
  ) OR EXISTS (
    SELECT 1 FROM public.product_local_entitlements
  ) OR EXISTS (
    SELECT 1 FROM public.entitlement_operation_receipts
  ) OR EXISTS (
    SELECT 1 FROM public.products WHERE version <> 1
  ) THEN
    RAISE EXCEPTION 'cannot roll back product versions while versioned product or entitlement facts exist'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
ALTER TABLE public.product_operation_receipts
  DROP CONSTRAINT product_operation_receipts_operation,
  ADD CONSTRAINT product_operation_receipts_operation CHECK (operation = 'create');
DROP TRIGGER entitlement_operation_receipts_transition ON public.entitlement_operation_receipts;
DROP TRIGGER entitlement_operation_receipts_complete_before_commit ON public.entitlement_operation_receipts;
DROP FUNCTION public.aicrm_entitlement_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_entitlement_receipt();
DROP TABLE public.entitlement_operation_receipts;
DROP TABLE public.product_local_entitlements;
ALTER TABLE public.products DROP CONSTRAINT products_version;
ALTER TABLE public.products DROP COLUMN version;
