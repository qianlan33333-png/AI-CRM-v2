-- name: ListOrderProjections :many
SELECT id, record_origin, provider, provider_label, merchant_order_no, platform_transaction_no,
       customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
       product_id, product_code, product_name_snapshot, amount_minor, currency,
       status, status_label, detail_url, created_at, updated_at
FROM order_list_projections
WHERE (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(order_no)::text IS NULL OR merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(mobile)::text IS NULL OR mobile_snapshot ILIKE '%' || sqlc.narg(mobile)::text || '%')
  AND (sqlc.narg(product_code)::text IS NULL OR product_code = sqlc.narg(product_code)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(customer_id)::bigint IS NULL OR customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at <= sqlc.narg(created_to)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountAllOrderProjections :one
SELECT total_orders FROM order_list_projection_counters WHERE singleton = TRUE;

-- name: GetPaidOrderProjection :one
SELECT id, product_id, customer_id
FROM order_list_projections
WHERE id = sqlc.arg(order_id)::bigint
  AND status = 'paid'
  AND record_origin = 'native'
  AND product_id IS NOT NULL
  AND customer_id IS NOT NULL
FOR UPDATE;

-- name: CountFilteredOrderProjections :one
SELECT count(*)
FROM order_list_projections
WHERE (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(order_no)::text IS NULL OR merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(mobile)::text IS NULL OR mobile_snapshot ILIKE '%' || sqlc.narg(mobile)::text || '%')
  AND (sqlc.narg(product_code)::text IS NULL OR product_code = sqlc.narg(product_code)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(customer_id)::bigint IS NULL OR customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at <= sqlc.narg(created_to)::timestamptz);

-- name: ListBoardOrders :many
SELECT id, record_origin, provider, provider_label, merchant_order_no, platform_transaction_no,
       customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
       product_id, product_code, product_name_snapshot, amount_minor, currency,
       status, status_label, detail_url, created_at, updated_at
FROM order_list_projections
WHERE (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(product_code)::text IS NULL OR product_code = sqlc.narg(product_code)::text)
  AND (sqlc.narg(mobile)::text IS NULL OR mobile_snapshot ILIKE '%' || sqlc.narg(mobile)::text || '%')
  AND (sqlc.narg(identity)::text IS NULL OR identity_value ILIKE '%' || sqlc.narg(identity)::text || '%')
  AND (sqlc.narg(transaction_id)::text IS NULL OR platform_transaction_no ILIKE '%' || sqlc.narg(transaction_id)::text || '%')
  AND (sqlc.narg(order_no)::text IS NULL OR merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at <= sqlc.narg(created_to)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountBoardOrders :one
SELECT count(*)
FROM order_list_projections
WHERE (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(product_code)::text IS NULL OR product_code = sqlc.narg(product_code)::text)
  AND (sqlc.narg(mobile)::text IS NULL OR mobile_snapshot ILIKE '%' || sqlc.narg(mobile)::text || '%')
  AND (sqlc.narg(identity)::text IS NULL OR identity_value ILIKE '%' || sqlc.narg(identity)::text || '%')
  AND (sqlc.narg(transaction_id)::text IS NULL OR platform_transaction_no ILIKE '%' || sqlc.narg(transaction_id)::text || '%')
  AND (sqlc.narg(order_no)::text IS NULL OR merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at <= sqlc.narg(created_to)::timestamptz);

-- name: GetBoardOrder :one
SELECT id, record_origin, provider, provider_label, merchant_order_no, platform_transaction_no,
       customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
       product_id, product_code, product_name_snapshot, amount_minor, currency,
       status, status_label, detail_url, created_at, updated_at
FROM order_list_projections
WHERE (sqlc.arg(provider)::text = 'auto' OR provider = sqlc.arg(provider)::text)
  AND (merchant_order_no = sqlc.arg(order_reference)::text
       OR platform_transaction_no = sqlc.arg(order_reference)::text
       OR id::text = sqlc.arg(order_reference)::text)
ORDER BY id DESC LIMIT 1;

-- name: GetBoardOrderByID :one
SELECT id, record_origin, provider, provider_label, merchant_order_no, platform_transaction_no,
       customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
       product_id, product_code, product_name_snapshot, amount_minor, currency,
       status, status_label, detail_url, created_at, updated_at
FROM order_list_projections
WHERE id = sqlc.arg(id)::bigint;

-- name: GetBoardOrderForUpdate :one
SELECT id, record_origin, provider, provider_label, merchant_order_no, platform_transaction_no,
       customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
       product_id, product_code, product_name_snapshot, amount_minor, currency,
       status, status_label, detail_url, created_at, updated_at
FROM order_list_projections
WHERE (sqlc.arg(provider)::text = 'auto' OR provider = sqlc.arg(provider)::text)
  AND (merchant_order_no = sqlc.arg(order_reference)::text
       OR platform_transaction_no = sqlc.arg(order_reference)::text
       OR id::text = sqlc.arg(order_reference)::text)
  AND record_origin = 'native'
ORDER BY id DESC LIMIT 1 FOR UPDATE;

-- name: CountActiveRefundAmount :one
SELECT COALESCE(sum(refund_amount_total), 0)::bigint
FROM order_refunds
WHERE order_id = sqlc.arg(order_id)::bigint
  AND status IN ('pending_external_gate', 'outcome_unknown', 'completed');

-- name: ReserveOrderOperationReceipt :one
INSERT INTO order_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea,
        sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetOrderOperationReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM order_operation_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteOrderOperationReceipt :one
UPDATE order_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: CreateOrderExportJob :one
INSERT INTO order_export_jobs (job_id, resource, format, operator_id, content_text, created_at)
VALUES (sqlc.arg(job_id)::text, sqlc.arg(resource)::text, sqlc.arg(format)::text,
        sqlc.arg(operator_id)::bigint, sqlc.arg(content_text)::text, sqlc.arg(created_at)::timestamptz)
RETURNING job_id, resource, format, operator_id, content_text, created_at;

-- name: GetOrderExportJob :one
SELECT job_id, resource, format, operator_id, content_text, created_at
FROM order_export_jobs WHERE job_id = sqlc.arg(job_id)::text;

-- name: CreateOrderExternalEffect :one
INSERT INTO order_external_effects (order_id, provider, effect_kind, state, provider_receipt, created_at, updated_at)
VALUES (sqlc.arg(order_id)::bigint, sqlc.arg(provider)::text, sqlc.arg(effect_kind)::text,
        sqlc.arg(state)::text, sqlc.narg(provider_receipt)::jsonb,
        sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz)
RETURNING id, order_id, provider, effect_kind, state, auto_retry_allowed, provider_receipt,
          manual_review_requested_at, created_at, updated_at;

-- name: GetOrderExternalEffect :one
SELECT id, order_id, provider, effect_kind, state, auto_retry_allowed, provider_receipt,
       manual_review_requested_at, created_at, updated_at
FROM order_external_effects WHERE id = sqlc.arg(id)::bigint;

-- name: GetOrderExternalEffectForUpdate :one
SELECT id, order_id, provider, effect_kind, state, auto_retry_allowed, provider_receipt,
       manual_review_requested_at, created_at, updated_at
FROM order_external_effects WHERE id = sqlc.arg(id)::bigint FOR UPDATE;

-- name: ListOrderExternalEffects :many
SELECT id, order_id, provider, effect_kind, state, auto_retry_allowed, provider_receipt,
       manual_review_requested_at, created_at, updated_at
FROM order_external_effects
WHERE order_id = sqlc.arg(order_id)::bigint
ORDER BY created_at DESC, id DESC;

-- name: CountOrderExternalEffects :one
SELECT count(*) FROM order_external_effects WHERE order_id = sqlc.arg(order_id)::bigint;

-- name: MarkOrderExternalEffectManualReview :one
UPDATE order_external_effects
SET manual_review_requested_at = sqlc.arg(reviewed_at)::timestamptz,
    updated_at = sqlc.arg(reviewed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint
RETURNING id, order_id, provider, effect_kind, state, auto_retry_allowed, provider_receipt,
          manual_review_requested_at, created_at, updated_at;

-- name: CreateOrderRefund :one
INSERT INTO order_refunds (order_id, external_effect_id, provider, refund_id, out_refund_no,
                           refund_amount_total, currency, reason, status, created_at)
VALUES (sqlc.arg(order_id)::bigint, sqlc.arg(external_effect_id)::bigint, sqlc.arg(provider)::text,
        sqlc.arg(refund_id)::text, sqlc.arg(out_refund_no)::text, sqlc.arg(refund_amount_total)::bigint,
        sqlc.arg(currency)::text, sqlc.arg(reason)::text, sqlc.arg(status)::text, sqlc.arg(created_at)::timestamptz)
RETURNING id, order_id, external_effect_id, provider, refund_id, out_refund_no, refund_amount_total,
          currency, reason, status, created_at;

-- name: ListOrderRefunds :many
SELECT refund.id, refund.order_id, refund.external_effect_id, refund.provider, refund.refund_id,
       refund.out_refund_no, refund.refund_amount_total, refund.currency, refund.reason, refund.status,
       refund.created_at, order_projection.merchant_order_no, order_projection.platform_transaction_no,
       effect.state AS external_effect_state, effect.auto_retry_allowed
FROM order_refunds AS refund
JOIN order_list_projections AS order_projection ON order_projection.id = refund.order_id
JOIN order_external_effects AS effect ON effect.id = refund.external_effect_id
WHERE (sqlc.narg(provider)::text IS NULL OR refund.provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(order_no)::text IS NULL OR order_projection.merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(transaction_id)::text IS NULL OR order_projection.platform_transaction_no ILIKE '%' || sqlc.narg(transaction_id)::text || '%')
  AND (sqlc.narg(refund_id)::text IS NULL OR refund.refund_id ILIKE '%' || sqlc.narg(refund_id)::text || '%')
  AND (sqlc.narg(out_refund_no)::text IS NULL OR refund.out_refund_no ILIKE '%' || sqlc.narg(out_refund_no)::text || '%')
  AND (sqlc.narg(status)::text IS NULL OR refund.status = sqlc.narg(status)::text)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR refund.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR refund.created_at <= sqlc.narg(created_to)::timestamptz)
ORDER BY refund.created_at DESC, refund.id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountOrderRefunds :one
SELECT count(*)
FROM order_refunds AS refund
JOIN order_list_projections AS order_projection ON order_projection.id = refund.order_id
WHERE (sqlc.narg(provider)::text IS NULL OR refund.provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(order_no)::text IS NULL OR order_projection.merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(transaction_id)::text IS NULL OR order_projection.platform_transaction_no ILIKE '%' || sqlc.narg(transaction_id)::text || '%')
  AND (sqlc.narg(refund_id)::text IS NULL OR refund.refund_id ILIKE '%' || sqlc.narg(refund_id)::text || '%')
  AND (sqlc.narg(out_refund_no)::text IS NULL OR refund.out_refund_no ILIKE '%' || sqlc.narg(out_refund_no)::text || '%')
  AND (sqlc.narg(status)::text IS NULL OR refund.status = sqlc.narg(status)::text)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR refund.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR refund.created_at <= sqlc.narg(created_to)::timestamptz);

-- name: GetOrderRefundByID :one
SELECT refund.id, refund.order_id, refund.external_effect_id, refund.provider, refund.refund_id,
       refund.out_refund_no, refund.refund_amount_total, refund.currency, refund.reason,
       refund.status, refund.created_at, order_projection.merchant_order_no,
       order_projection.platform_transaction_no, effect.state AS external_effect_state,
       effect.auto_retry_allowed
FROM order_refunds AS refund
JOIN order_list_projections AS order_projection ON order_projection.id = refund.order_id
JOIN order_external_effects AS effect ON effect.id = refund.external_effect_id
WHERE refund.id = sqlc.arg(id)::bigint;
