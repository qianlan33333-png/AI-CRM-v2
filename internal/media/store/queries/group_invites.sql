-- name: ListMediaGroupInvites :many
SELECT id, name, title, description, join_url, cover_image_id, enabled,
       created_by, updated_by, version, created_at, updated_at, archived_at
FROM media_group_invites
WHERE archived_at IS NULL
  AND (NOT sqlc.arg(enabled_only)::boolean OR enabled = TRUE)
  AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)::text) in lower(name)) > 0 OR position(lower(sqlc.arg(search)::text) in lower(title)) > 0)
ORDER BY id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountMediaGroupInvites :one
SELECT count(*)
FROM media_group_invites
WHERE archived_at IS NULL
  AND (NOT sqlc.arg(enabled_only)::boolean OR enabled = TRUE)
  AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)::text) in lower(name)) > 0 OR position(lower(sqlc.arg(search)::text) in lower(title)) > 0);

-- name: GetMediaGroupInvite :one
SELECT id, name, title, description, join_url, cover_image_id, enabled,
       created_by, updated_by, version, created_at, updated_at, archived_at
FROM media_group_invites
WHERE id = sqlc.arg(id)::bigint AND archived_at IS NULL;

-- name: LockMediaGroupInvite :one
SELECT id FROM media_group_invites
WHERE id = sqlc.arg(id)::bigint AND archived_at IS NULL
FOR UPDATE;

-- name: CreateMediaGroupInvite :one
INSERT INTO media_group_invites (
  name, title, description, join_url, cover_image_id, enabled,
  created_by, updated_by, version, created_at, updated_at
) VALUES (
  sqlc.arg(name)::text, sqlc.arg(title)::text, sqlc.arg(description)::text, sqlc.arg(join_url)::text,
  sqlc.narg(cover_image_id)::bigint, sqlc.arg(enabled)::boolean, sqlc.arg(created_by)::bigint,
  sqlc.arg(updated_by)::bigint, sqlc.arg(version)::bigint, sqlc.arg(created_at)::timestamptz,
  sqlc.arg(updated_at)::timestamptz
)
RETURNING id;

-- name: UpdateMediaGroupInvite :exec
UPDATE media_group_invites
SET name = sqlc.arg(name)::text, title = sqlc.arg(title)::text, description = sqlc.arg(description)::text,
    join_url = sqlc.arg(join_url)::text, cover_image_id = sqlc.narg(cover_image_id)::bigint,
    enabled = sqlc.arg(enabled)::boolean, updated_by = sqlc.arg(updated_by)::bigint,
    version = sqlc.arg(version)::bigint, updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND archived_at IS NULL;

-- name: ArchiveMediaGroupInvite :exec
UPDATE media_group_invites
SET enabled = FALSE, updated_by = sqlc.arg(updated_by)::bigint, version = sqlc.arg(version)::bigint,
    updated_at = sqlc.arg(updated_at)::timestamptz, archived_at = sqlc.arg(archived_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND archived_at IS NULL;

-- name: MediaImageExists :one
SELECT EXISTS (SELECT 1 FROM media_images WHERE id = sqlc.arg(id)::bigint);

-- name: ReserveMediaGroupInviteReceipt :one
INSERT INTO media_group_invite_operation_receipts (
  operation, actor_scope, business_key, key_digest, payload_digest, created_at
) VALUES (
  sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(business_key)::text,
  sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (operation, actor_scope, business_key, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;

-- name: GetMediaGroupInviteReceipt :one
SELECT id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot
FROM media_group_invite_operation_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text
  AND business_key = sqlc.arg(business_key)::text AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteMediaGroupInviteReceipt :one
UPDATE media_group_invite_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb, completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, business_key, key_digest, payload_digest, state, result_snapshot;
