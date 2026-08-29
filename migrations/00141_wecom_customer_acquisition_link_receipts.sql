-- +goose Up
-- This receipt table is intentionally limited to WeCom customer-acquisition
-- links. It is not an HXC table and it does not create a generic workflow.
CREATE TABLE public.wecom_customer_acquisition_link_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  request_digest BYTEA NOT NULL CHECK (octet_length(request_digest) = 32),
  operation TEXT NOT NULL CHECK (operation IN ('create', 'update', 'delete')),
  link_id TEXT NOT NULL DEFAULT '' CHECK (btrim(link_id) = link_id AND char_length(link_id) <= 1024),
  command_input JSONB NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('accepted', 'attempted', 'executed', 'final_failed', 'outcome_unknown', 'reconciled')),
  provider_link JSONB,
  outcome_digest BYTEA CHECK (outcome_digest IS NULL OR octet_length(outcome_digest) = 32),
  business_endpoint_dispatched BOOLEAN NOT NULL DEFAULT FALSE,
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE,
  reconcile_actor_id BIGINT CHECK (reconcile_actor_id IS NULL OR reconcile_actor_id > 0),
  reconcile_key_digest BYTEA CHECK (reconcile_key_digest IS NULL OR octet_length(reconcile_key_digest) = 32),
  evidence_digest BYTEA CHECK (evidence_digest IS NULL OR octet_length(evidence_digest) = 32),
  resolution TEXT CHECK (resolution IS NULL OR resolution IN ('provider_applied', 'provider_not_applied')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (actor_id, key_digest),
  CHECK (
    (operation = 'create' AND link_id = '') OR
    (operation IN ('update', 'delete') AND link_id <> '')
  ),
  CHECK (
    (state = 'accepted' AND provider_link IS NULL AND outcome_digest IS NULL AND NOT business_endpoint_dispatched AND NOT real_external_call_executed AND resolution IS NULL) OR
    (state = 'attempted' AND provider_link IS NULL AND outcome_digest IS NULL AND NOT business_endpoint_dispatched AND NOT real_external_call_executed AND resolution IS NULL) OR
    (state IN ('executed', 'final_failed', 'outcome_unknown') AND outcome_digest IS NOT NULL AND resolution IS NULL) OR
    -- A reconciled delete with an explicit Provider not-found readback has no
    -- link payload by design. The readback receipt and resolution are the
    -- evidence; a synthetic deleted link would be misleading.
    (state = 'reconciled' AND outcome_digest IS NOT NULL AND business_endpoint_dispatched AND real_external_call_executed AND reconcile_actor_id IS NOT NULL AND reconcile_key_digest IS NOT NULL AND evidence_digest IS NOT NULL AND resolution IS NOT NULL)
  )
);
CREATE INDEX wecom_customer_acquisition_link_receipts_state_idx
  ON public.wecom_customer_acquisition_link_receipts(state, updated_at DESC, id DESC);

-- +goose Down
LOCK TABLE public.wecom_customer_acquisition_link_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.wecom_customer_acquisition_link_receipts) THEN
    RAISE EXCEPTION 'cannot roll back populated customer-acquisition link receipts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.wecom_customer_acquisition_link_receipts;
