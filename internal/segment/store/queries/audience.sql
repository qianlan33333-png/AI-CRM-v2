-- name: SelectSegmentUniverse :many
SELECT id FROM customers ORDER BY id;

-- name: SelectSegmentStageEqual :many
SELECT id FROM customers WHERE stage_id = sqlc.arg(stage_id)::bigint;

-- name: SelectSegmentStageAny :many
SELECT id FROM customers WHERE stage_id = ANY(sqlc.arg(stage_ids)::bigint[]);

-- name: SelectSegmentOwnerEqual :many
SELECT id FROM customers WHERE owner_staff_id = sqlc.arg(owner_staff_id)::bigint;

-- name: SelectSegmentOwnerAny :many
SELECT id FROM customers WHERE owner_staff_id = ANY(sqlc.arg(owner_staff_ids)::bigint[]);

-- name: SelectSegmentChannelEqual :many
SELECT id FROM customers WHERE channel_id = sqlc.arg(channel_id)::bigint;

-- name: SelectSegmentChannelAny :many
SELECT id FROM customers WHERE channel_id = ANY(sqlc.arg(channel_ids)::bigint[]);

-- name: SelectSegmentTagAny :many
SELECT DISTINCT customer_id
FROM customer_tags
WHERE tag_id = ANY(sqlc.arg(tag_ids)::bigint[]);

-- name: SelectSegmentAddedBefore :many
SELECT id FROM customers WHERE added_at < sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentAddedAfter :many
SELECT id FROM customers WHERE added_at > sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentLastInteractBefore :many
SELECT id FROM customers WHERE last_interact_at < sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentLastInteractAfter :many
SELECT id FROM customers WHERE last_interact_at > sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentDeletedEqual :many
SELECT id FROM customers WHERE is_deleted = sqlc.arg(is_deleted)::boolean;
