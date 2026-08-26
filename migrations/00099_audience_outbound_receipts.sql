-- +goose Up
-- Legacy Audience dispatches select the customer's active relationship owner
-- only when that owner is enabled in the package whitelist. These snapshots
-- are private provider input, not read-model fields, and freeze that decision
-- before the C01 effect is accepted.
ALTER TABLE public.outbound_campaign_dispatches
  ADD COLUMN sender_userid_snapshot TEXT,
  ADD COLUMN external_userid_snapshot TEXT,
  DROP CONSTRAINT outbound_campaign_dispatches_block_reason_check,
  ADD CONSTRAINT outbound_campaign_dispatches_block_reason_check CHECK (
    block_reason IN (
      'external_gate_disabled','identity_unresolved','contact_policy','inactive_customer',
      'provider_preflight_failed','sender_not_allowed','target_unresolved'
    )
  ),
  ADD CONSTRAINT outbound_campaign_dispatches_target_snapshot_shape CHECK (
    (sender_userid_snapshot IS NULL AND external_userid_snapshot IS NULL)
    OR (
      sender_userid_snapshot = btrim(sender_userid_snapshot)
      AND external_userid_snapshot = btrim(external_userid_snapshot)
      AND length(sender_userid_snapshot) BETWEEN 1 AND 128
      AND length(external_userid_snapshot) BETWEEN 1 AND 1024
    )
  );

-- One Provider attempt has exactly one pre-reconciliation outcome. The later
-- reconciliation row remains distinct and append-only.
CREATE UNIQUE INDEX outbound_campaign_provider_attempt_receipts_attempt_once
  ON public.outbound_campaign_provider_attempt_receipts(external_effect_id, attempt_number)
  WHERE completion <> 'reconciled';

ALTER TABLE public.outbound_campaign_provider_attempt_receipts
  DROP CONSTRAINT outbound_campaign_provider_attempt_receip_delivery_proven_check,
  ADD COLUMN provider_message_id TEXT,
  ADD COLUMN provider_code TEXT,
  ADD COLUMN provider_result_received BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN reconciliation_evidence_digest TEXT,
  ADD CONSTRAINT outbound_campaign_provider_attempt_receipts_result_shape CHECK (
    (provider_message_id IS NULL OR (provider_message_id = btrim(provider_message_id) AND length(provider_message_id) BETWEEN 1 AND 1024))
    AND (provider_code IS NULL OR (provider_code = btrim(provider_code) AND length(provider_code) BETWEEN 1 AND 128))
    AND (provider_result_received OR (provider_message_id IS NULL AND provider_code IS NULL))
    AND (reconciliation_evidence_digest IS NULL OR reconciliation_evidence_digest ~ '^sha256:[0-9a-f]{64}$')
    AND (
      NOT delivery_proven
      OR (completion = 'reconciled' AND provider_result_received AND provider_message_id IS NOT NULL AND business_call_dispatched AND real_external_call_executed AND reconciliation_evidence_digest IS NOT NULL)
    )
  );

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_outbound_campaign_dispatches_no_delete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'outbound campaign dispatch facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.handoff_id IS DISTINCT FROM OLD.handoff_id
     OR NEW.customer_id IS DISTINCT FROM OLD.customer_id OR NEW.step_index IS DISTINCT FROM OLD.step_index
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id OR NEW.recipient_digest IS DISTINCT FROM OLD.recipient_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.sender_userid_snapshot IS DISTINCT FROM OLD.sender_userid_snapshot
     OR NEW.external_userid_snapshot IS DISTINCT FROM OLD.external_userid_snapshot
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'outbound campaign dispatch identity is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
LOCK TABLE public.outbound_campaign_provider_attempt_receipts, public.outbound_campaign_dispatches IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.outbound_campaign_dispatches
    WHERE sender_userid_snapshot IS NOT NULL OR external_userid_snapshot IS NOT NULL
       OR block_reason IN ('sender_not_allowed','target_unresolved')
  ) OR EXISTS (
    SELECT 1 FROM public.outbound_campaign_provider_attempt_receipts
    WHERE provider_message_id IS NOT NULL OR provider_code IS NOT NULL OR provider_result_received
       OR reconciliation_evidence_digest IS NOT NULL OR delivery_proven
  ) THEN
    RAISE EXCEPTION 'cannot roll back populated audience outbound receipt facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

DROP INDEX public.outbound_campaign_provider_attempt_receipts_attempt_once;

-- Restore the exact 00078 guard before dropping the immutable snapshots.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_outbound_campaign_dispatches_no_delete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'outbound campaign dispatch facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.handoff_id IS DISTINCT FROM OLD.handoff_id
     OR NEW.customer_id IS DISTINCT FROM OLD.customer_id OR NEW.step_index IS DISTINCT FROM OLD.step_index
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id OR NEW.recipient_digest IS DISTINCT FROM OLD.recipient_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'outbound campaign dispatch identity is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE public.outbound_campaign_provider_attempt_receipts
  DROP CONSTRAINT outbound_campaign_provider_attempt_receipts_result_shape,
  DROP COLUMN reconciliation_evidence_digest,
  DROP COLUMN provider_result_received,
  DROP COLUMN provider_code,
  DROP COLUMN provider_message_id,
  ADD CONSTRAINT outbound_campaign_provider_attempt_receip_delivery_proven_check CHECK (NOT delivery_proven);
ALTER TABLE public.outbound_campaign_dispatches
  DROP CONSTRAINT outbound_campaign_dispatches_target_snapshot_shape,
  DROP CONSTRAINT outbound_campaign_dispatches_block_reason_check,
  ADD CONSTRAINT outbound_campaign_dispatches_block_reason_check CHECK (
    block_reason IN ('external_gate_disabled','identity_unresolved','contact_policy','inactive_customer','provider_preflight_failed')
  ),
  DROP COLUMN external_userid_snapshot,
  DROP COLUMN sender_userid_snapshot;
