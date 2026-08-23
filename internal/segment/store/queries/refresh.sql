-- name: LockSegmentDefinitionForRefresh :one
SELECT definition
FROM segments
WHERE id = sqlc.arg(segment_id)::bigint
  AND lifecycle_status = 'active'
FOR UPDATE;

-- name: DeleteSegmentMembersForRefresh :exec
DELETE FROM segment_members
WHERE segment_id = sqlc.arg(segment_id)::bigint;

-- name: InsertSegmentMembersForRefresh :exec
INSERT INTO segment_members (segment_id, customer_id, computed_at)
SELECT
  sqlc.arg(segment_id)::bigint,
  customer_id,
  sqlc.arg(computed_at)::timestamptz
FROM unnest(sqlc.arg(customer_ids)::bigint[]) AS customer_id;

-- name: CompleteSegmentRefresh :one
UPDATE segments
SET member_count = sqlc.arg(member_count)::bigint,
    refreshed_at = sqlc.arg(refreshed_at)::timestamptz,
    refresh_status = 'idle',
    updated_at = sqlc.arg(refreshed_at)::timestamptz
WHERE id = sqlc.arg(segment_id)::bigint
RETURNING member_count;
