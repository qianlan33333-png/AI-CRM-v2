-- name: CreateServicePeriodHistoryDefinition :one
INSERT INTO public.product_service_period_history
(source_definition_id, product_id, membership_config_id, membership_config_name, duration_days, deleted, created_at, updated_at)
SELECT sqlc.arg(source_definition_id), p.id, sqlc.arg(membership_config_id), sqlc.arg(membership_config_name),
  sqlc.arg(duration_days), sqlc.arg(deleted), sqlc.arg(created_at), sqlc.arg(updated_at)
FROM public.products p WHERE p.id=sqlc.arg(product_id) AND p.local_lifecycle='disabled' AND p.version=1
RETURNING *;

-- name: GetServicePeriodHistoryDefinition :one
SELECT * FROM public.product_service_period_history WHERE id=$1 FOR UPDATE;

-- name: CreateServicePeriodHistoryEntitlement :one
INSERT INTO public.product_service_period_entitlement_history
(source_entitlement_id,definition_id,customer_id,membership_config_id,status,start_at,end_at,last_order_id,last_out_trade_no,renewal_count,created_at,updated_at)
VALUES (sqlc.arg(source_entitlement_id),sqlc.arg(definition_id),sqlc.narg(customer_id),sqlc.arg(membership_config_id),sqlc.arg(status),sqlc.arg(start_at),sqlc.arg(end_at),sqlc.narg(last_order_id),sqlc.arg(last_out_trade_no),sqlc.arg(renewal_count),sqlc.arg(created_at),sqlc.arg(updated_at))
RETURNING *;

-- name: GetServicePeriodHistoryEntitlement :one
SELECT * FROM public.product_service_period_entitlement_history WHERE id=$1 FOR UPDATE;

-- name: CreateServicePeriodHistoryEvent :one
INSERT INTO public.product_service_period_event_history
(source_event_id,definition_id,entitlement_id,customer_id,order_id,event_id,event_type,duration_days,out_trade_no,before_start_at,before_end_at,after_start_at,after_end_at,created_at)
SELECT sqlc.arg(source_event_id),d.id,sqlc.narg(entitlement_id),sqlc.narg(customer_id),sqlc.narg(order_id),sqlc.arg(event_id),sqlc.arg(event_type),sqlc.arg(duration_days),sqlc.arg(out_trade_no),sqlc.narg(before_start_at),sqlc.narg(before_end_at),sqlc.narg(after_start_at),sqlc.narg(after_end_at),sqlc.arg(created_at)
FROM public.product_service_period_history d
WHERE d.id=sqlc.arg(definition_id) AND (sqlc.narg(entitlement_id)::bigint IS NULL OR EXISTS (
  SELECT 1 FROM public.product_service_period_entitlement_history e WHERE e.id=sqlc.narg(entitlement_id) AND e.definition_id=d.id
)) RETURNING *;

-- name: GetServicePeriodHistoryEvent :one
SELECT * FROM public.product_service_period_event_history WHERE id=$1 FOR UPDATE;

-- name: ListServicePeriodHistoryDefinitions :many
SELECT h.*,p.product_code,p.name AS product_name,p.price_minor,p.currency
FROM public.product_service_period_history h JOIN public.products p ON p.id=h.product_id
ORDER BY h.id LIMIT $1 OFFSET $2;

-- name: CountServicePeriodHistoryDefinitions :one
SELECT count(*) FROM public.product_service_period_history;

-- name: ListServicePeriodHistoryEntitlements :many
SELECT * FROM public.product_service_period_entitlement_history WHERE definition_id=$1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountServicePeriodHistoryEntitlements :one
SELECT count(*) FROM public.product_service_period_entitlement_history WHERE definition_id=$1;

-- name: ListServicePeriodHistoryEvents :many
SELECT * FROM public.product_service_period_event_history WHERE definition_id=$1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountServicePeriodHistoryEvents :one
SELECT count(*) FROM public.product_service_period_event_history WHERE definition_id=$1;
