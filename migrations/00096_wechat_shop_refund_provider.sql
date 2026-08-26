-- +goose Up
-- Provider activation is additive. Existing local/v1 reservations remain
-- readable but can never cross the Provider boundary because their exact
-- product/SKU/count/reason material cannot be reconstructed safely.
ALTER TABLE public.order_wechat_shop_refunds
  ADD COLUMN contract_version TEXT NOT NULL DEFAULT 'local/v1',
  ADD COLUMN provider_order_id TEXT,
  ADD COLUMN product_id TEXT,
  ADD COLUMN sku_id TEXT,
  ADD COLUMN refund_count BIGINT,
  ADD COLUMN unit_price_minor BIGINT,
  ADD COLUMN reason_code TEXT,
  ADD COLUMN material_evidence_digest BYTEA,
  ADD COLUMN provider_aftersale_id TEXT;

ALTER TABLE public.order_wechat_shop_refunds
  ADD CONSTRAINT order_wechat_shop_refunds_provider_contract CHECK (
    contract_version = 'local/v1'
      AND provider_order_id IS NULL AND product_id IS NULL AND sku_id IS NULL
      AND refund_count IS NULL AND unit_price_minor IS NULL AND reason_code IS NULL
      AND material_evidence_digest IS NULL AND provider_aftersale_id IS NULL
    OR contract_version = 'provider/v2'
      AND provider_order_id <> '' AND btrim(provider_order_id) = provider_order_id
      AND char_length(provider_order_id) <= 128
      AND product_id <> '' AND btrim(product_id) = product_id AND char_length(product_id) <= 128
      AND sku_id <> '' AND btrim(sku_id) = sku_id AND char_length(sku_id) <= 128
      AND refund_count > 0 AND refund_count <= 1000000
      AND unit_price_minor >= 0 AND unit_price_minor <= 1000000000
      AND amount_minor <= unit_price_minor * refund_count
      AND reason_code IN (
        '10000000','10000001','10000002','10000006','10000007',
        '10000008','10000014','10000015','10000017','10000021'
      )
      AND octet_length(material_evidence_digest) = 32
      AND (
        state IN ('accepted','executing','outcome_unknown','final_failed')
          AND provider_aftersale_id IS NULL
        OR state IN ('provider_accepted','succeeded')
          AND provider_aftersale_id <> ''
          AND btrim(provider_aftersale_id) = provider_aftersale_id
          AND char_length(provider_aftersale_id) <= 128
          AND octet_length(provider_acceptance_digest) = 32
      )
  );

CREATE UNIQUE INDEX order_wechat_shop_refunds_provider_aftersale_id_uidx
  ON public.order_wechat_shop_refunds (provider_aftersale_id)
  WHERE provider_aftersale_id IS NOT NULL;
CREATE INDEX order_wechat_shop_refunds_line_state_idx
  ON public.order_wechat_shop_refunds (order_id, product_id, sku_id, state, id)
  WHERE contract_version = 'provider/v2';

ALTER TABLE public.order_wechat_shop_refund_attempts
  DROP CONSTRAINT order_wechat_shop_attempts_completion,
  ADD CONSTRAINT order_wechat_shop_attempts_completion CHECK (
    outcome IS NULL AND evidence_digest IS NULL AND completed_at IS NULL
    OR outcome = 'provider_accepted' AND evidence_digest IS NOT NULL AND completed_at IS NOT NULL
    OR outcome IN ('outcome_unknown','final_failed') AND completed_at IS NOT NULL
  );

ALTER TABLE public.order_wechat_shop_refund_callbacks
  ADD COLUMN contract_version TEXT NOT NULL DEFAULT 'local/v1',
  ADD COLUMN provider_aftersale_id TEXT,
  ADD COLUMN provider_status TEXT,
  ADD COLUMN river_job_id BIGINT;
ALTER TABLE public.order_wechat_shop_refund_callbacks
  DROP CONSTRAINT order_wechat_shop_callbacks_completion,
  ADD CONSTRAINT order_wechat_shop_callbacks_completion CHECK (
    contract_version = 'local/v1'
      AND provider_aftersale_id IS NULL AND provider_status IS NULL AND river_job_id IS NULL
      AND (
        state = 'reserved' AND outcome IS NULL AND result_digest IS NULL AND completed_at IS NULL
        OR state = 'completed' AND outcome IN ('applied','rejected')
          AND result_digest IS NOT NULL AND completed_at IS NOT NULL
      )
    OR contract_version = 'provider/v2'
      AND provider_aftersale_id <> '' AND btrim(provider_aftersale_id) = provider_aftersale_id
      AND char_length(provider_aftersale_id) <= 128
      AND provider_status <> '' AND btrim(provider_status) = provider_status
      AND char_length(provider_status) <= 64
      AND (
        state = 'reserved' AND outcome IS NULL AND result_digest IS NULL
          AND river_job_id IS NULL AND completed_at IS NULL
        OR state = 'completed' AND outcome = 'query_queued'
          AND river_job_id > 0 AND result_digest IS NOT NULL AND completed_at IS NOT NULL
      )
  );

CREATE TABLE public.order_wechat_shop_material_sync_requests (
  provider_order_id TEXT PRIMARY KEY,
  generation BIGINT NOT NULL DEFAULT 1,
  state TEXT NOT NULL DEFAULT 'reserved',
  river_job_id BIGINT UNIQUE,
  evidence_digest BYTEA,
  requested_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT order_wechat_shop_material_sync_requests_order CHECK (
    provider_order_id <> '' AND btrim(provider_order_id) = provider_order_id
      AND char_length(provider_order_id) <= 128
  ),
  CONSTRAINT order_wechat_shop_material_sync_requests_generation CHECK (generation > 0),
  CONSTRAINT order_wechat_shop_material_sync_requests_state CHECK (state IN ('reserved','queued','completed')),
  CONSTRAINT order_wechat_shop_material_sync_requests_completion CHECK (
    state = 'reserved' AND river_job_id IS NULL AND evidence_digest IS NULL AND completed_at IS NULL
    OR state = 'queued' AND river_job_id > 0 AND evidence_digest IS NULL AND completed_at IS NULL
    OR state = 'completed' AND river_job_id > 0
      AND octet_length(evidence_digest) = 32 AND completed_at >= requested_at
  )
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_wechat_shop_material_sync_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.order_wechat_shop_material_sync_requests
    WHERE provider_order_id = NEW.provider_order_id AND state <> 'reserved'
  ) THEN
    RAISE EXCEPTION 'WeChat Shop material sync reservation must enqueue in its command transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER order_wechat_shop_material_sync_complete_before_commit
AFTER INSERT OR UPDATE ON public.order_wechat_shop_material_sync_requests
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_wechat_shop_material_sync_complete_before_commit();

-- +goose Down
LOCK TABLE public.order_wechat_shop_material_sync_requests,
  public.order_wechat_shop_refund_callbacks,
  public.order_wechat_shop_refund_attempts,
  public.order_wechat_shop_refunds IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.order_wechat_shop_material_sync_requests)
    OR EXISTS (
      SELECT 1 FROM public.order_wechat_shop_refunds
      WHERE contract_version = 'provider/v2'
    )
    OR EXISTS (
      SELECT 1 FROM public.order_wechat_shop_refund_callbacks
      WHERE contract_version = 'provider/v2'
    )
  THEN
    RAISE EXCEPTION 'cannot roll back materialized WeChat Shop Provider facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER order_wechat_shop_material_sync_complete_before_commit ON public.order_wechat_shop_material_sync_requests;
DROP FUNCTION public.aicrm_wechat_shop_material_sync_complete_before_commit();
DROP TABLE public.order_wechat_shop_material_sync_requests;

ALTER TABLE public.order_wechat_shop_refund_callbacks
  DROP CONSTRAINT order_wechat_shop_callbacks_completion;
ALTER TABLE public.order_wechat_shop_refund_callbacks
  DROP COLUMN river_job_id,
  DROP COLUMN provider_status,
  DROP COLUMN provider_aftersale_id,
  DROP COLUMN contract_version;
ALTER TABLE public.order_wechat_shop_refund_callbacks
  ADD CONSTRAINT order_wechat_shop_callbacks_completion CHECK (
    state = 'reserved' AND outcome IS NULL AND result_digest IS NULL AND completed_at IS NULL
    OR state = 'completed' AND outcome IN ('applied','rejected')
      AND result_digest IS NOT NULL AND completed_at IS NOT NULL
  );

ALTER TABLE public.order_wechat_shop_refund_attempts
  DROP CONSTRAINT order_wechat_shop_attempts_completion,
  ADD CONSTRAINT order_wechat_shop_attempts_completion CHECK (
    outcome IS NULL AND evidence_digest IS NULL AND completed_at IS NULL
    OR outcome = 'provider_accepted' AND evidence_digest IS NOT NULL AND completed_at IS NOT NULL
    OR outcome IN ('outcome_unknown','final_failed') AND completed_at IS NOT NULL
  );

DROP INDEX public.order_wechat_shop_refunds_line_state_idx;
DROP INDEX public.order_wechat_shop_refunds_provider_aftersale_id_uidx;
ALTER TABLE public.order_wechat_shop_refunds
  DROP CONSTRAINT order_wechat_shop_refunds_provider_contract;
ALTER TABLE public.order_wechat_shop_refunds
  DROP COLUMN provider_aftersale_id,
  DROP COLUMN material_evidence_digest,
  DROP COLUMN reason_code,
  DROP COLUMN unit_price_minor,
  DROP COLUMN refund_count,
  DROP COLUMN sku_id,
  DROP COLUMN product_id,
  DROP COLUMN provider_order_id,
  DROP COLUMN contract_version;
