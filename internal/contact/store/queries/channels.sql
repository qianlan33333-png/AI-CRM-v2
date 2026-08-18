-- Contact owns the local channel catalog; WeCom remains a provider adapter.
-- name: ListChannels :many
SELECT id, code AS channel_code, name AS channel_name, status, config AS legacy_projection, created_by, updated_by, created_at, updated_at
FROM channels
WHERE (sqlc.arg(filter_status)::text = '' OR status = sqlc.arg(filter_status)::text)
  AND (sqlc.arg(include_archived)::boolean OR status <> 'archived')
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: GetChannel :one
SELECT id, code AS channel_code, name AS channel_name, status, config AS legacy_projection, created_by, updated_by, created_at, updated_at
FROM channels WHERE id = sqlc.arg(channel_id)::bigint;

-- name: ListChannelImageReferencePackages :many
SELECT id, COALESCE(config -> 'welcome_image_library_ids', '[]'::jsonb)::text AS welcome_image_library_ids
FROM channels
ORDER BY id ASC;

-- name: CreateChannel :one
INSERT INTO channels (code, name, status, config, created_by, updated_by, created_at, updated_at)
VALUES (sqlc.arg(channel_code)::text, sqlc.arg(channel_name)::text, sqlc.arg(status)::text,
        sqlc.arg(legacy_projection)::jsonb, sqlc.arg(actor)::bigint, sqlc.arg(actor)::bigint,
        sqlc.arg(changed_at)::timestamptz, sqlc.arg(changed_at)::timestamptz)
RETURNING id, code AS channel_code, name AS channel_name, status, config AS legacy_projection, created_by, updated_by, created_at, updated_at;

-- name: UpdateChannel :one
UPDATE channels
SET name = sqlc.arg(channel_name)::text,
    status = sqlc.arg(status)::text,
    config = sqlc.arg(legacy_projection)::jsonb,
    updated_by = sqlc.arg(actor)::bigint,
    updated_at = sqlc.arg(changed_at)::timestamptz
WHERE id = sqlc.arg(channel_id)::bigint
RETURNING id, code AS channel_code, name AS channel_name, status, config AS legacy_projection, created_by, updated_by, created_at, updated_at;

-- name: ReserveChannelOperationReceipt :one
INSERT INTO channel_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea,
        sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetChannelOperationReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM channel_operation_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteChannelOperationReceipt :one
UPDATE channel_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;
