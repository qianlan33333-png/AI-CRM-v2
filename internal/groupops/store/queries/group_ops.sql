-- name: ListGroupOpsPlans :many
SELECT id, name, status, revision, created_by, updated_by, created_at, updated_at
FROM group_ops_plans
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGroupOpsPlans :one
SELECT count(*) FROM group_ops_plans;

-- name: GetGroupOpsPlan :one
SELECT id, name, status, revision, created_by, updated_by, created_at, updated_at
FROM group_ops_plans
WHERE id = sqlc.arg(plan_id);

-- name: LockGroupOpsPlan :one
SELECT id, name, status, revision, created_by, updated_by, created_at, updated_at
FROM group_ops_plans
WHERE id = sqlc.arg(plan_id)
FOR UPDATE;

-- name: ListGroupOpsPlanMembers :many
SELECT staff_id FROM group_ops_plan_members
WHERE plan_id = sqlc.arg(plan_id)
ORDER BY staff_id;

-- name: ListGroupOpsPlanGroupAssets :many
SELECT id, asset_reference FROM group_ops_plan_group_assets
WHERE plan_id = sqlc.arg(plan_id)
ORDER BY asset_reference, id;

-- name: ListGroupOpsPlanNodes :many
SELECT id, position, kind, message_text, delay_minutes FROM group_ops_plan_nodes
WHERE plan_id = sqlc.arg(plan_id)
ORDER BY position, id;

-- name: GetGroupOpsPlanWebhookDescriptor :one
SELECT reference FROM group_ops_plan_webhook_descriptors
WHERE plan_id = sqlc.arg(plan_id);

-- name: CreateGroupOpsPlan :one
INSERT INTO group_ops_plans (name, status, revision, created_by, updated_by, created_at, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(status), sqlc.arg(revision), sqlc.arg(created_by), sqlc.arg(updated_by), sqlc.arg(created_at), sqlc.arg(updated_at))
RETURNING id;

-- name: CreateGroupOpsPlanWebhookDescriptor :exec
INSERT INTO group_ops_plan_webhook_descriptors (plan_id, reference)
VALUES (sqlc.arg(plan_id), '');

-- name: SaveGroupOpsPlan :one
UPDATE group_ops_plans
SET name = sqlc.arg(name), status = sqlc.arg(status), revision = sqlc.arg(revision), updated_by = sqlc.arg(updated_by), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(plan_id) AND revision = sqlc.arg(previous_revision)
RETURNING id;

-- name: DeleteGroupOpsPlanMembers :exec
DELETE FROM group_ops_plan_members WHERE plan_id = sqlc.arg(plan_id);

-- name: CreateGroupOpsPlanMember :exec
INSERT INTO group_ops_plan_members (plan_id, staff_id)
VALUES (sqlc.arg(plan_id), sqlc.arg(staff_id));

-- name: DeleteMissingGroupOpsPlanGroupAssets :exec
DELETE FROM group_ops_plan_group_assets
WHERE plan_id = sqlc.arg(plan_id) AND id <> ALL(sqlc.arg(ids)::bigint[]);

-- name: UpsertGroupOpsPlanGroupAsset :exec
INSERT INTO group_ops_plan_group_assets (plan_id, asset_reference)
VALUES (sqlc.arg(plan_id), sqlc.arg(asset_reference))
ON CONFLICT (plan_id, asset_reference) DO NOTHING;

-- name: DeleteMissingGroupOpsPlanNodes :exec
DELETE FROM group_ops_plan_nodes
WHERE plan_id = sqlc.arg(plan_id) AND id <> ALL(sqlc.arg(ids)::bigint[]);

-- name: UpdateGroupOpsPlanNode :execrows
UPDATE group_ops_plan_nodes
SET position = sqlc.arg(position), kind = sqlc.arg(kind), message_text = sqlc.arg(message_text), delay_minutes = sqlc.arg(delay_minutes)
WHERE plan_id = sqlc.arg(plan_id) AND id = sqlc.arg(node_id);

-- name: CreateGroupOpsPlanNode :exec
INSERT INTO group_ops_plan_nodes (plan_id, position, kind, message_text, delay_minutes)
VALUES (sqlc.arg(plan_id), sqlc.arg(position), sqlc.arg(kind), sqlc.arg(message_text), sqlc.arg(delay_minutes));

-- name: SaveGroupOpsPlanWebhookDescriptor :execrows
UPDATE group_ops_plan_webhook_descriptors
SET reference = sqlc.arg(reference)
WHERE plan_id = sqlc.arg(plan_id);

-- name: ReserveGroupOpsOperationReceipt :one
INSERT INTO group_ops_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation), sqlc.arg(actor_scope), sqlc.arg(key_digest), sqlc.arg(payload_digest), sqlc.arg(created_at))
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetGroupOpsOperationReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM group_ops_operation_receipts
WHERE operation = sqlc.arg(operation) AND actor_scope = sqlc.arg(actor_scope) AND key_digest = sqlc.arg(key_digest)
FOR UPDATE;

-- name: CompleteGroupOpsOperationReceipt :one
UPDATE group_ops_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot), completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(id) AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;
