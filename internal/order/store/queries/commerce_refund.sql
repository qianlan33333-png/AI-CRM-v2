-- name: FindPE01RefundOrderCandidates :many
SELECT id
FROM public.order_list_projections
WHERE provider = 'wechat' AND pe01_contract_version = 'pe01/v1'
  AND (merchant_order_no = sqlc.arg(order_reference)::text
    OR platform_transaction_no = sqlc.arg(order_reference)::text
    OR id::text = sqlc.arg(order_reference)::text)
ORDER BY id
LIMIT 2;

-- name: FindWeChatShopRefundOrderCandidates :many
SELECT id, merchant_order_no, platform_transaction_no, amount_minor, currency,
  status, created_at, updated_at
FROM public.order_list_projections
WHERE provider = 'wechat_shop'
  AND (merchant_order_no = sqlc.arg(order_reference)::text
    OR platform_transaction_no = sqlc.arg(order_reference)::text
    OR id::text = sqlc.arg(order_reference)::text)
ORDER BY id
LIMIT 2
FOR UPDATE;

-- name: CountWeChatShopReservedRefundAmount :one
SELECT COALESCE(sum(value), 0)::bigint
FROM (
  SELECT amount_minor AS value
  FROM public.order_wechat_shop_refunds
  WHERE order_id = sqlc.arg(order_id)::bigint AND state <> 'final_failed'
  UNION ALL
  SELECT refund_amount_total AS value
  FROM public.order_refunds
  WHERE order_id = sqlc.arg(order_id)::bigint AND provider = 'wechat_shop'
    AND status IN ('pending_external_gate','outcome_unknown','completed')
) AS reserved;

-- name: CountWeChatShopReservedRefundLineCount :one
SELECT COALESCE(sum(refund_count), 0)::bigint
FROM public.order_wechat_shop_refunds
WHERE order_id = sqlc.arg(order_id)::bigint
  AND contract_version = 'provider/v2'
  AND product_id = sqlc.arg(product_id)::text
  AND sku_id = sqlc.arg(sku_id)::text
  AND state <> 'final_failed';

-- name: GetWeChatShopRefundMaterial :one
SELECT id, provider_order_id, status_code, deal_recorded, amount_minor, currency,
  transaction_digest, evidence_digest, source, source_key_digest, readiness,
  provider_verified, provider_created_at, provider_paid_at, provider_updated_at,
  synced_at, version, created_at, updated_at
FROM public.order_wechat_shop_materials
WHERE provider_order_id = sqlc.arg(provider_order_id);

-- name: ListWeChatShopRefundMaterialLines :many
SELECT material_id, position, product_id, sku_id, sku_count,
  on_aftersale_sku_count, finish_aftersale_sku_count, real_price_minor,
  remaining_sku_count, aftersale_evidence_exact, readiness, created_at
FROM public.order_wechat_shop_material_lines
WHERE material_id = sqlc.arg(material_id)
ORDER BY position;

-- name: ReserveWeChatShopMaterialSync :one
INSERT INTO public.order_wechat_shop_material_sync_requests (
  provider_order_id, requested_at
) VALUES (
  sqlc.arg(provider_order_id), sqlc.arg(requested_at)
)
ON CONFLICT (provider_order_id) DO UPDATE SET
  generation = order_wechat_shop_material_sync_requests.generation + 1,
  state = 'reserved', river_job_id = NULL, evidence_digest = NULL,
  requested_at = EXCLUDED.requested_at, completed_at = NULL
WHERE order_wechat_shop_material_sync_requests.state = 'completed'
RETURNING provider_order_id, generation, state, river_job_id,
  evidence_digest, requested_at, completed_at;

-- name: GetWeChatShopMaterialSync :one
SELECT provider_order_id, generation, state, river_job_id,
  evidence_digest, requested_at, completed_at
FROM public.order_wechat_shop_material_sync_requests
WHERE provider_order_id = sqlc.arg(provider_order_id);

-- name: MarkWeChatShopMaterialSyncQueued :one
UPDATE public.order_wechat_shop_material_sync_requests
SET state = 'queued', river_job_id = sqlc.arg(river_job_id)
WHERE provider_order_id = sqlc.arg(provider_order_id)
  AND generation = sqlc.arg(generation) AND state = 'reserved'
RETURNING provider_order_id, generation, state, river_job_id,
  evidence_digest, requested_at, completed_at;

-- name: CreateWeChatShopRefund :one
INSERT INTO public.order_wechat_shop_refunds (
  order_id, actor_id, contract_version, merchant_order_no, provider_order_id,
  product_id, sku_id, refund_count, unit_price_minor, reason_code,
  material_evidence_digest, out_refund_no, amount_minor, currency,
  reason_digest, transaction_digest, command_key_digest, command_payload_digest,
  source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  created_at, updated_at
) VALUES (
  sqlc.arg(order_id), sqlc.arg(actor_id), 'provider/v2', sqlc.arg(merchant_order_no),
  sqlc.arg(provider_order_id), sqlc.arg(product_id), sqlc.arg(sku_id),
  sqlc.arg(refund_count), sqlc.arg(unit_price_minor), sqlc.arg(reason_code),
  sqlc.arg(material_evidence_digest), sqlc.arg(out_refund_no),
  sqlc.arg(amount_minor), sqlc.arg(currency),
  sqlc.arg(reason_digest), sqlc.arg(transaction_digest), sqlc.arg(command_key_digest),
  sqlc.arg(command_payload_digest), sqlc.arg(source_ref_digest), sqlc.arg(target_ref_digest),
  sqlc.arg(payload_digest), sqlc.arg(policy_version_digest), sqlc.arg(created_at), sqlc.arg(created_at)
)
ON CONFLICT (actor_id, command_key_digest) DO NOTHING
RETURNING *;

-- name: GetWeChatShopRefundByCommand :one
SELECT *
FROM public.order_wechat_shop_refunds
WHERE actor_id = sqlc.arg(actor_id) AND command_key_digest = sqlc.arg(command_key_digest);

-- name: LockWeChatShopRefundByID :one
SELECT *
FROM public.order_wechat_shop_refunds
WHERE id = sqlc.arg(refund_id)
FOR UPDATE;

-- name: LockWeChatShopRefundByOutRefundNo :one
SELECT *
FROM public.order_wechat_shop_refunds
WHERE out_refund_no = sqlc.arg(out_refund_no)
FOR UPDATE;

-- name: LockWeChatShopRefundByAfterSaleID :one
SELECT *
FROM public.order_wechat_shop_refunds
WHERE provider_aftersale_id = sqlc.arg(provider_aftersale_id)
  AND contract_version = 'provider/v2'
FOR UPDATE;

-- name: StartWeChatShopRefundExecution :one
UPDATE public.order_wechat_shop_refunds
SET state = 'executing', attempt_count = attempt_count + 1,
  version = version + 1, updated_at = sqlc.arg(started_at)
WHERE id = sqlc.arg(refund_id) AND state = 'accepted'
  AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: LockIncompleteWeChatShopRefundAttempt :one
SELECT id, refund_id, attempt_no, river_job_id, river_attempt, args_digest,
  request_digest, outcome, evidence_digest, started_at, completed_at
FROM public.order_wechat_shop_refund_attempts
WHERE refund_id = sqlc.arg(refund_id) AND outcome IS NULL AND completed_at IS NULL
ORDER BY attempt_no DESC
LIMIT 1
FOR UPDATE;

-- name: InsertWeChatShopRefundAttempt :one
INSERT INTO public.order_wechat_shop_refund_attempts (
  refund_id, attempt_no, river_job_id, river_attempt, args_digest,
  request_digest, started_at
) VALUES (
  sqlc.arg(refund_id), sqlc.arg(attempt_no), sqlc.arg(river_job_id),
  sqlc.arg(river_attempt), sqlc.arg(args_digest), sqlc.arg(request_digest),
  sqlc.arg(started_at)
)
RETURNING id, refund_id, attempt_no, river_job_id, river_attempt, args_digest,
  request_digest, outcome, evidence_digest, started_at, completed_at;

-- name: CompleteWeChatShopRefundAttempt :one
UPDATE public.order_wechat_shop_refund_attempts
SET outcome = sqlc.arg(outcome), evidence_digest = sqlc.narg(evidence_digest),
  completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(attempt_id) AND outcome IS NULL AND completed_at IS NULL
RETURNING id, refund_id, attempt_no, river_job_id, river_attempt, args_digest,
  request_digest, outcome, evidence_digest, started_at, completed_at;

-- name: CompleteWeChatShopRefundExecution :one
UPDATE public.order_wechat_shop_refunds
SET state = sqlc.arg(state), provider_acceptance_digest = sqlc.narg(provider_acceptance_digest),
  provider_aftersale_id = sqlc.narg(provider_aftersale_id),
  version = version + 1, updated_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(refund_id) AND state = 'executing'
  AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: ReserveWeChatShopRefundCallback :one
INSERT INTO public.order_wechat_shop_refund_callbacks (
  refund_id, contract_version, provider_event_digest, payload_digest,
  provider_refund_digest, provider_aftersale_id, provider_status, received_at
) VALUES (
  sqlc.arg(refund_id), 'provider/v2', sqlc.arg(provider_event_digest),
  sqlc.arg(payload_digest), sqlc.arg(provider_refund_digest),
  sqlc.arg(provider_aftersale_id), sqlc.arg(provider_status), sqlc.arg(received_at)
)
ON CONFLICT (provider_event_digest) DO NOTHING
RETURNING *;

-- name: GetWeChatShopRefundCallback :one
SELECT *
FROM public.order_wechat_shop_refund_callbacks
WHERE provider_event_digest = sqlc.arg(provider_event_digest);

-- name: CompleteWeChatShopRefundCallback :one
UPDATE public.order_wechat_shop_refund_callbacks
SET outcome = sqlc.arg(outcome), result_digest = sqlc.arg(result_digest),
  river_job_id = sqlc.arg(river_job_id), state = 'completed', completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(callback_id) AND state = 'reserved'
RETURNING *;

-- name: ApplyWeChatShopRefundSettlement :one
UPDATE public.order_wechat_shop_refunds
SET state = 'succeeded', provider_refund_digest = sqlc.arg(provider_refund_digest),
  settlement_receipt_digest = sqlc.arg(settlement_receipt_digest),
  settled_at = sqlc.arg(settled_at), updated_at = sqlc.arg(settled_at),
  version = version + 1
WHERE id = sqlc.arg(refund_id)
  AND state IN ('executing','provider_accepted','outcome_unknown')
  AND amount_minor = sqlc.arg(amount_minor) AND currency = sqlc.arg(currency)
  AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: MarkWeChatShopRefundFinalFailed :one
UPDATE public.order_wechat_shop_refunds
SET state = 'final_failed', provider_acceptance_digest = NULL,
  version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(refund_id)
  AND state IN ('executing','provider_accepted','outcome_unknown')
  AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: InsertWeChatShopRefundQuery :one
INSERT INTO public.order_wechat_shop_refund_queries (
  refund_id, evidence_digest, provider_refund_digest, amount_minor,
  currency, outcome, recorded_at
) VALUES (
  sqlc.arg(refund_id), sqlc.arg(evidence_digest), sqlc.narg(provider_refund_digest),
  sqlc.arg(amount_minor), sqlc.arg(currency), sqlc.arg(outcome), sqlc.arg(recorded_at)
)
ON CONFLICT (refund_id, evidence_digest) DO NOTHING
RETURNING id, refund_id, evidence_digest, provider_refund_digest,
  amount_minor, currency, outcome, recorded_at;

-- name: GetWeChatShopRefundQuery :one
SELECT id, refund_id, evidence_digest, provider_refund_digest,
  amount_minor, currency, outcome, recorded_at
FROM public.order_wechat_shop_refund_queries
WHERE refund_id = sqlc.arg(refund_id) AND evidence_digest = sqlc.arg(evidence_digest);
