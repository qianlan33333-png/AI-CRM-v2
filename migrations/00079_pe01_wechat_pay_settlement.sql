-- +goose Up
-- PE01 keeps financial facts in Order, entitlement/member facts in Product and
-- provider execution control in External Effects Runtime. Provider bodies,
-- payer identifiers and credentials are deliberately absent from this schema.

ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync',
    'survey_webhook','audience_webhook','order_payment_prepay',
    'order_payment_capture','order_refund'
  ));

ALTER TABLE public.order_operation_receipts
  DROP CONSTRAINT order_operation_receipts_operation,
  ADD CONSTRAINT order_operation_receipts_operation CHECK (operation IN (
    'export','refund','external_effect_retry','pe01.checkout',
    'pe01.payment_callback','pe01.refund','pe01.refund_callback',
    'pe01.reconcile'
  ));

ALTER TABLE public.order_list_projections
  ADD COLUMN pe01_contract_version TEXT,
  ADD COLUMN product_version BIGINT,
  ADD COLUMN product_kind TEXT,
  ADD COLUMN payment_identity_digest BYTEA,
  ADD COLUMN settled_amount_minor BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN refunded_amount_minor BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN settlement_receipt_digest BYTEA,
  ADD COLUMN paid_at TIMESTAMPTZ,
  ADD COLUMN fully_refunded_at TIMESTAMPTZ,
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1,
  ADD CONSTRAINT order_list_projections_pe01_version CHECK (
    pe01_contract_version IS NULL OR pe01_contract_version = 'pe01/v1'
  ),
  ADD CONSTRAINT order_list_projections_product_version CHECK (
    product_version IS NULL OR product_version > 0
  ),
  ADD CONSTRAINT order_list_projections_product_kind CHECK (
    product_kind IS NULL OR product_kind IN ('ordinary','service_period')
  ),
  ADD CONSTRAINT order_list_projections_payment_identity CHECK (
    payment_identity_digest IS NULL OR octet_length(payment_identity_digest) = 32
  ),
  ADD CONSTRAINT order_list_projections_financial_amounts CHECK (
    settled_amount_minor >= 0 AND refunded_amount_minor >= 0
    AND settled_amount_minor <= amount_minor
    AND refunded_amount_minor <= settled_amount_minor
  ),
  ADD CONSTRAINT order_list_projections_settlement_digest CHECK (
    settlement_receipt_digest IS NULL OR octet_length(settlement_receipt_digest) = 32
  ),
  ADD CONSTRAINT order_list_projections_financial_time CHECK (
    (paid_at IS NULL OR paid_at >= created_at)
    AND (fully_refunded_at IS NULL OR paid_at IS NOT NULL AND fully_refunded_at >= paid_at)
  ),
  ADD CONSTRAINT order_list_projections_pe01_lifecycle CHECK (
    pe01_contract_version IS NULL OR (
      provider = 'wechat' AND customer_id IS NOT NULL AND product_id IS NOT NULL
      AND product_version IS NOT NULL AND product_kind IS NOT NULL
      AND payment_identity_digest IS NOT NULL AND amount_minor > 0 AND currency = 'CNY'
      AND payer_name_snapshot = '' AND mobile_snapshot = ''
      AND identity_kind = '' AND identity_value = ''
      AND status IN ('awaiting_prepay','awaiting_payment','paid','partially_refunded','refunded')
      AND (
        status IN ('awaiting_prepay','awaiting_payment')
        AND settled_amount_minor = 0 AND refunded_amount_minor = 0
        AND settlement_receipt_digest IS NULL AND paid_at IS NULL AND fully_refunded_at IS NULL
        OR status = 'paid'
        AND settled_amount_minor = amount_minor AND refunded_amount_minor = 0
        AND settlement_receipt_digest IS NOT NULL AND paid_at IS NOT NULL AND fully_refunded_at IS NULL
        OR status = 'partially_refunded'
        AND settled_amount_minor = amount_minor AND refunded_amount_minor > 0 AND refunded_amount_minor < amount_minor
        AND settlement_receipt_digest IS NOT NULL AND paid_at IS NOT NULL AND fully_refunded_at IS NULL
        OR status = 'refunded'
        AND settled_amount_minor = amount_minor AND refunded_amount_minor = amount_minor
        AND settlement_receipt_digest IS NOT NULL AND paid_at IS NOT NULL AND fully_refunded_at IS NOT NULL
      )
    )
  ),
  ADD CONSTRAINT order_list_projections_version CHECK (version > 0);

CREATE TABLE public.order_payment_commands (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  order_id BIGINT NOT NULL UNIQUE REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  external_effect_id BIGINT UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  source_ref_digest BYTEA NOT NULL UNIQUE,
  target_ref_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  policy_version_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'accepted',
  provider_prepay_digest BYTEA,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT order_payment_commands_digests CHECK (
    octet_length(source_ref_digest) = 32 AND octet_length(target_ref_digest) = 32
    AND octet_length(payload_digest) = 32 AND octet_length(policy_version_digest) = 32
    AND (provider_prepay_digest IS NULL OR octet_length(provider_prepay_digest) = 32)
  ),
  CONSTRAINT order_payment_commands_state CHECK (state IN (
    'accepted','queued','prepay_ready','outcome_unknown','reconciled','final_failed'
  )),
  CONSTRAINT order_payment_commands_prepay CHECK (
    (state = 'prepay_ready' AND provider_prepay_digest IS NOT NULL)
    OR (state <> 'prepay_ready')
  ),
  CONSTRAINT order_payment_commands_version CHECK (version > 0),
  CONSTRAINT order_payment_commands_time CHECK (updated_at >= created_at)
);
CREATE INDEX order_payment_commands_state_updated_idx
  ON public.order_payment_commands (state, updated_at, id);

CREATE TABLE public.order_financial_refunds (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  external_effect_id BIGINT UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  out_refund_no TEXT NOT NULL UNIQUE,
  amount_minor BIGINT NOT NULL,
  currency CHAR(3) NOT NULL,
  reason TEXT NOT NULL,
  source_ref_digest BYTEA NOT NULL UNIQUE,
  target_ref_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  policy_version_digest BYTEA NOT NULL,
  provider_refund_digest BYTEA,
  settlement_receipt_digest BYTEA,
  state TEXT NOT NULL DEFAULT 'accepted',
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  settled_at TIMESTAMPTZ,
  CONSTRAINT order_financial_refunds_number CHECK (
    out_refund_no ~ '^pe01r_[A-Za-z0-9_-]{16,64}$'
  ),
  CONSTRAINT order_financial_refunds_amount CHECK (amount_minor > 0),
  CONSTRAINT order_financial_refunds_currency CHECK (currency = 'CNY'),
  CONSTRAINT order_financial_refunds_reason CHECK (
    btrim(reason) = reason AND reason <> '' AND char_length(reason) <= 500
  ),
  CONSTRAINT order_financial_refunds_digests CHECK (
    octet_length(source_ref_digest) = 32 AND octet_length(target_ref_digest) = 32
    AND octet_length(payload_digest) = 32 AND octet_length(policy_version_digest) = 32
    AND (provider_refund_digest IS NULL OR octet_length(provider_refund_digest) = 32)
    AND (settlement_receipt_digest IS NULL OR octet_length(settlement_receipt_digest) = 32)
  ),
  CONSTRAINT order_financial_refunds_state CHECK (state IN (
    'accepted','queued','executed','outcome_unknown','reconciled','succeeded','final_failed'
  )),
  CONSTRAINT order_financial_refunds_settlement CHECK (
    (state = 'succeeded' AND provider_refund_digest IS NOT NULL
      AND settlement_receipt_digest IS NOT NULL AND settled_at IS NOT NULL)
    OR (state <> 'succeeded' AND settlement_receipt_digest IS NULL AND settled_at IS NULL)
  ),
  CONSTRAINT order_financial_refunds_version CHECK (version > 0),
  CONSTRAINT order_financial_refunds_time CHECK (
    updated_at >= created_at AND (settled_at IS NULL OR settled_at >= created_at)
  )
);
CREATE INDEX order_financial_refunds_order_state_idx
  ON public.order_financial_refunds (order_id, state, id);

CREATE TABLE public.order_provider_callback_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  callback_kind TEXT NOT NULL,
  provider_event_digest BYTEA NOT NULL UNIQUE,
  payload_digest BYTEA NOT NULL,
  order_id BIGINT NOT NULL REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  refund_id BIGINT REFERENCES public.order_financial_refunds(id) ON DELETE RESTRICT,
  outcome TEXT,
  result_digest BYTEA,
  state TEXT NOT NULL DEFAULT 'reserved',
  received_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT order_provider_callbacks_kind CHECK (callback_kind IN ('payment','refund')),
  CONSTRAINT order_provider_callbacks_digests CHECK (
    octet_length(provider_event_digest) = 32 AND octet_length(payload_digest) = 32
    AND (result_digest IS NULL OR octet_length(result_digest) = 32)
  ),
  CONSTRAINT order_provider_callbacks_target CHECK (
    callback_kind = 'payment' AND refund_id IS NULL
    OR callback_kind = 'refund' AND refund_id IS NOT NULL
  ),
  CONSTRAINT order_provider_callbacks_state CHECK (state IN ('reserved','completed')),
  CONSTRAINT order_provider_callbacks_completion CHECK (
    state = 'reserved' AND outcome IS NULL AND result_digest IS NULL AND completed_at IS NULL
    OR state = 'completed' AND outcome IN ('applied','rejected')
      AND result_digest IS NOT NULL AND completed_at IS NOT NULL
  )
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_pe01_callback_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.order_provider_callback_receipts
    WHERE id = NEW.id AND state = 'completed' AND outcome IS NOT NULL
      AND result_digest IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'PE01 callback receipt must complete in its financial transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER order_provider_callbacks_complete_before_commit
AFTER INSERT OR UPDATE ON public.order_provider_callback_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_pe01_callback_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_pe01_callback_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'PE01 callback receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed'
     OR NEW.callback_kind IS DISTINCT FROM OLD.callback_kind
     OR NEW.provider_event_digest IS DISTINCT FROM OLD.provider_event_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.order_id IS DISTINCT FROM OLD.order_id
     OR NEW.refund_id IS DISTINCT FROM OLD.refund_id
     OR NEW.received_at IS DISTINCT FROM OLD.received_at THEN
    RAISE EXCEPTION 'invalid PE01 callback receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER order_provider_callbacks_transition
BEFORE UPDATE OR DELETE ON public.order_provider_callback_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_pe01_callback_transition_valid();

CREATE TABLE public.order_financial_reconciliations (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  external_effect_id BIGINT NOT NULL REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  evidence_digest BYTEA NOT NULL,
  result_digest BYTEA NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('prepay_ready','payment_paid','refund_succeeded','final_failed')),
  recorded_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT order_financial_reconciliations_digests CHECK (
    octet_length(evidence_digest) = 32 AND octet_length(result_digest) = 32
  ),
  UNIQUE (external_effect_id, evidence_digest)
);

ALTER TABLE public.product_local_entitlements
  ALTER COLUMN granted_by DROP NOT NULL,
  DROP CONSTRAINT product_local_entitlements_granted_by,
  DROP CONSTRAINT product_local_entitlements_revoked_by,
  DROP CONSTRAINT product_local_entitlements_lifecycle,
  ADD COLUMN source TEXT NOT NULL DEFAULT 'manual',
  ADD COLUMN settlement_receipt_digest BYTEA,
  ADD COLUMN granted_actor_scope TEXT,
  ADD COLUMN revoked_actor_scope TEXT,
  ADD CONSTRAINT product_local_entitlements_source CHECK (source IN ('manual','paid_order')),
  ADD CONSTRAINT product_local_entitlements_settlement_lineage CHECK (
    source = 'manual' AND settlement_receipt_digest IS NULL
      AND granted_by IS NOT NULL AND granted_by > 0 AND granted_actor_scope IS NULL
      AND (revoked_by IS NULL OR revoked_by > 0) AND revoked_actor_scope IS NULL
    OR source = 'paid_order' AND settlement_receipt_digest IS NOT NULL
      AND octet_length(settlement_receipt_digest) = 32
      AND granted_by IS NULL AND granted_actor_scope = 'provider:wechat'
      AND revoked_by IS NULL
      AND (revoked_actor_scope IS NULL OR revoked_actor_scope = 'provider:wechat')
  ),
  ADD CONSTRAINT product_local_entitlements_lifecycle CHECK (
    state = 'active' AND revoked_by IS NULL AND revoked_actor_scope IS NULL AND revoked_at IS NULL
    OR state = 'revoked' AND revoked_at IS NOT NULL
      AND (revoked_by IS NOT NULL OR revoked_actor_scope IS NOT NULL)
  );

ALTER TABLE public.service_period_members
  ADD COLUMN pe01_lineage_version TEXT,
  ADD COLUMN paid_order_id BIGINT REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  ADD COLUMN entitlement_id BIGINT REFERENCES public.product_local_entitlements(id) ON DELETE RESTRICT,
  ADD COLUMN settlement_receipt_digest BYTEA,
  ADD CONSTRAINT service_period_members_pe01_lineage CHECK (
    pe01_lineage_version IS NULL OR (
      pe01_lineage_version = 'pe01/v1' AND source = 'paid_order'
      AND paid_order_id IS NOT NULL AND entitlement_id IS NOT NULL
      AND settlement_receipt_digest IS NOT NULL
      AND octet_length(settlement_receipt_digest) = 32
    )
  );
CREATE UNIQUE INDEX service_period_members_pe01_order_once
  ON public.service_period_members (paid_order_id) WHERE pe01_lineage_version = 'pe01/v1';
CREATE UNIQUE INDEX service_period_members_pe01_entitlement_once
  ON public.service_period_members (entitlement_id) WHERE pe01_lineage_version = 'pe01/v1';

-- +goose Down
LOCK TABLE public.order_payment_commands,
  public.order_financial_refunds,
  public.order_provider_callback_receipts,
  public.order_financial_reconciliations,
  public.order_list_projections,
  public.order_operation_receipts,
  public.product_local_entitlements,
  public.service_period_members,
  public.external_effects IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.order_payment_commands)
     OR EXISTS (SELECT 1 FROM public.order_financial_refunds)
     OR EXISTS (SELECT 1 FROM public.order_provider_callback_receipts)
     OR EXISTS (SELECT 1 FROM public.order_financial_reconciliations)
     OR EXISTS (SELECT 1 FROM public.order_list_projections WHERE pe01_contract_version IS NOT NULL)
     OR EXISTS (SELECT 1 FROM public.order_operation_receipts WHERE operation LIKE 'pe01.%')
     OR EXISTS (SELECT 1 FROM public.product_local_entitlements WHERE source = 'paid_order')
     OR EXISTS (SELECT 1 FROM public.service_period_members WHERE pe01_lineage_version IS NOT NULL)
     OR EXISTS (SELECT 1 FROM public.external_effects WHERE kind = 'order_payment_prepay') THEN
    RAISE EXCEPTION 'cannot roll back materialized PE01 financial or entitlement facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

DROP INDEX public.service_period_members_pe01_entitlement_once;
DROP INDEX public.service_period_members_pe01_order_once;
ALTER TABLE public.service_period_members
  DROP CONSTRAINT service_period_members_pe01_lineage,
  DROP COLUMN settlement_receipt_digest,
  DROP COLUMN entitlement_id,
  DROP COLUMN paid_order_id,
  DROP COLUMN pe01_lineage_version;
ALTER TABLE public.product_local_entitlements
  DROP CONSTRAINT product_local_entitlements_lifecycle,
  DROP CONSTRAINT product_local_entitlements_settlement_lineage,
  DROP CONSTRAINT product_local_entitlements_source,
  DROP COLUMN revoked_actor_scope,
  DROP COLUMN granted_actor_scope,
  DROP COLUMN settlement_receipt_digest,
  DROP COLUMN source,
  ALTER COLUMN granted_by SET NOT NULL,
  ADD CONSTRAINT product_local_entitlements_granted_by CHECK (granted_by > 0),
  ADD CONSTRAINT product_local_entitlements_revoked_by CHECK (revoked_by IS NULL OR revoked_by > 0),
  ADD CONSTRAINT product_local_entitlements_lifecycle CHECK (
    (state = 'active' AND revoked_by IS NULL AND revoked_at IS NULL) OR
    (state = 'revoked' AND revoked_by IS NOT NULL AND revoked_at IS NOT NULL)
  );
DROP TABLE public.order_financial_reconciliations;
DROP TRIGGER order_provider_callbacks_transition ON public.order_provider_callback_receipts;
DROP FUNCTION public.aicrm_pe01_callback_transition_valid();
DROP TRIGGER order_provider_callbacks_complete_before_commit ON public.order_provider_callback_receipts;
DROP FUNCTION public.aicrm_pe01_callback_complete_before_commit();
DROP TABLE public.order_provider_callback_receipts;
DROP TABLE public.order_financial_refunds;
DROP TABLE public.order_payment_commands;
ALTER TABLE public.order_list_projections
  DROP CONSTRAINT order_list_projections_version,
  DROP CONSTRAINT order_list_projections_pe01_lifecycle,
  DROP CONSTRAINT order_list_projections_financial_time,
  DROP CONSTRAINT order_list_projections_settlement_digest,
  DROP CONSTRAINT order_list_projections_financial_amounts,
  DROP CONSTRAINT order_list_projections_payment_identity,
  DROP CONSTRAINT order_list_projections_product_kind,
  DROP CONSTRAINT order_list_projections_product_version,
  DROP CONSTRAINT order_list_projections_pe01_version,
  DROP COLUMN version,
  DROP COLUMN fully_refunded_at,
  DROP COLUMN paid_at,
  DROP COLUMN settlement_receipt_digest,
  DROP COLUMN refunded_amount_minor,
  DROP COLUMN settled_amount_minor,
  DROP COLUMN payment_identity_digest,
  DROP COLUMN product_kind,
  DROP COLUMN product_version,
  DROP COLUMN pe01_contract_version;
ALTER TABLE public.order_operation_receipts
  DROP CONSTRAINT order_operation_receipts_operation,
  ADD CONSTRAINT order_operation_receipts_operation CHECK (
    operation IN ('export','refund','external_effect_retry')
  );
ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync',
    'survey_webhook','audience_webhook','order_payment_capture','order_refund'
  ));
