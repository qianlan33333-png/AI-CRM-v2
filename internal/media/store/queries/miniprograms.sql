-- name: ListMediaMiniPrograms :many
SELECT id, name, app_id, page_path, title, thumbnail_image_url, thumbnail_image_id,
       thumbnail_media_id, thumbnail_media_expires_at, enabled, created_by, updated_by,
       version, created_at, updated_at
FROM media_miniprograms
WHERE (NOT sqlc.arg(enabled_only)::boolean OR enabled = TRUE)
  AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)::text) in lower(name)) > 0
       OR position(lower(sqlc.arg(search)::text) in lower(app_id)) > 0
       OR position(lower(sqlc.arg(search)::text) in lower(title)) > 0)
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountMediaMiniPrograms :one
SELECT count(*)
FROM media_miniprograms
WHERE (NOT sqlc.arg(enabled_only)::boolean OR enabled = TRUE)
  AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)::text) in lower(name)) > 0
       OR position(lower(sqlc.arg(search)::text) in lower(app_id)) > 0
       OR position(lower(sqlc.arg(search)::text) in lower(title)) > 0);

-- name: GetMediaMiniProgram :one
SELECT id, name, app_id, page_path, title, thumbnail_image_url, thumbnail_image_id,
       thumbnail_media_id, thumbnail_media_expires_at, enabled, created_by, updated_by,
       version, created_at, updated_at
FROM media_miniprograms
WHERE id = sqlc.arg(id)::bigint;

-- name: LockMediaMiniProgram :one
SELECT id FROM media_miniprograms WHERE id = sqlc.arg(id)::bigint FOR UPDATE;

-- name: CreateMediaMiniProgram :one
INSERT INTO media_miniprograms (
  name, app_id, page_path, title, thumbnail_image_url, thumbnail_image_id,
  thumbnail_media_id, thumbnail_media_expires_at, enabled, created_by, updated_by,
  version, created_at, updated_at
) VALUES (
  sqlc.arg(name)::text, sqlc.arg(app_id)::text, sqlc.arg(page_path)::text, sqlc.arg(title)::text,
  sqlc.arg(thumbnail_image_url)::text, sqlc.narg(thumbnail_image_id)::bigint,
  sqlc.arg(thumbnail_media_id)::text, sqlc.narg(thumbnail_media_expires_at)::timestamptz,
  sqlc.arg(enabled)::boolean, sqlc.arg(created_by)::bigint, sqlc.arg(updated_by)::bigint,
  sqlc.arg(version)::bigint, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
)
RETURNING id;

-- name: UpdateMediaMiniProgram :exec
UPDATE media_miniprograms
SET name = sqlc.arg(name)::text, app_id = sqlc.arg(app_id)::text,
    page_path = sqlc.arg(page_path)::text, title = sqlc.arg(title)::text,
    thumbnail_image_url = sqlc.arg(thumbnail_image_url)::text,
    thumbnail_image_id = sqlc.narg(thumbnail_image_id)::bigint,
    thumbnail_media_id = sqlc.arg(thumbnail_media_id)::text,
    thumbnail_media_expires_at = sqlc.narg(thumbnail_media_expires_at)::timestamptz,
    enabled = sqlc.arg(enabled)::boolean, updated_by = sqlc.arg(updated_by)::bigint,
    version = sqlc.arg(version)::bigint, updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint;

-- name: DeleteMediaMiniProgram :exec
DELETE FROM media_miniprograms WHERE id = sqlc.arg(id)::bigint;

-- name: GetMediaThumbnailCache :one
SELECT state, cache_receipt, media_id, expires_at
FROM media_thumbnail_cache_entries
WHERE image_id = sqlc.arg(image_id)::bigint;

-- name: ReserveMediaMiniProgramReceipt :one
INSERT INTO media_miniprogram_operation_receipts (
  operation, actor_scope, business_key, key_digest, payload_digest, created_at
) VALUES (
  sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(business_key)::text,
  sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (operation, actor_scope, business_key, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;

-- name: GetMediaMiniProgramReceipt :one
SELECT id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot
FROM media_miniprogram_operation_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text
  AND business_key = sqlc.arg(business_key)::text AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteMediaMiniProgramReceipt :one
UPDATE media_miniprogram_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;
