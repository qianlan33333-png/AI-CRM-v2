-- name: ListOrderProjections :many
SELECT id, provider, provider_label, merchant_order_no, platform_transaction_no,
       customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
       product_id, product_code, product_name_snapshot, amount_minor, currency,
       status, status_label, detail_url, created_at, updated_at
FROM order_list_projections
WHERE (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(order_no)::text IS NULL OR merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(mobile)::text IS NULL OR mobile_snapshot ILIKE '%' || sqlc.narg(mobile)::text || '%')
  AND (sqlc.narg(product_code)::text IS NULL OR product_code = sqlc.narg(product_code)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at <= sqlc.narg(created_to)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountAllOrderProjections :one
SELECT total_orders FROM order_list_projection_counters WHERE singleton = TRUE;

-- name: CountFilteredOrderProjections :one
SELECT count(*)
FROM order_list_projections
WHERE (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
  AND (sqlc.narg(order_no)::text IS NULL OR merchant_order_no ILIKE '%' || sqlc.narg(order_no)::text || '%')
  AND (sqlc.narg(mobile)::text IS NULL OR mobile_snapshot ILIKE '%' || sqlc.narg(mobile)::text || '%')
  AND (sqlc.narg(product_code)::text IS NULL OR product_code = sqlc.narg(product_code)::text)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at <= sqlc.narg(created_to)::timestamptz);
