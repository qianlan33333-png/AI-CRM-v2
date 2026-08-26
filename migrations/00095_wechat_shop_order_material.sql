-- +goose Up
-- Package A persists only the typed, PII-free order material required by the
-- later WeChat Shop aftersale flow. Provider response bodies and credentials
-- are deliberately excluded.
CREATE TABLE public.order_wechat_shop_materials (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  provider_order_id TEXT NOT NULL UNIQUE,
  status_code BIGINT NOT NULL,
  deal_recorded BOOLEAN NOT NULL,
  amount_minor BIGINT NOT NULL,
  currency TEXT NOT NULL,
  transaction_digest BYTEA,
  evidence_digest BYTEA NOT NULL,
  source TEXT NOT NULL,
  source_key_digest BYTEA,
  readiness TEXT NOT NULL,
  provider_verified BOOLEAN NOT NULL,
  provider_created_at TIMESTAMPTZ,
  provider_paid_at TIMESTAMPTZ,
  provider_updated_at TIMESTAMPTZ,
  synced_at TIMESTAMPTZ NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT order_wechat_shop_materials_provider_order_id CHECK (
    provider_order_id <> '' AND btrim(provider_order_id) = provider_order_id
      AND char_length(provider_order_id) <= 128
  ),
  CONSTRAINT order_wechat_shop_materials_amount CHECK (
    status_code >= 0 AND amount_minor >= 0 AND currency = 'CNY'
  ),
  CONSTRAINT order_wechat_shop_materials_digests CHECK (
    (transaction_digest IS NULL OR octet_length(transaction_digest) = 32)
      AND octet_length(evidence_digest) = 32
      AND (source_key_digest IS NULL OR octet_length(source_key_digest) = 32)
  ),
  CONSTRAINT order_wechat_shop_materials_readiness CHECK (
    readiness IN (
      'ready', 'provider_sync_required', 'order_not_paid',
      'aftersale_evidence_missing', 'no_refundable_line'
    )
  ),
  CONSTRAINT order_wechat_shop_materials_source CHECK (
    source = 'provider'
      AND provider_verified
      AND source_key_digest IS NULL
      AND readiness <> 'provider_sync_required'
    OR source = 'legacy_raw'
      AND NOT provider_verified
      AND source_key_digest IS NOT NULL
      AND readiness = 'provider_sync_required'
  ),
  CONSTRAINT order_wechat_shop_materials_times CHECK (
    version >= 1
      AND created_at <= updated_at
      AND (provider_paid_at IS NULL OR provider_created_at IS NULL OR provider_paid_at >= provider_created_at)
      AND (provider_updated_at IS NULL OR provider_created_at IS NULL OR provider_updated_at >= provider_created_at)
  )
);

CREATE TABLE public.order_wechat_shop_material_lines (
  material_id BIGINT NOT NULL REFERENCES public.order_wechat_shop_materials(id) ON DELETE RESTRICT,
  position INTEGER NOT NULL,
  product_id TEXT NOT NULL,
  sku_id TEXT NOT NULL,
  sku_count BIGINT NOT NULL,
  on_aftersale_sku_count BIGINT,
  finish_aftersale_sku_count BIGINT,
  real_price_minor BIGINT NOT NULL,
  remaining_sku_count BIGINT,
  aftersale_evidence_exact BOOLEAN NOT NULL,
  readiness TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (material_id, position),
  UNIQUE (material_id, product_id, sku_id),
  CONSTRAINT order_wechat_shop_material_lines_identifiers CHECK (
    position > 0
      AND product_id <> '' AND btrim(product_id) = product_id AND char_length(product_id) <= 128
      AND sku_id <> '' AND btrim(sku_id) = sku_id AND char_length(sku_id) <= 128
  ),
  CONSTRAINT order_wechat_shop_material_lines_amount CHECK (
    sku_count > 0 AND real_price_minor >= 0
  ),
  CONSTRAINT order_wechat_shop_material_lines_readiness CHECK (
    readiness IN ('ready', 'aftersale_evidence_missing', 'no_remaining_count')
  ),
  CONSTRAINT order_wechat_shop_material_lines_aftersale CHECK (
    aftersale_evidence_exact
      AND on_aftersale_sku_count IS NOT NULL AND on_aftersale_sku_count >= 0
      AND finish_aftersale_sku_count IS NOT NULL AND finish_aftersale_sku_count >= 0
      AND on_aftersale_sku_count + finish_aftersale_sku_count <= sku_count
      AND remaining_sku_count = sku_count - on_aftersale_sku_count - finish_aftersale_sku_count
      AND (
        remaining_sku_count > 0 AND readiness = 'ready'
        OR remaining_sku_count = 0 AND readiness = 'no_remaining_count'
      )
    OR NOT aftersale_evidence_exact
      AND on_aftersale_sku_count IS NULL
      AND finish_aftersale_sku_count IS NULL
      AND remaining_sku_count IS NULL
      AND readiness = 'aftersale_evidence_missing'
  )
);

CREATE TABLE public.order_wechat_shop_material_quarantines (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_table TEXT NOT NULL,
  source_key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  reason_code TEXT NOT NULL,
  recorded_at TIMESTAMPTZ NOT NULL,
  UNIQUE (source_table, source_key_digest, payload_digest),
  CONSTRAINT order_wechat_shop_material_quarantines_source CHECK (
    source_table = 'wechat_shop_orders'
  ),
  CONSTRAINT order_wechat_shop_material_quarantines_digests CHECK (
    octet_length(source_key_digest) = 32 AND octet_length(payload_digest) = 32
  ),
  CONSTRAINT order_wechat_shop_material_quarantines_reason CHECK (
    reason_code IN (
      'invalid_source_row', 'raw_order_json_invalid', 'raw_order_json_not_exact',
      'typed_amount_conflict', 'typed_transaction_conflict'
    )
  )
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.order_wechat_shop_materials)
    OR EXISTS (SELECT 1 FROM public.order_wechat_shop_material_quarantines)
  THEN
    RAISE EXCEPTION 'cannot roll back populated WeChat Shop order material'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

DROP TABLE public.order_wechat_shop_material_quarantines;
DROP TABLE public.order_wechat_shop_material_lines;
DROP TABLE public.order_wechat_shop_materials;
