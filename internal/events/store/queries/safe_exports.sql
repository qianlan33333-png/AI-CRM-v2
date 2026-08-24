-- name: ReserveInternalEventSafeExportReceipt :one
INSERT INTO public.internal_event_safe_export_receipts(actor_id,key_digest,payload_digest,created_at)
VALUES (sqlc.arg(actor_id)::bigint,sqlc.arg(key_digest)::bytea,sqlc.arg(payload_digest)::bytea,sqlc.arg(created_at)::timestamptz)
ON CONFLICT (actor_id,key_digest) DO NOTHING
RETURNING id,payload_digest,result_digest,result_snapshot,(state='completed') AS completed;

-- name: GetInternalEventSafeExportReceipt :one
SELECT id,payload_digest,result_digest,result_snapshot,(state='completed') AS completed
FROM public.internal_event_safe_export_receipts
WHERE actor_id=sqlc.arg(actor_id)::bigint AND key_digest=sqlc.arg(key_digest)::bytea
FOR UPDATE;

-- name: ListInternalEventSafeExportSourceSnapshot :many
WITH bounds AS (
  SELECT statement_timestamp() AS watermark,
         COALESCE((SELECT max(id) FROM public.event_log WHERE occurred_at <= statement_timestamp()),0)::bigint AS upper_event_id
), source AS (
  SELECT event.id,event.event_type,event.occurred_at,event.dispatched,
         delivery.consumer,delivery.status,delivery.attempt_count,delivery.completed_at
  FROM bounds
  JOIN public.event_log AS event
    ON event.occurred_at <= bounds.watermark AND event.id <= bounds.upper_event_id
  LEFT JOIN public.event_deliveries AS delivery ON delivery.event_id=event.id
  WHERE (sqlc.arg(event_type)::text='' OR event.event_type=sqlc.arg(event_type)::text)
    AND (sqlc.arg(consumer)::text='' OR delivery.consumer=sqlc.arg(consumer)::text)
    AND (sqlc.arg(status)::text='' OR delivery.status=sqlc.arg(status)::text)
  ORDER BY event.occurred_at,event.id,delivery.consumer NULLS FIRST
  LIMIT sqlc.arg(row_limit)::integer
)
SELECT bounds.watermark::timestamptz AS watermark,bounds.upper_event_id,
       source.id,source.event_type,source.occurred_at,source.dispatched,
       source.consumer,source.status,source.attempt_count,source.completed_at
FROM bounds
LEFT JOIN source ON TRUE
ORDER BY source.occurred_at,source.id,source.consumer NULLS FIRST;

-- name: InsertInternalEventSafeExport :exec
INSERT INTO public.internal_event_safe_exports(id,actor_id,filter_digest,digest_version,rows_digest,result_digest,watermark,upper_event_id,record_count,created_at)
VALUES (sqlc.arg(id)::text,sqlc.arg(actor_id)::bigint,sqlc.arg(filter_digest)::bytea,sqlc.arg(digest_version)::smallint,sqlc.arg(rows_digest)::bytea,sqlc.arg(result_digest)::bytea,sqlc.arg(watermark)::timestamptz,sqlc.arg(upper_event_id)::bigint,sqlc.arg(record_count)::integer,sqlc.arg(created_at)::timestamptz);

-- name: InsertInternalEventSafeExportRow :exec
INSERT INTO public.internal_event_safe_export_rows(export_id,row_index,event_id,event_type,occurred_at,dispatched,consumer,status,attempt_count,completed_at)
VALUES (sqlc.arg(export_id)::text,sqlc.arg(row_index)::integer,sqlc.arg(event_id)::bigint,sqlc.arg(event_type)::text,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(dispatched)::boolean,sqlc.narg(consumer)::text,sqlc.narg(status)::text,sqlc.narg(attempt_count)::integer,sqlc.narg(completed_at)::timestamptz);

-- name: CompleteInternalEventSafeExportReceipt :one
UPDATE public.internal_event_safe_export_receipts
SET export_id=sqlc.arg(export_id)::text,state='completed',result_snapshot=sqlc.arg(result_snapshot)::jsonb,completed_at=sqlc.arg(completed_at)::timestamptz
    ,result_digest=sqlc.arg(result_digest)::bytea
WHERE id=sqlc.arg(id)::bigint AND state='reserved'
RETURNING id,payload_digest,result_digest,result_snapshot,(state='completed') AS completed;

-- name: GetInternalEventSafeExport :one
SELECT id,actor_id,filter_digest,digest_version,rows_digest,result_digest,upper_event_id,record_count,watermark,created_at
FROM public.internal_event_safe_exports
WHERE id=sqlc.arg(id)::text AND actor_id=sqlc.arg(actor_id)::bigint;

-- name: InternalEventSafeExportReceiptExists :one
SELECT EXISTS(
  SELECT 1 FROM public.internal_event_safe_export_receipts
  WHERE export_id=sqlc.arg(export_id)::text AND actor_id=sqlc.arg(actor_id)::bigint
) AS receipt_exists;

-- name: ListInternalEventSafeExportRows :many
SELECT event_id,event_type,occurred_at,dispatched,consumer,status,attempt_count,completed_at
FROM public.internal_event_safe_export_rows
WHERE export_id=sqlc.arg(export_id)::text
ORDER BY row_index;

-- name: GetInternalEventSafeExportIntegrity :one
SELECT receipt.id AS receipt_id,receipt.payload_digest,receipt.result_digest AS receipt_result_digest,
       receipt.result_snapshot,event.event_type AS audit_event_type,
       event.idempotency_key AS audit_idempotency_key,event.occurred_at AS audit_occurred_at,
       event.payload AS audit_payload
FROM public.internal_event_safe_export_receipts AS receipt
JOIN public.event_log AS event
  ON event.idempotency_key='internal-event-safe-export:' || receipt.id::text
WHERE receipt.export_id=sqlc.arg(export_id)::text AND receipt.state='completed'
FOR SHARE OF event;
