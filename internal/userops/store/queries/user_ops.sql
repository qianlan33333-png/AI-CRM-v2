-- name: ReadUserOpsLocalOverview :one
SELECT
  (SELECT count(*) FROM user_ops_dnd) AS active_dnd_count,
  (SELECT count(*) FROM user_ops_local_plans WHERE state = 'draft') AS draft_plan_count,
  (SELECT count(*) FROM user_ops_local_plans WHERE state = 'pending_review') AS pending_review_plan_count;

-- name: ReadUserOpsDND :one
SELECT customer_id, reason, version, created_at, updated_at
FROM user_ops_dnd
WHERE customer_id = sqlc.arg(customer_id);

-- name: ListUserOpsActiveDND :many
SELECT customer_id, reason, version, created_at, updated_at
FROM user_ops_dnd
WHERE customer_id = ANY(sqlc.arg(customer_ids)::bigint[])
ORDER BY customer_id;

-- name: LockUserOpsActiveDND :many
SELECT customer_id, reason, version, created_at, updated_at
FROM user_ops_dnd
WHERE customer_id = ANY(sqlc.arg(customer_ids)::bigint[])
ORDER BY customer_id
FOR KEY SHARE;

-- name: InsertUserOpsDND :one
INSERT INTO user_ops_dnd (customer_id, reason, version, created_at, updated_at)
VALUES (sqlc.arg(customer_id), sqlc.arg(reason), 1, clock_timestamp(), clock_timestamp())
ON CONFLICT (customer_id) DO NOTHING
RETURNING customer_id, reason, version, created_at, updated_at;

-- name: UpdateUserOpsDND :one
UPDATE user_ops_dnd
SET reason = sqlc.arg(reason), version = version + 1, updated_at = clock_timestamp()
WHERE customer_id = sqlc.arg(customer_id) AND version = sqlc.arg(expected_version)
RETURNING customer_id, reason, version, created_at, updated_at;

-- name: DeleteUserOpsDND :one
DELETE FROM user_ops_dnd
WHERE customer_id = sqlc.arg(customer_id) AND version = sqlc.arg(expected_version)
RETURNING customer_id;

-- name: InsertUserOpsLocalPlan :one
INSERT INTO user_ops_local_plans (state, content_snapshot, content_digest, target_digest, target_count, version, created_by, created_at, updated_at)
VALUES (sqlc.arg(state), sqlc.arg(content_snapshot), sqlc.arg(content_digest), sqlc.arg(target_digest), sqlc.arg(target_count), 1, sqlc.arg(created_by), clock_timestamp(), clock_timestamp())
RETURNING id;

-- name: InsertUserOpsLocalPlanTarget :exec
INSERT INTO user_ops_local_plan_targets (plan_id, customer_id)
VALUES (sqlc.arg(plan_id), sqlc.arg(customer_id));

-- name: InsertUserOpsSendRecord :exec
INSERT INTO user_ops_send_records (plan_id, customer_id, technical_status, created_at, updated_at)
VALUES (sqlc.arg(plan_id), sqlc.arg(customer_id), sqlc.arg(technical_status), clock_timestamp(), clock_timestamp());

-- name: ReadUserOpsLocalPlan :one
SELECT id, state, content_snapshot, content_digest, target_digest, target_count, version, created_at, updated_at
FROM user_ops_local_plans WHERE id = sqlc.arg(plan_id);

-- name: ListUserOpsSendRecords :many
SELECT id, plan_id, customer_id, technical_status, created_at, updated_at
FROM user_ops_send_records
WHERE plan_id = sqlc.arg(plan_id) AND id < sqlc.arg(before_id)
ORDER BY id DESC
LIMIT sqlc.arg(row_limit);

-- name: CountUserOpsSendRecords :one
SELECT count(*) FROM user_ops_send_records WHERE plan_id = sqlc.arg(plan_id);

-- name: ReserveUserOpsReceipt :one
INSERT INTO user_ops_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation), sqlc.arg(actor_scope), sqlc.arg(key_digest), sqlc.arg(payload_digest), clock_timestamp())
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: ReadUserOpsReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM user_ops_operation_receipts
WHERE operation = sqlc.arg(operation) AND actor_scope = sqlc.arg(actor_scope) AND key_digest = sqlc.arg(key_digest)
FOR UPDATE;

-- name: CompleteUserOpsReceipt :one
UPDATE user_ops_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot), completed_at = clock_timestamp()
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;
