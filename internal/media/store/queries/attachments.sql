-- name: ReserveMediaAttachmentMutation :one
INSERT INTO media_attachment_mutation_receipts (
  operation, actor_scope, business_key, key_digest, payload_digest, created_at
) VALUES (
  sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(business_key)::text,
  sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (operation, actor_scope, business_key, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;

-- name: GetMediaAttachmentMutation :one
SELECT id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot
FROM media_attachment_mutation_receipts
WHERE operation = sqlc.arg(operation)::text
  AND actor_scope = sqlc.arg(actor_scope)::text
  AND business_key = sqlc.arg(business_key)::text
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: InsertMediaAttachment :one
INSERT INTO media_attachments (
  name, file_name, mime_type, file_size, checksum, description, tags, enabled,
  version, created_by, updated_by, created_at, updated_at
) VALUES (
  sqlc.arg(name)::text, sqlc.arg(file_name)::text, sqlc.arg(mime_type)::text,
  sqlc.arg(file_size)::integer, sqlc.arg(checksum)::bytea, sqlc.arg(description)::text,
  sqlc.arg(tags)::jsonb, sqlc.arg(enabled)::boolean, 1, sqlc.arg(created_by)::bigint,
  sqlc.arg(updated_by)::bigint, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
)
RETURNING id, name, file_name, mime_type, file_size, description, tags, enabled,
          version, created_by, updated_by, created_at, updated_at;

-- name: InsertMediaAttachmentBlob :exec
INSERT INTO media_attachment_blobs (attachment_id, content, checksum, created_at)
VALUES (
  sqlc.arg(attachment_id)::bigint, sqlc.arg(content)::bytea,
  sqlc.arg(checksum)::bytea, sqlc.arg(created_at)::timestamptz
);

-- name: GetMediaAttachment :one
SELECT id, name, file_name, mime_type, file_size, description, tags, enabled,
       version, created_by, updated_by, created_at, updated_at
FROM media_attachments
WHERE id = sqlc.arg(attachment_id)::bigint;

-- name: LockMediaAttachmentForUpdate :one
SELECT id, name, file_name, mime_type, file_size, description, tags, enabled,
       version, created_by, updated_by, created_at, updated_at
FROM media_attachments
WHERE id = sqlc.arg(attachment_id)::bigint
FOR UPDATE;

-- name: LockMediaAttachmentReference :one
SELECT id
FROM media_attachments
WHERE id = sqlc.arg(attachment_id)::bigint
FOR KEY SHARE;

-- name: ReadMediaAttachment :one
SELECT attachment.id,
       attachment.name,
       attachment.file_name,
       attachment.mime_type,
       attachment.file_size,
       attachment.description,
       attachment.tags,
       attachment.enabled,
       attachment.version,
       attachment.created_by,
       attachment.updated_by,
       attachment.created_at,
       attachment.updated_at,
       attachment.checksum AS attachment_checksum,
       blob.checksum AS blob_checksum,
       blob.content
FROM media_attachments AS attachment
JOIN media_attachment_blobs AS blob ON blob.attachment_id = attachment.id
WHERE attachment.id = sqlc.arg(attachment_id)::bigint;

-- name: ListMediaAttachments :many
WITH filtered AS MATERIALIZED (
  SELECT id, name, file_name, mime_type, file_size, description, tags, enabled,
         version, created_by, updated_by, created_at, updated_at
  FROM media_attachments
  WHERE (
    sqlc.arg(search)::text = ''
    OR name ILIKE ('%' || sqlc.arg(search)::text || '%')
    OR file_name ILIKE ('%' || sqlc.arg(search)::text || '%')
    OR description ILIKE ('%' || sqlc.arg(search)::text || '%')
    OR EXISTS (
      SELECT 1
      FROM jsonb_array_elements_text(tags) AS tag(value)
      WHERE tag.value ILIKE ('%' || sqlc.arg(search)::text || '%')
    )
  )
  AND (NOT sqlc.arg(enabled_only)::boolean OR enabled = TRUE)
), total AS (
  SELECT count(*)::bigint AS value FROM filtered
), page AS (
  SELECT *
  FROM filtered
  ORDER BY updated_at DESC, id DESC
  LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint
)
SELECT total.value AS total,
       page.id,
       page.name,
       page.file_name,
       page.mime_type,
       page.file_size,
       page.description,
       page.tags,
       page.enabled,
       page.version,
       page.created_by,
       page.updated_by,
       page.created_at,
       page.updated_at
FROM total
LEFT JOIN page ON TRUE
ORDER BY page.updated_at DESC NULLS LAST, page.id DESC NULLS LAST;

-- name: UpdateMediaAttachment :one
UPDATE media_attachments
SET name = sqlc.arg(name)::text,
    description = sqlc.arg(description)::text,
    tags = sqlc.arg(tags)::jsonb,
    enabled = sqlc.arg(enabled)::boolean,
    version = sqlc.arg(version)::bigint,
    updated_by = sqlc.arg(updated_by)::bigint,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(attachment_id)::bigint
  AND version = sqlc.arg(expected_version)::bigint
RETURNING id, name, file_name, mime_type, file_size, description, tags, enabled,
          version, created_by, updated_by, created_at, updated_at;

-- name: DeleteMediaAttachment :execrows
DELETE FROM media_attachments
WHERE id = sqlc.arg(attachment_id)::bigint;

-- name: CompleteMediaAttachmentMutation :one
UPDATE media_attachment_mutation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;
