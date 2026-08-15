-- +goose Up
-- Order owns compatibility exports, refund intent and the local external-effect
-- boundary. This migration never contacts a payment provider.
CREATE TABLE public.order_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT order_operation_receipts_operation CHECK (operation IN ('export', 'refund', 'external_effect_retry')),
  CONSTRAINT order_operation_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT order_operation_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT order_operation_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT order_operation_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT order_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

CREATE TABLE public.order_export_jobs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  job_id TEXT NOT NULL UNIQUE,
  resource TEXT NOT NULL,
  format TEXT NOT NULL,
  operator_id BIGINT NOT NULL,
  content_text TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT order_export_jobs_job_id CHECK (job_id ~ '^exp_[A-Za-z0-9_-]{8,64}$'),
  CONSTRAINT order_export_jobs_resource CHECK (resource IN ('orders', 'payments', 'refunds')),
  CONSTRAINT order_export_jobs_format CHECK (format = 'csv'),
  CONSTRAINT order_export_jobs_operator CHECK (operator_id > 0)
);
CREATE INDEX order_export_jobs_operator_created_idx ON public.order_export_jobs (operator_id, created_at DESC, id DESC);

CREATE TABLE public.order_external_effects (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  provider TEXT NOT NULL,
  effect_kind TEXT NOT NULL,
  state TEXT NOT NULL,
  auto_retry_allowed BOOLEAN NOT NULL DEFAULT FALSE,
  provider_receipt JSONB,
  manual_review_requested_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT order_external_effects_provider CHECK (provider IN ('wechat', 'alipay', 'wechat_shop')),
  CONSTRAINT order_external_effects_kind CHECK (effect_kind IN ('refund', 'external_push')),
  CONSTRAINT order_external_effects_state CHECK (state IN ('pending_external_gate', 'outcome_unknown', 'completed', 'final_failed')),
  CONSTRAINT order_external_effects_no_auto_retry CHECK (auto_retry_allowed = FALSE),
  CONSTRAINT order_external_effects_time CHECK (updated_at >= created_at)
);
CREATE INDEX order_external_effects_order_created_idx ON public.order_external_effects (order_id, created_at DESC, id DESC);
CREATE INDEX order_external_effects_state_created_idx ON public.order_external_effects (state, created_at DESC, id DESC);

CREATE TABLE public.order_refunds (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES public.order_list_projections(id) ON DELETE RESTRICT,
  external_effect_id BIGINT NOT NULL UNIQUE REFERENCES public.order_external_effects(id) ON DELETE RESTRICT,
  provider TEXT NOT NULL,
  refund_id TEXT NOT NULL,
  out_refund_no TEXT NOT NULL,
  refund_amount_total BIGINT NOT NULL,
  currency TEXT NOT NULL,
  reason TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT order_refunds_provider CHECK (provider IN ('wechat', 'alipay', 'wechat_shop')),
  CONSTRAINT order_refunds_refund_id CHECK (btrim(refund_id) = refund_id AND refund_id <> '' AND char_length(refund_id) <= 200),
  CONSTRAINT order_refunds_out_refund_no CHECK (btrim(out_refund_no) = out_refund_no AND out_refund_no <> '' AND char_length(out_refund_no) <= 200),
  CONSTRAINT order_refunds_amount CHECK (refund_amount_total > 0),
  CONSTRAINT order_refunds_currency CHECK (currency ~ '^[A-Z]{3}$'),
  CONSTRAINT order_refunds_reason CHECK (btrim(reason) = reason AND reason <> '' AND char_length(reason) <= 500),
  CONSTRAINT order_refunds_status CHECK (status IN ('pending_external_gate', 'outcome_unknown', 'completed', 'final_failed')),
  UNIQUE (provider, refund_id),
  UNIQUE (provider, out_refund_no)
);
CREATE INDEX order_refunds_provider_created_idx ON public.order_refunds (provider, created_at DESC, id DESC);
CREATE INDEX order_refunds_order_created_idx ON public.order_refunds (order_id, created_at DESC, id DESC);

-- A receipt must be resolved in the transaction that reserved it; this makes
-- command replay deterministic and prevents a partially durable refund intent.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_order_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.order_operation_receipts WHERE id = NEW.id AND state = 'completed') THEN
    RAISE EXCEPTION 'order operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER order_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.order_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_order_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_order_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed order operation receipt is immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid order operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER order_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.order_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_order_receipt_transition_valid();

-- +goose Down
DROP TRIGGER order_operation_receipts_transition ON public.order_operation_receipts;
DROP TRIGGER order_operation_receipts_complete_before_commit ON public.order_operation_receipts;
DROP FUNCTION public.aicrm_order_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_order_receipt();
DROP TABLE public.order_refunds;
DROP TABLE public.order_external_effects;
DROP TABLE public.order_export_jobs;
DROP TABLE public.order_operation_receipts;
