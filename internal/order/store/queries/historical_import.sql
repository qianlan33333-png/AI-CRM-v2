-- name: CreateHistoricalOrder :one
INSERT INTO order_list_projections (
 record_origin,provider,provider_label,merchant_order_no,platform_transaction_no,
 customer_id,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,
 product_id,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at)
VALUES ('v1_history','wechat',sqlc.arg(provider_label),sqlc.arg(merchant_order_no),sqlc.arg(platform_transaction_no),
 sqlc.narg(customer_id),sqlc.arg(payer_name_snapshot),sqlc.arg(mobile_snapshot),sqlc.arg(identity_kind),sqlc.arg(identity_value),
 sqlc.narg(product_id),sqlc.arg(product_code),sqlc.arg(product_name_snapshot),sqlc.arg(amount_minor),'CNY',
 sqlc.arg(status),sqlc.arg(status_label),sqlc.arg(detail_url),sqlc.arg(created_at),sqlc.arg(updated_at))
ON CONFLICT (provider,merchant_order_no) DO NOTHING
RETURNING id,record_origin,provider,provider_label,merchant_order_no,platform_transaction_no,
 customer_id,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,
 product_id,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at;

-- name: GetHistoricalOrder :one
SELECT id,record_origin,provider,provider_label,merchant_order_no,platform_transaction_no,
 customer_id,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,
 product_id,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at
FROM order_list_projections WHERE id=sqlc.arg(id)::bigint AND record_origin='v1_history' FOR UPDATE;

-- name: CreateHistoricalRefund :one
INSERT INTO order_historical_refunds (
 order_id,source_refund_id,refund_number,provider_refund_id,transaction_id,status,
 amount_minor,order_amount_minor,currency,reason,created_at,updated_at)
SELECT orders.id,sqlc.arg(source_refund_id)::bigint,sqlc.arg(refund_number)::text,
 sqlc.arg(provider_refund_id)::text,sqlc.arg(transaction_id)::text,sqlc.arg(status)::text,
 sqlc.arg(amount_minor)::bigint,sqlc.arg(order_amount_minor)::bigint,'CNY',sqlc.arg(reason)::text,
 sqlc.arg(created_at)::timestamptz,sqlc.arg(updated_at)::timestamptz
FROM order_list_projections orders
WHERE orders.id=sqlc.arg(order_id)::bigint AND orders.record_origin='v1_history'
 AND orders.currency='CNY' AND orders.amount_minor=sqlc.arg(order_amount_minor)::bigint
 AND (sqlc.arg(transaction_id)::text='' OR orders.platform_transaction_no=sqlc.arg(transaction_id)::text)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetHistoricalRefund :one
SELECT * FROM order_historical_refunds WHERE id=sqlc.arg(id)::bigint FOR UPDATE;

-- name: ListHistoricalOrderRefunds :many
SELECT * FROM order_historical_refunds WHERE order_id=sqlc.arg(order_id)::bigint
ORDER BY created_at,id;
