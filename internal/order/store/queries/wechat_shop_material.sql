-- name: UpsertWeChatShopOrderMaterial :one
WITH changed AS (
  INSERT INTO public.order_wechat_shop_materials (
    provider_order_id, status_code, deal_recorded, amount_minor, currency,
    transaction_digest, evidence_digest, source, source_key_digest, readiness,
    provider_verified, provider_created_at, provider_paid_at, provider_updated_at,
    synced_at, created_at, updated_at
  ) VALUES (
    sqlc.arg(provider_order_id), sqlc.arg(status_code), sqlc.arg(deal_recorded),
    sqlc.arg(amount_minor), sqlc.arg(currency), sqlc.narg(transaction_digest),
    sqlc.arg(evidence_digest), sqlc.arg(source), sqlc.narg(source_key_digest),
    sqlc.arg(readiness), sqlc.arg(provider_verified), sqlc.narg(provider_created_at),
    sqlc.narg(provider_paid_at), sqlc.narg(provider_updated_at), sqlc.arg(synced_at),
    now(), now()
  )
  ON CONFLICT (provider_order_id) DO UPDATE SET
    status_code = EXCLUDED.status_code,
    deal_recorded = EXCLUDED.deal_recorded,
    amount_minor = EXCLUDED.amount_minor,
    currency = EXCLUDED.currency,
    transaction_digest = EXCLUDED.transaction_digest,
    evidence_digest = EXCLUDED.evidence_digest,
    source = EXCLUDED.source,
    source_key_digest = EXCLUDED.source_key_digest,
    readiness = EXCLUDED.readiness,
    provider_verified = EXCLUDED.provider_verified,
    provider_created_at = EXCLUDED.provider_created_at,
    provider_paid_at = EXCLUDED.provider_paid_at,
    provider_updated_at = EXCLUDED.provider_updated_at,
    synced_at = EXCLUDED.synced_at,
    version = order_wechat_shop_materials.version + 1,
    updated_at = EXCLUDED.updated_at
  WHERE order_wechat_shop_materials.evidence_digest <> EXCLUDED.evidence_digest
    AND (
      (EXCLUDED.source = 'provider'
        AND (order_wechat_shop_materials.source = 'legacy_raw'
          OR EXCLUDED.synced_at >= order_wechat_shop_materials.synced_at))
      OR (order_wechat_shop_materials.source = 'legacy_raw'
        AND EXCLUDED.source = 'legacy_raw'
        AND EXCLUDED.synced_at >= order_wechat_shop_materials.synced_at)
    )
  RETURNING id, version, TRUE AS changed
)
SELECT id, version, changed FROM changed
UNION ALL
SELECT material.id, material.version, FALSE AS changed
FROM public.order_wechat_shop_materials AS material
WHERE material.provider_order_id = sqlc.arg(provider_order_id)
  AND NOT EXISTS (SELECT 1 FROM changed)
LIMIT 1;

-- name: DeleteWeChatShopOrderMaterialLines :exec
DELETE FROM public.order_wechat_shop_material_lines
WHERE material_id = sqlc.arg(material_id);

-- name: InsertWeChatShopOrderMaterialLine :exec
INSERT INTO public.order_wechat_shop_material_lines (
  material_id, position, product_id, sku_id, sku_count,
  on_aftersale_sku_count, finish_aftersale_sku_count, real_price_minor,
  remaining_sku_count, aftersale_evidence_exact, readiness, created_at
) VALUES (
  sqlc.arg(material_id), sqlc.arg(position), sqlc.arg(product_id), sqlc.arg(sku_id),
  sqlc.arg(sku_count), sqlc.narg(on_aftersale_sku_count),
  sqlc.narg(finish_aftersale_sku_count), sqlc.arg(real_price_minor),
  sqlc.narg(remaining_sku_count), sqlc.arg(aftersale_evidence_exact),
  sqlc.arg(readiness), now()
);

-- name: GetWeChatShopOrderMaterial :one
SELECT id, provider_order_id, status_code, deal_recorded, amount_minor, currency,
  transaction_digest, evidence_digest, source, source_key_digest, readiness,
  provider_verified, provider_created_at, provider_paid_at, provider_updated_at,
  synced_at, version, created_at, updated_at
FROM public.order_wechat_shop_materials
WHERE provider_order_id = sqlc.arg(provider_order_id);

-- name: ListWeChatShopOrderMaterialLines :many
SELECT material_id, position, product_id, sku_id, sku_count,
  on_aftersale_sku_count, finish_aftersale_sku_count, real_price_minor,
  remaining_sku_count, aftersale_evidence_exact, readiness, created_at
FROM public.order_wechat_shop_material_lines
WHERE material_id = sqlc.arg(material_id)
ORDER BY position;

-- name: RecordWeChatShopLegacyQuarantine :one
INSERT INTO public.order_wechat_shop_material_quarantines (
  source_table, source_key_digest, payload_digest, reason_code, recorded_at
) VALUES (
  sqlc.arg(source_table), sqlc.arg(source_key_digest), sqlc.arg(payload_digest),
  sqlc.arg(reason_code), sqlc.arg(recorded_at)
)
ON CONFLICT (source_table, source_key_digest, payload_digest) DO NOTHING
RETURNING id;

-- name: GetWeChatShopLegacyQuarantine :one
SELECT id, source_table, source_key_digest, payload_digest, reason_code, recorded_at
FROM public.order_wechat_shop_material_quarantines
WHERE source_table = sqlc.arg(source_table)
  AND source_key_digest = sqlc.arg(source_key_digest)
  AND payload_digest = sqlc.arg(payload_digest);

-- name: CompleteWeChatShopMaterialSyncRequest :execrows
UPDATE public.order_wechat_shop_material_sync_requests
SET state = 'completed', evidence_digest = sqlc.arg(evidence_digest),
  completed_at = sqlc.arg(completed_at)
WHERE provider_order_id = sqlc.arg(provider_order_id)
  AND state = 'queued';
