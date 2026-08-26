-- name: ListGroupOpsPlans :many
SELECT p.id, p.name, p.status, p.revision,
       count(e.id) FILTER (WHERE effect.state IN ('accepted', 'queued', 'attempted'))::bigint AS queue_count,
       p.created_by, p.updated_by, p.created_at, p.updated_at
FROM group_ops_plans p
LEFT JOIN group_ops_executions e ON e.plan_id = p.id
LEFT JOIN external_effects effect ON effect.id = e.external_effect_id
GROUP BY p.id
ORDER BY p.updated_at DESC, p.id DESC
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
SELECT id, position, kind, message_text, delay_minutes, material_reference FROM group_ops_plan_nodes
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
SET position = sqlc.arg(position), kind = sqlc.arg(kind), message_text = sqlc.arg(message_text), delay_minutes = sqlc.arg(delay_minutes), material_reference = sqlc.arg(material_reference)
WHERE plan_id = sqlc.arg(plan_id) AND id = sqlc.arg(node_id);

-- name: CreateGroupOpsPlanNode :exec
INSERT INTO group_ops_plan_nodes (plan_id, position, kind, message_text, delay_minutes, material_reference)
VALUES (sqlc.arg(plan_id), sqlc.arg(position), sqlc.arg(kind), sqlc.arg(message_text), sqlc.arg(delay_minutes), sqlc.arg(material_reference));

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

-- name: ListGroupOpsExecutionKeys :many
SELECT e.node_id, e.target_reference
FROM group_ops_executions e
JOIN group_ops_runs r ON r.id = e.run_id
WHERE e.plan_id = sqlc.arg(plan_id) AND e.plan_revision = sqlc.arg(plan_revision) AND r.trigger_kind = 'run_due'
ORDER BY node_id, target_reference;

-- name: ReserveGroupOpsRun :one
WITH inserted AS (
  INSERT INTO group_ops_runs (plan_id, trigger_kind, source_key_digest, plan_revision, scheduled_for, accepted_at, accepted_by)
  VALUES (sqlc.arg(plan_id), sqlc.arg(trigger_kind), sqlc.arg(source_key_digest), sqlc.arg(plan_revision), sqlc.arg(scheduled_for), sqlc.arg(accepted_at), sqlc.arg(accepted_by))
  ON CONFLICT (plan_id, trigger_kind, source_key_digest) DO NOTHING
  RETURNING id, plan_id, trigger_kind, source_key_digest, plan_revision, scheduled_for, accepted_at, accepted_by
)
SELECT * FROM inserted
UNION ALL
SELECT id, plan_id, trigger_kind, source_key_digest, plan_revision, scheduled_for, accepted_at, accepted_by
FROM group_ops_runs
WHERE plan_id = sqlc.arg(plan_id) AND trigger_kind = sqlc.arg(trigger_kind) AND source_key_digest = sqlc.arg(source_key_digest)
  AND NOT EXISTS (SELECT 1 FROM inserted)
LIMIT 1;

-- name: GetGroupOpsRun :one
SELECT id, plan_id, trigger_kind, source_key_digest, plan_revision, scheduled_for, accepted_at, accepted_by
FROM group_ops_runs WHERE id = sqlc.arg(run_id);

-- name: InsertGroupOpsExecution :one
WITH inserted AS (
  INSERT INTO group_ops_executions (
    run_id, plan_id, node_id, plan_revision, node_position, target_reference, target_digest,
    content_snapshot, content_digest, material_snapshot, material_digest, execution_key_digest, external_effect_id, sender_userid_snapshot,
    created_at, updated_at
  ) VALUES (
    sqlc.arg(run_id), sqlc.arg(plan_id), sqlc.arg(node_id), sqlc.arg(plan_revision), sqlc.arg(node_position),
    sqlc.arg(target_reference), sqlc.arg(target_digest), sqlc.arg(content_snapshot), sqlc.arg(content_digest),
    sqlc.arg(material_snapshot), sqlc.arg(material_digest), sqlc.arg(execution_key_digest), sqlc.arg(external_effect_id), sqlc.narg(sender_userid_snapshot), sqlc.arg(created_at), sqlc.arg(created_at)
  )
  ON CONFLICT (execution_key_digest) DO NOTHING
  RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT * FROM group_ops_executions
WHERE execution_key_digest = sqlc.arg(execution_key_digest)
  AND NOT EXISTS (SELECT 1 FROM inserted)
LIMIT 1;

-- name: ListGroupOpsRunExecutions :many
SELECT * FROM group_ops_executions
WHERE run_id = sqlc.arg(run_id)
ORDER BY node_position, target_reference, id;

-- name: ListGroupOpsExecutions :many
SELECT * FROM group_ops_executions
WHERE plan_id = sqlc.arg(plan_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGroupOpsExecutions :one
SELECT count(*) FROM group_ops_executions WHERE plan_id = sqlc.arg(plan_id);

-- name: GetGroupOpsExecution :one
SELECT * FROM group_ops_executions WHERE id = sqlc.arg(execution_id) FOR UPDATE;

-- name: GetGroupOpsExecutionByExternalEffectID :one
SELECT * FROM group_ops_executions
WHERE external_effect_id = sqlc.arg(external_effect_id)
FOR UPDATE;

-- name: LockGroupOpsDirectoryGroupOwner :one
SELECT owner_staff_id FROM group_ops_directory_groups
WHERE chat_reference = sqlc.arg(chat_reference)
FOR SHARE;

-- name: GetGroupOpsExternalEffect :one
SELECT id, owner, kind, state, generation, lease_fence, lease_expires_at, attempt_count
FROM external_effects
WHERE id = sqlc.arg(external_effect_id);

-- name: GetGroupOpsExternalEffectAttempt :one
SELECT effect_id, number, generation, fence, completion, receipt_digest, started_at, completed_at
FROM external_effect_attempts
WHERE effect_id = sqlc.arg(external_effect_id)
ORDER BY number DESC
LIMIT 1;

-- name: GetGroupOpsWeComGroupMessageReceipt :one
SELECT * FROM group_ops_wecom_group_message_receipts
WHERE external_effect_id = sqlc.arg(external_effect_id);

-- name: InsertGroupOpsWeComGroupMessageReceipt :one
INSERT INTO group_ops_wecom_group_message_receipts (
  external_effect_id, execution_id, msgid, sender_userid, chat_id, userid,
  task_evidence_digest, created_at, updated_at
) VALUES (
  sqlc.arg(external_effect_id), sqlc.arg(execution_id), sqlc.arg(msgid),
  sqlc.arg(sender_userid), sqlc.arg(chat_id), sqlc.arg(userid),
  sqlc.arg(task_evidence_digest), sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (external_effect_id) DO NOTHING
RETURNING *;

-- name: RecordGroupOpsWeComGroupMessageDelivery :one
UPDATE group_ops_wecom_group_message_receipts
SET send_status = 1, delivery_evidence_digest = sqlc.arg(delivery_evidence_digest), updated_at = sqlc.arg(updated_at)
WHERE external_effect_id = sqlc.arg(external_effect_id)
  AND msgid = sqlc.arg(msgid)
  AND sender_userid = sqlc.arg(sender_userid)
  AND chat_id = sqlc.arg(chat_id)
  AND userid = sqlc.arg(userid)
  AND task_evidence_digest = sqlc.arg(task_evidence_digest)
RETURNING *;

-- name: RecordGroupOpsExecutionOutcome :one
UPDATE group_ops_executions
SET state = sqlc.arg(state), provider_accepted = sqlc.arg(provider_accepted), delivery_proven = sqlc.arg(delivery_proven),
    provider_receipt_digest = sqlc.arg(provider_receipt_digest), attempt_count = sqlc.arg(attempt_count), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(execution_id)
RETURNING *;

-- name: ReconcileGroupOpsExecution :one
UPDATE group_ops_executions
SET state = 'reconciled', provider_accepted = sqlc.arg(provider_accepted), delivery_proven = sqlc.arg(delivery_proven),
    reconciliation_evidence_digest = sqlc.arg(reconciliation_evidence_digest), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(execution_id) AND state = 'outcome_unknown'
RETURNING *;

-- name: FindGroupOpsPlanByWebhookReference :one
SELECT p.id
FROM group_ops_plans p
JOIN group_ops_plan_webhook_descriptors d ON d.plan_id = p.id
WHERE d.reference = sqlc.arg(reference) AND p.status = 'active';

-- name: ListGroupOpsDirectoryGroups :many
SELECT chat_reference, owner_staff_id, display_name, member_count, source_digest, refreshed_at
FROM group_ops_directory_groups
WHERE sqlc.arg(owner_staff_id)::bigint = 0 OR owner_staff_id = sqlc.arg(owner_staff_id)
ORDER BY display_name, chat_reference
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGroupOpsDirectoryGroups :one
SELECT count(*) FROM group_ops_directory_groups
WHERE sqlc.arg(owner_staff_id)::bigint = 0 OR owner_staff_id = sqlc.arg(owner_staff_id);

-- name: DeleteGroupOpsDirectoryGroups :exec
DELETE FROM group_ops_directory_groups WHERE owner_staff_id = sqlc.arg(owner_staff_id);

-- name: InsertGroupOpsDirectoryGroup :exec
INSERT INTO group_ops_directory_groups (chat_reference, owner_staff_id, display_name, member_count, source_digest, refreshed_at)
VALUES (sqlc.arg(chat_reference), sqlc.arg(owner_staff_id), sqlc.arg(display_name), sqlc.arg(member_count), sqlc.arg(source_digest), sqlc.arg(refreshed_at));

-- name: ReserveGroupOpsDirectoryRefresh :one
INSERT INTO group_ops_directory_refresh_receipts (
  refresh_kind, actor_id, owner_staff_id, key_digest, snapshot_digest, item_count, provider_read_executed, refreshed_at
) VALUES (
  sqlc.arg(refresh_kind), sqlc.arg(actor_id), sqlc.arg(owner_staff_id), sqlc.arg(key_digest), sqlc.arg(snapshot_digest),
  sqlc.arg(item_count), sqlc.arg(provider_read_executed), sqlc.arg(refreshed_at)
)
ON CONFLICT (refresh_kind, actor_id, key_digest) DO NOTHING
RETURNING *;

-- name: GetGroupOpsDirectoryRefresh :one
SELECT * FROM group_ops_directory_refresh_receipts
WHERE refresh_kind = sqlc.arg(refresh_kind) AND actor_id = sqlc.arg(actor_id) AND key_digest = sqlc.arg(key_digest);
