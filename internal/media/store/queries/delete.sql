-- name: LockMediaImageForDelete :one
SELECT id
FROM media_images
WHERE id = sqlc.arg(image_id)::bigint
FOR UPDATE;

-- name: ListMediaImageDeleteMiniprogramReferences :many
SELECT id
FROM media_miniprograms
WHERE thumbnail_image_id = sqlc.arg(image_id)::bigint
ORDER BY id ASC;

-- name: ListMediaImageDeleteGroupInviteReferences :many
SELECT id
FROM media_group_invites
WHERE cover_image_id = sqlc.arg(image_id)::bigint
ORDER BY id ASC;

-- name: ListMediaImageDeleteImportPreflightReferences :many
SELECT DISTINCT preflight.id
FROM media_miniprogram_import_preflights AS preflight
JOIN media_miniprogram_import_ledger AS ledger ON ledger.preflight_id = preflight.id
WHERE preflight.state <> 'completed'
  AND ledger.target_media_image_id = sqlc.arg(image_id)::bigint
ORDER BY preflight.id ASC;

-- name: ReserveMediaImageDeleteReceipt :one
INSERT INTO media_image_delete_receipts (
  operation, actor_scope, business_key, key_digest, payload_digest, created_at
) VALUES (
  'delete', sqlc.arg(actor_scope)::text, sqlc.arg(business_key)::text,
  sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;

-- name: GetMediaImageDeleteReceipt :one
SELECT id, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot
FROM media_image_delete_receipts
WHERE operation = 'delete'
  AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: DeleteMediaImage :execrows
DELETE FROM media_images
WHERE id = sqlc.arg(image_id)::bigint;

-- name: CompleteMediaImageDeleteReceipt :one
UPDATE media_image_delete_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;
