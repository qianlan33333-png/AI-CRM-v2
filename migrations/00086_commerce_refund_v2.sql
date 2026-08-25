-- +goose Up
-- Commerce Refund V2 keeps WeChat Pay refunds canonical in PE01. WeChat Shop
-- uses separate Order-owned facts and never writes the legacy order_refunds or
-- order_external_effects tables and never reuses the WeChat Pay provider.

CREATE TABLE public.order_wechat_shop_refunds (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  actor_id BIGINT NOT NULL,
  merchant_order_no TEXT NOT NULL,
  out_refund_no TEXT NOT NULL UNIQUE,
  amount_minor BIGINT NOT NULL,
  currency CHAR(3) NOT NULL,
  reason_digest BYTEA NOT NULL,
  transaction_digest BYTEA NOT NULL,
  command_key_digest BYTEA NOT NULL,
  command_payload_digest BYTEA NOT NULL,
  source_ref_digest BYTEA NOT NULL UNIQUE,
  target_ref_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  policy_version_digest BYTEA NOT NULL,
  provider_acceptance_digest BYTEA,
  provider_refund_digest BYTEA,
  settlement_receipt_digest BYTEA,
  state TEXT NOT NULL DEFAULT 'accepted',
  attempt_count BIGINT NOT NULL DEFAULT 0,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  settled_at TIMESTAMPTZ,
  CONSTRAINT order_wechat_shop_refunds_actor CHECK (actor_id > 0),
  CONSTRAINT order_wechat_shop_refunds_order_no CHECK (
    btrim(merchant_order_no) = merchant_order_no AND merchant_order_no <> ''
      AND char_length(merchant_order_no) <= 200
  ),
  CONSTRAINT order_wechat_shop_refunds_number CHECK (out_refund_no ~ '^wsr_[a-f0-9]{32}$'),
  CONSTRAINT order_wechat_shop_refunds_amount CHECK (amount_minor > 0),
  CONSTRAINT order_wechat_shop_refunds_currency CHECK (currency = 'CNY'),
  CONSTRAINT order_wechat_shop_refunds_digests CHECK (
    octet_length(reason_digest) = 32 AND octet_length(transaction_digest) = 32
      AND octet_length(command_key_digest) = 32 AND octet_length(command_payload_digest) = 32
      AND octet_length(source_ref_digest) = 32 AND octet_length(target_ref_digest) = 32
      AND octet_length(payload_digest) = 32 AND octet_length(policy_version_digest) = 32
      AND (provider_acceptance_digest IS NULL OR octet_length(provider_acceptance_digest) = 32)
      AND (provider_refund_digest IS NULL OR octet_length(provider_refund_digest) = 32)
      AND (settlement_receipt_digest IS NULL OR octet_length(settlement_receipt_digest) = 32)
  ),
  CONSTRAINT order_wechat_shop_refunds_state CHECK (state IN (
    'accepted','executing','provider_accepted','outcome_unknown','succeeded','final_failed'
  )),
  CONSTRAINT order_wechat_shop_refunds_evidence CHECK (
    state = 'provider_accepted'
      AND provider_acceptance_digest IS NOT NULL
      AND provider_refund_digest IS NULL AND settlement_receipt_digest IS NULL AND settled_at IS NULL
    OR state = 'succeeded'
      AND provider_refund_digest IS NOT NULL AND settlement_receipt_digest IS NOT NULL AND settled_at IS NOT NULL
    OR state IN ('accepted','executing','outcome_unknown','final_failed')
      AND provider_acceptance_digest IS NULL
      AND provider_refund_digest IS NULL AND settlement_receipt_digest IS NULL AND settled_at IS NULL
  ),
  CONSTRAINT order_wechat_shop_refunds_version CHECK (attempt_count >= 0 AND version > 0),
  CONSTRAINT order_wechat_shop_refunds_time CHECK (
    updated_at >= created_at AND (settled_at IS NULL OR settled_at >= created_at)
  ),
  UNIQUE (actor_id, command_key_digest)
);
CREATE INDEX order_wechat_shop_refunds_order_state_idx
  ON public.order_wechat_shop_refunds (order_id, state, id);

CREATE TABLE public.order_wechat_shop_refund_attempts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  refund_id BIGINT NOT NULL REFERENCES public.order_wechat_shop_refunds(id) ON DELETE RESTRICT,
  attempt_no BIGINT NOT NULL,
  river_job_id BIGINT NOT NULL,
  river_attempt BIGINT NOT NULL,
  args_digest BYTEA NOT NULL,
  request_digest BYTEA NOT NULL,
  outcome TEXT,
  evidence_digest BYTEA,
  started_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT order_wechat_shop_attempts_numbers CHECK (
    attempt_no > 0 AND river_job_id > 0 AND river_attempt > 0
  ),
  CONSTRAINT order_wechat_shop_attempts_digests CHECK (
    octet_length(args_digest) = 32 AND octet_length(request_digest) = 32
      AND (evidence_digest IS NULL OR octet_length(evidence_digest) = 32)
  ),
  CONSTRAINT order_wechat_shop_attempts_outcome CHECK (
    outcome IS NULL OR outcome IN ('provider_accepted','outcome_unknown','final_failed')
  ),
  CONSTRAINT order_wechat_shop_attempts_completion CHECK (
    outcome IS NULL AND evidence_digest IS NULL AND completed_at IS NULL
    OR outcome = 'provider_accepted' AND evidence_digest IS NOT NULL AND completed_at IS NOT NULL
    OR outcome IN ('outcome_unknown','final_failed') AND completed_at IS NOT NULL
  ),
  UNIQUE (refund_id, attempt_no),
  UNIQUE (river_job_id, river_attempt)
);

CREATE TABLE public.order_wechat_shop_refund_callbacks (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  refund_id BIGINT NOT NULL REFERENCES public.order_wechat_shop_refunds(id) ON DELETE RESTRICT,
  provider_event_digest BYTEA NOT NULL UNIQUE,
  payload_digest BYTEA NOT NULL,
  provider_refund_digest BYTEA NOT NULL,
  outcome TEXT,
  result_digest BYTEA,
  state TEXT NOT NULL DEFAULT 'reserved',
  received_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT order_wechat_shop_callbacks_digests CHECK (
    octet_length(provider_event_digest) = 32 AND octet_length(payload_digest) = 32
      AND octet_length(provider_refund_digest) = 32
      AND (result_digest IS NULL OR octet_length(result_digest) = 32)
  ),
  CONSTRAINT order_wechat_shop_callbacks_state CHECK (state IN ('reserved','completed')),
  CONSTRAINT order_wechat_shop_callbacks_completion CHECK (
    state = 'reserved' AND outcome IS NULL AND result_digest IS NULL AND completed_at IS NULL
    OR state = 'completed' AND outcome IN ('applied','rejected')
      AND result_digest IS NOT NULL AND completed_at IS NOT NULL
  )
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_wechat_shop_callback_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.order_wechat_shop_refund_callbacks
    WHERE id = NEW.id AND state = 'completed' AND outcome IS NOT NULL
      AND result_digest IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'WeChat Shop callback must complete in its refund transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER order_wechat_shop_callbacks_complete_before_commit
AFTER INSERT OR UPDATE ON public.order_wechat_shop_refund_callbacks
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_wechat_shop_callback_complete_before_commit();

CREATE TABLE public.order_wechat_shop_refund_queries (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  refund_id BIGINT NOT NULL REFERENCES public.order_wechat_shop_refunds(id) ON DELETE RESTRICT,
  evidence_digest BYTEA NOT NULL,
  provider_refund_digest BYTEA,
  amount_minor BIGINT NOT NULL,
  currency TEXT NOT NULL,
  outcome TEXT NOT NULL,
  recorded_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT order_wechat_shop_queries_digests CHECK (
    octet_length(evidence_digest) = 32
      AND (provider_refund_digest IS NULL OR octet_length(provider_refund_digest) = 32)
  ),
  CONSTRAINT order_wechat_shop_queries_outcome CHECK (outcome IN ('not_confirmed','applied','conflict')),
  CONSTRAINT order_wechat_shop_queries_evidence CHECK (
    outcome = 'not_confirmed' AND provider_refund_digest IS NULL AND amount_minor = 0 AND currency = ''
    OR outcome = 'applied' AND provider_refund_digest IS NOT NULL
      AND amount_minor > 0 AND currency = 'CNY'
    OR outcome = 'conflict' AND amount_minor >= 0
      AND (currency = '' OR currency ~ '^[A-Z]{3}$')
  ),
  UNIQUE (refund_id, evidence_digest)
);

-- +goose Down
LOCK TABLE public.order_wechat_shop_refund_queries,
  public.order_wechat_shop_refund_callbacks,
  public.order_wechat_shop_refund_attempts,
  public.order_wechat_shop_refunds IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.order_wechat_shop_refund_queries)
     OR EXISTS (SELECT 1 FROM public.order_wechat_shop_refund_callbacks)
     OR EXISTS (SELECT 1 FROM public.order_wechat_shop_refund_attempts)
     OR EXISTS (SELECT 1 FROM public.order_wechat_shop_refunds) THEN
    RAISE EXCEPTION 'cannot roll back materialized WeChat Shop refund facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.order_wechat_shop_refund_queries;
DROP TRIGGER order_wechat_shop_callbacks_complete_before_commit ON public.order_wechat_shop_refund_callbacks;
DROP FUNCTION public.aicrm_wechat_shop_callback_complete_before_commit();
DROP TABLE public.order_wechat_shop_refund_callbacks;
DROP TABLE public.order_wechat_shop_refund_attempts;
DROP TABLE public.order_wechat_shop_refunds;
