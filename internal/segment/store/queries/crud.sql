-- name: ReserveSegmentOperationReceipt :one
INSERT INTO segment_operation_receipts (
  operation, actor_scope, key_digest, payload_digest, created_at
) VALUES (
  sqlc.arg(operation)::text,
  sqlc.arg(actor_scope)::text,
  sqlc.arg(key_digest)::bytea,
  sqlc.arg(payload_digest)::bytea,
  sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_segment_id;

-- name: GetSegmentOperationReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_segment_id
FROM segment_operation_receipts
WHERE operation = sqlc.arg(operation)::text
  AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteSegmentOperationReceipt :one
UPDATE segment_operation_receipts
SET state = 'completed',
    result_segment_id = sqlc.arg(result_segment_id)::bigint,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint
  AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_segment_id;

-- name: ListSegments :many
SELECT id, name, definition, refresh_mode, refresh_cron, member_count,
       refreshed_at, refresh_status, created_at, updated_at, lifecycle_status
FROM segments
WHERE lifecycle_status = 'active'
  AND (sqlc.narg(after_id)::bigint IS NULL OR id > sqlc.narg(after_id)::bigint)
ORDER BY id
LIMIT sqlc.arg(row_limit)::integer;
-- name: GetSegment :one
SELECT id, name, definition, refresh_mode, refresh_cron, member_count,
       refreshed_at, refresh_status, created_at, updated_at, lifecycle_status
FROM segments
WHERE id = sqlc.arg(segment_id)::bigint;

-- name: LockSegmentForUpdate :one
SELECT id, name, definition, refresh_mode, refresh_cron, member_count,
       refreshed_at, refresh_status, created_at, updated_at, lifecycle_status
FROM segments
WHERE id = sqlc.arg(segment_id)::bigint
FOR UPDATE;

-- name: CreateSegment :one
INSERT INTO segments (
  name, definition, refresh_mode, refresh_cron, created_at, updated_at
) VALUES (
  sqlc.arg(name)::text,
  sqlc.arg(definition)::jsonb,
  sqlc.arg(refresh_mode)::text,
  sqlc.narg(refresh_cron)::text,
  sqlc.arg(created_at)::timestamptz,
  sqlc.arg(created_at)::timestamptz
)
RETURNING id, name, definition, refresh_mode, refresh_cron, member_count,
          refreshed_at, refresh_status, created_at, updated_at;

-- name: ArchiveSegment :one
UPDATE segments
SET lifecycle_status = 'archived',
    archived_at = sqlc.arg(archived_at)::timestamptz,
    archived_by = sqlc.arg(archived_by)::text,
    updated_at = sqlc.arg(archived_at)::timestamptz
WHERE id = sqlc.arg(segment_id)::bigint
  AND lifecycle_status = 'active'
RETURNING id, name, definition, refresh_mode, refresh_cron, member_count,
          refreshed_at, refresh_status, created_at, updated_at, lifecycle_status;

-- name: UpdateSegment :one
UPDATE segments
SET name = sqlc.arg(name)::text,
    definition = sqlc.arg(definition)::jsonb,
    refresh_mode = sqlc.arg(refresh_mode)::text,
    refresh_cron = sqlc.narg(refresh_cron)::text,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(segment_id)::bigint
RETURNING id, name, definition, refresh_mode, refresh_cron, member_count,
          refreshed_at, refresh_status, created_at, updated_at;

-- name: ListSegmentMemberRecords :many
SELECT c.id, c.name, c.avatar_url, c.gender, c.stage_id, c.owner_staff_id,
       c.channel_id, c.added_at, c.last_interact_at, c.is_deleted, c.extra,
       c.created_at, c.updated_at
FROM segment_members AS sm
JOIN customers AS c ON c.id = sm.customer_id
WHERE sm.segment_id = sqlc.arg(segment_id)::bigint
  AND (sqlc.narg(after_customer_id)::bigint IS NULL OR c.id > sqlc.narg(after_customer_id)::bigint)
ORDER BY c.id
LIMIT sqlc.arg(row_limit)::integer;
