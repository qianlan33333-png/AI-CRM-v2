-- name: ReserveInternalEventSafeExportReceipt :one
INSERT INTO public.internal_event_safe_export_receipts(actor_id,key_digest,payload_digest,created_at)
VALUES (sqlc.arg(actor_id)::bigint,sqlc.arg(key_digest)::bytea,sqlc.arg(payload_digest)::bytea,sqlc.arg(created_at)::timestamptz)
ON CONFLICT (actor_id,key_digest) DO NOTHING
RETURNING id,payload_digest,result_snapshot,(state='completed') AS completed;

-- name: GetInternalEventSafeExportReceipt :one
SELECT id,payload_digest,result_snapshot,(state='completed') AS completed
FROM public.internal_event_safe_export_receipts
WHERE actor_id=sqlc.arg(actor_id)::bigint AND key_digest=sqlc.arg(key_digest)::bytea
FOR UPDATE;

-- name: ListInternalEventSafeExportSourceRows :many
SELECT event.id,event.event_type,event.occurred_at,event.dispatched,
       delivery.consumer,delivery.status,delivery.attempt_count,delivery.completed_at
FROM event_log AS event
LEFT JOIN event_deliveries AS delivery ON delivery.event_id=event.id
WHERE event.occurred_at <= sqlc.arg(watermark)::timestamptz
  AND event.id <= sqlc.arg(upper_event_id)::bigint
  AND (sqlc.arg(event_type)::text='' OR event.event_type=sqlc.arg(event_type)::text)
  AND (sqlc.arg(consumer)::text='' OR delivery.consumer=sqlc.arg(consumer)::text)
  AND (sqlc.arg(status)::text='' OR delivery.status=sqlc.arg(status)::text)
ORDER BY event.occurred_at,event.id,delivery.consumer NULLS FIRST
LIMIT sqlc.arg(row_limit)::integer;

-- name: GetInternalEventSafeExportUpperEventID :one
SELECT COALESCE(max(id),0)::bigint AS upper_event_id
FROM public.event_log
WHERE occurred_at <= sqlc.arg(watermark)::timestamptz;

-- name: InsertInternalEventSafeExport :exec
INSERT INTO public.internal_event_safe_exports(id,actor_id,filter_digest,watermark,upper_event_id,record_count,created_at)
VALUES (sqlc.arg(id)::text,sqlc.arg(actor_id)::bigint,sqlc.arg(filter_digest)::bytea,sqlc.arg(watermark)::timestamptz,sqlc.arg(upper_event_id)::bigint,sqlc.arg(record_count)::integer,sqlc.arg(created_at)::timestamptz);

-- name: InsertInternalEventSafeExportRow :exec
INSERT INTO public.internal_event_safe_export_rows(export_id,row_index,event_id,event_type,occurred_at,dispatched,consumer,status,attempt_count,completed_at)
VALUES (sqlc.arg(export_id)::text,sqlc.arg(row_index)::integer,sqlc.arg(event_id)::bigint,sqlc.arg(event_type)::text,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(dispatched)::boolean,sqlc.narg(consumer)::text,sqlc.narg(status)::text,sqlc.narg(attempt_count)::integer,sqlc.narg(completed_at)::timestamptz);

-- name: CompleteInternalEventSafeExportReceipt :one
UPDATE public.internal_event_safe_export_receipts
SET export_id=sqlc.arg(export_id)::text,state='completed',result_snapshot=sqlc.arg(result_snapshot)::jsonb,completed_at=sqlc.arg(completed_at)::timestamptz
WHERE id=sqlc.arg(id)::bigint AND state='reserved'
RETURNING id,payload_digest,result_snapshot,(state='completed') AS completed;

-- name: GetInternalEventSafeExport :one
SELECT id,record_count,watermark,created_at
FROM public.internal_event_safe_exports
WHERE id=sqlc.arg(id)::text AND actor_id=sqlc.arg(actor_id)::bigint;

-- name: ListInternalEventSafeExportRows :many
SELECT event_id,event_type,occurred_at,dispatched,consumer,status,attempt_count,completed_at
FROM public.internal_event_safe_export_rows
WHERE export_id=sqlc.arg(export_id)::text
ORDER BY row_index;
