-- +goose Up
-- C01 records whether a completed provider attempt actually crossed the
-- business Provider boundary. It remains intentionally distinct from
-- delivery: WeCom add_msg_template acceptance is never delivery proof.
ALTER TABLE public.outbound_campaign_provider_attempt_receipts
  ADD COLUMN business_call_dispatched BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE,
  ADD CONSTRAINT outbound_campaign_provider_attempt_receipts_evidence CHECK (
    NOT real_external_call_executed OR business_call_dispatched
  );

COMMENT ON COLUMN public.outbound_campaign_provider_attempt_receipts.business_call_dispatched IS
  'True only when the C01 provider request crossed the business Provider boundary.';
COMMENT ON COLUMN public.outbound_campaign_provider_attempt_receipts.real_external_call_executed IS
  'True only when that business Provider boundary was the real external WeCom call; not delivery proof.';

-- +goose Down
-- A true fact must not be silently discarded by rollback. Existing local-only
-- receipts have both fields false and remain reversible.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM public.outbound_campaign_provider_attempt_receipts
    WHERE business_call_dispatched OR real_external_call_executed
  ) THEN
    RAISE EXCEPTION 'cannot roll back populated outbound campaign provider evidence' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE public.outbound_campaign_provider_attempt_receipts
  DROP CONSTRAINT outbound_campaign_provider_attempt_receipts_evidence,
  DROP COLUMN real_external_call_executed,
  DROP COLUMN business_call_dispatched;
