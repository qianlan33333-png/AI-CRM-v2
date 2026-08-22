-- +goose Up
-- Dedicated service-period membership facts. These rows use local OneID only;
-- no phone, unionid, openid, external_userid, payment, refund, or provider
-- identifier belongs in this schema.
CREATE TABLE public.service_period_members (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  member_ref TEXT NOT NULL UNIQUE,
  service_product_id BIGINT NOT NULL REFERENCES public.products(id) ON DELETE RESTRICT,
  customer_id BIGINT NOT NULL REFERENCES public.customers(id) ON DELETE RESTRICT,
  state TEXT NOT NULL DEFAULT 'active',
  source TEXT NOT NULL,
  starts_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ,
  expired_at TIMESTAMPTZ,
  removed_at TIMESTAMPTZ,
  remark TEXT,
  alliance TEXT,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT service_period_members_ref CHECK (member_ref ~ '^spm_[A-Za-z0-9_-]{22}$'),
  CONSTRAINT service_period_members_state CHECK (state IN ('active', 'expired', 'removed')),
  CONSTRAINT service_period_members_source CHECK (source IN ('manual', 'paid_order')),
  CONSTRAINT service_period_members_version CHECK (version >= 1),
  CONSTRAINT service_period_members_expiry CHECK (expires_at IS NULL OR expires_at >= starts_at),
  CONSTRAINT service_period_members_remark CHECK (
    remark IS NULL OR (btrim(remark) = remark AND remark <> '' AND char_length(remark) <= 500)
  ),
  CONSTRAINT service_period_members_alliance CHECK (
    alliance IS NULL OR (btrim(alliance) = alliance AND alliance <> '' AND char_length(alliance) <= 120)
  ),
  CONSTRAINT service_period_members_timestamps CHECK (updated_at >= created_at AND created_at >= starts_at),
  CONSTRAINT service_period_members_lifecycle CHECK (
    (state = 'active' AND expired_at IS NULL AND removed_at IS NULL) OR
    (state = 'expired' AND expired_at IS NOT NULL AND expired_at >= starts_at AND removed_at IS NULL) OR
    (state = 'removed' AND removed_at IS NOT NULL AND removed_at >= starts_at)
  )
);

CREATE INDEX service_period_members_product_page_idx
  ON public.service_period_members (service_product_id, updated_at DESC, member_ref DESC);
CREATE INDEX service_period_members_customer_idx
  ON public.service_period_members (customer_id, service_product_id, id DESC);

CREATE TABLE public.service_period_member_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'reserved',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT service_period_member_receipts_operation CHECK (operation IN (
    'service_period_member.add',
    'service_period_member.expire',
    'service_period_member.remove',
    'service_period_member.update_fields'
  )),
  CONSTRAINT service_period_member_receipts_actor CHECK (
    actor_scope ~ '^service_period_members:actor:[1-9][0-9]*$'
  ),
  CONSTRAINT service_period_member_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT service_period_member_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT service_period_member_receipts_state CHECK (state IN ('reserved', 'completed')),
  CONSTRAINT service_period_member_receipts_completion CHECK (
    (state = 'reserved' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_service_period_member_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.service_period_member_operation_receipts
    WHERE id = NEW.id AND state = 'completed'
      AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'service-period member receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER service_period_member_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.service_period_member_operation_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_service_period_member_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_service_period_member_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed service-period member receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid service-period member receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER service_period_member_receipts_transition
BEFORE UPDATE OR DELETE ON public.service_period_member_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_service_period_member_receipt_transition_valid();

COMMENT ON TABLE public.service_period_members IS
  'CRM-local service-period members keyed by OneID and opaque member_ref; no external identity or provider facts.';

-- +goose Down
LOCK TABLE public.service_period_members,
  public.service_period_member_operation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.service_period_members)
     OR EXISTS (SELECT 1 FROM public.service_period_member_operation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back service-period member facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER service_period_member_receipts_transition ON public.service_period_member_operation_receipts;
DROP FUNCTION public.aicrm_service_period_member_receipt_transition_valid();
DROP TRIGGER service_period_member_receipts_complete_before_commit ON public.service_period_member_operation_receipts;
DROP FUNCTION public.aicrm_service_period_member_receipt_complete_before_commit();
DROP TABLE public.service_period_member_operation_receipts;
DROP TABLE public.service_period_members;
