-- name: ReserveCustomerSafeExportReceipt :one
INSERT INTO public.customer_safe_export_receipts(actor_id,key_digest,payload_digest,created_at)
VALUES(sqlc.arg(actor_id),sqlc.arg(key_digest),sqlc.arg(payload_digest),sqlc.arg(created_at))
ON CONFLICT(actor_id,key_digest) DO NOTHING
RETURNING id,payload_digest,result_snapshot,state='completed' AS completed;

-- name: GetCustomerSafeExportReceipt :one
SELECT id,payload_digest,result_snapshot,state='completed' AS completed
FROM public.customer_safe_export_receipts
WHERE actor_id=sqlc.arg(actor_id) AND key_digest=sqlc.arg(key_digest)
FOR UPDATE;

-- name: ListCustomerSafeExportSnapshotRows :many
SELECT c.id,c.name,c.owner_staff_id,c.stage_id,c.channel_id,c.added_at,c.last_interact_at
FROM public.customers c
WHERE c.updated_at <= sqlc.arg(watermark)::timestamptz
  AND NOT c.is_deleted
  AND (sqlc.narg(keyword)::text IS NULL OR lower(c.name) % lower(sqlc.narg(keyword)::text))
  AND (sqlc.narg(owner_staff_id)::bigint IS NULL OR c.owner_staff_id=sqlc.narg(owner_staff_id)::bigint)
  AND (sqlc.narg(stage_id)::bigint IS NULL OR c.stage_id=sqlc.narg(stage_id)::bigint)
  AND (sqlc.narg(channel_id)::bigint IS NULL OR c.channel_id=sqlc.narg(channel_id)::bigint)
  AND (sqlc.narg(tag_id)::bigint IS NULL OR EXISTS (SELECT 1 FROM public.customer_tags ct WHERE ct.customer_id=c.id AND ct.tag_id=sqlc.narg(tag_id)::bigint))
  AND (sqlc.narg(added_after)::timestamptz IS NULL OR c.added_at >= sqlc.narg(added_after)::timestamptz)
  AND (sqlc.narg(added_before)::timestamptz IS NULL OR c.added_at <= sqlc.narg(added_before)::timestamptz)
  AND (sqlc.narg(last_interact_after)::timestamptz IS NULL OR c.last_interact_at >= sqlc.narg(last_interact_after)::timestamptz)
  AND (sqlc.narg(last_interact_before)::timestamptz IS NULL OR c.last_interact_at <= sqlc.narg(last_interact_before)::timestamptz)
ORDER BY c.updated_at DESC,c.id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: InsertCustomerSafeExport :exec
INSERT INTO public.customer_safe_exports(id,actor_id,owner_scope_staff_id,filter_digest,watermark,record_count,created_at)
VALUES(sqlc.arg(id),sqlc.arg(actor_id),sqlc.narg(owner_scope_staff_id),sqlc.arg(filter_digest),sqlc.arg(watermark),sqlc.arg(record_count),sqlc.arg(created_at));

-- name: InsertCustomerSafeExportRow :exec
INSERT INTO public.customer_safe_export_rows(export_id,row_index,customer_id,display_name,owner_staff_id,stage_id,channel_id,added_at,last_interact_at)
VALUES(sqlc.arg(export_id),sqlc.arg(row_index),sqlc.arg(customer_id),sqlc.arg(display_name),sqlc.narg(owner_staff_id),sqlc.narg(stage_id),sqlc.narg(channel_id),sqlc.narg(added_at),sqlc.narg(last_interact_at));

-- name: CompleteCustomerSafeExportReceipt :one
WITH locked_export AS (
  SELECT e.id
  FROM public.customer_safe_exports e
  JOIN public.customer_safe_export_receipts r ON r.id=sqlc.arg(id) AND r.actor_id=e.actor_id
  WHERE e.id=sqlc.arg(export_id)
  FOR UPDATE OF e
)
UPDATE public.customer_safe_export_receipts r
SET export_id=sqlc.arg(export_id),state='completed',result_snapshot=sqlc.arg(result_snapshot),completed_at=sqlc.arg(completed_at)
FROM locked_export
WHERE r.id=sqlc.arg(id) AND r.state='reserved'
RETURNING r.id,r.payload_digest,r.result_snapshot,r.state='completed' AS completed;

-- name: GetCustomerSafeExport :one
SELECT id,record_count,watermark,created_at,owner_scope_staff_id
FROM public.customer_safe_exports
WHERE id=sqlc.arg(id) AND actor_id=sqlc.arg(actor_id);

-- name: ListLockedCustomerSafeExportRows :many
SELECT r.customer_id,r.display_name,r.owner_staff_id,r.stage_id,r.channel_id,r.added_at,r.last_interact_at,c.owner_staff_id AS current_owner_staff_id,c.is_deleted
FROM public.customer_safe_export_rows r
JOIN public.customer_safe_exports e ON e.id=r.export_id
JOIN public.customers c ON c.id=r.customer_id
WHERE r.export_id=sqlc.arg(export_id) AND e.actor_id=sqlc.arg(actor_id)
ORDER BY r.row_index
FOR SHARE OF c;
