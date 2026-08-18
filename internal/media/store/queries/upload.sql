-- name: ReserveMediaImageUploadReceipt :one
INSERT INTO media_image_upload_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES ('upload', sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetMediaImageUploadReceipt :one
SELECT id, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM media_image_upload_receipts
WHERE operation = 'upload' AND actor_scope = sqlc.arg(actor_scope)::text AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: InsertMediaImage :one
-- This legacy default-true path must remain executable at the historical
-- H01A1 waterline (migration 30), before migration 47 added enabled.
INSERT INTO media_images (name, file_name, mime_type, file_size, width, height, checksum, description, tags, category, created_by, created_at, updated_at)
VALUES (sqlc.arg(name)::text, sqlc.arg(file_name)::text, sqlc.arg(mime_type)::text, sqlc.arg(file_size)::integer,
        sqlc.arg(width)::integer, sqlc.arg(height)::integer, sqlc.arg(checksum)::bytea, sqlc.arg(description)::text,
        sqlc.arg(tags)::text, sqlc.arg(category)::text, sqlc.arg(created_by)::bigint,
        sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz)
RETURNING id, name, file_name, mime_type, file_size, width, height, description, tags, category, created_at, updated_at;

-- name: InsertMediaImageWithEnabled :one
-- Only the 0357 explicit-false branch requires migration 47's enabled column.
INSERT INTO media_images (name, file_name, mime_type, file_size, width, height, checksum, description, tags, category, enabled, created_by, created_at, updated_at)
VALUES (sqlc.arg(name)::text, sqlc.arg(file_name)::text, sqlc.arg(mime_type)::text, sqlc.arg(file_size)::integer,
        sqlc.arg(width)::integer, sqlc.arg(height)::integer, sqlc.arg(checksum)::bytea, sqlc.arg(description)::text,
        sqlc.arg(tags)::text, sqlc.arg(category)::text, sqlc.arg(enabled)::boolean, sqlc.arg(created_by)::bigint,
        sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz)
RETURNING id, name, file_name, mime_type, file_size, width, height, description, tags, category, enabled, created_at, updated_at;

-- name: InsertMediaImageBlob :exec
INSERT INTO media_image_blobs (image_id, content, checksum, created_at)
VALUES (sqlc.arg(image_id)::bigint, sqlc.arg(content)::bytea, sqlc.arg(checksum)::bytea, sqlc.arg(created_at)::timestamptz);

-- name: CompleteMediaImageUploadReceipt :one
UPDATE media_image_upload_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb, completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, actor_scope, key_digest, payload_digest, state, result_snapshot;
