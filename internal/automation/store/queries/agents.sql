-- name: ListAutomationAgents :many
SELECT id, agent_name, agent_code, automation_type, status, draft_role_prompt,
       draft_task_prompt, published_role_prompt, published_task_prompt,
       draft_version, published_version, fixed_content_package_json, created_by,
       updated_by, created_at, updated_at
FROM automation_agent_configurations
WHERE status <> 'archived'
  AND (sqlc.arg(automation_type)::text = '' OR automation_type = sqlc.arg(automation_type)::text)
ORDER BY updated_at DESC, id DESC LIMIT 200;

-- name: GetAutomationAgent :one
SELECT id, agent_name, agent_code, automation_type, status, draft_role_prompt,
       draft_task_prompt, published_role_prompt, published_task_prompt,
       draft_version, published_version, fixed_content_package_json, created_by,
       updated_by, created_at, updated_at
FROM automation_agent_configurations WHERE id = sqlc.arg(id) AND status <> 'archived';

-- name: LockAutomationAgent :one
SELECT id, agent_name, agent_code, automation_type, status, draft_role_prompt,
       draft_task_prompt, published_role_prompt, published_task_prompt,
       draft_version, published_version, fixed_content_package_json, created_by,
       updated_by, created_at, updated_at
FROM automation_agent_configurations WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: CreateAutomationAgent :one
INSERT INTO automation_agent_configurations (
  agent_name, agent_code, automation_type, status, draft_role_prompt,
  draft_task_prompt, published_role_prompt, published_task_prompt,
  draft_version, published_version, fixed_content_package_json, created_by,
  updated_by, created_at, updated_at
) VALUES (
  sqlc.arg(agent_name), sqlc.arg(agent_code), sqlc.arg(automation_type), sqlc.arg(status),
  sqlc.arg(draft_role_prompt), sqlc.arg(draft_task_prompt), sqlc.arg(published_role_prompt),
  sqlc.arg(published_task_prompt), sqlc.arg(draft_version), sqlc.arg(published_version),
  sqlc.arg(fixed_content_package_json), sqlc.arg(created_by), sqlc.arg(updated_by),
  sqlc.arg(created_at), sqlc.arg(updated_at)
) RETURNING id, agent_name, agent_code, automation_type, status, draft_role_prompt,
  draft_task_prompt, published_role_prompt, published_task_prompt, draft_version,
  published_version, fixed_content_package_json, created_by, updated_by, created_at, updated_at;

-- name: UpdateAutomationAgent :one
UPDATE automation_agent_configurations SET
  agent_name = sqlc.arg(agent_name), automation_type = sqlc.arg(automation_type),
  status = sqlc.arg(status), draft_role_prompt = sqlc.arg(draft_role_prompt),
  draft_task_prompt = sqlc.arg(draft_task_prompt), published_role_prompt = sqlc.arg(published_role_prompt),
  published_task_prompt = sqlc.arg(published_task_prompt), draft_version = sqlc.arg(draft_version),
  published_version = sqlc.arg(published_version), fixed_content_package_json = sqlc.arg(fixed_content_package_json),
  updated_by = sqlc.arg(updated_by), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
RETURNING id, agent_name, agent_code, automation_type, status, draft_role_prompt,
  draft_task_prompt, published_role_prompt, published_task_prompt, draft_version,
  published_version, fixed_content_package_json, created_by, updated_by, created_at, updated_at;

-- name: ListAutomationAgentCodesByCopyPrefix :many
SELECT agent_code FROM automation_agent_configurations
WHERE agent_code LIKE sqlc.arg(copy_prefix)::text ESCAPE E'\\';

-- name: ReserveAutomationAgentReceipt :one
INSERT INTO automation_agent_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation), sqlc.arg(actor_scope), sqlc.arg(key_digest), sqlc.arg(payload_digest), sqlc.arg(created_at))
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot, created_at, completed_at;

-- name: GetAutomationAgentReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot, created_at, completed_at
FROM automation_agent_operation_receipts
WHERE operation = sqlc.arg(operation) AND actor_scope = sqlc.arg(actor_scope) AND key_digest = sqlc.arg(key_digest);

-- name: CompleteAutomationAgentReceipt :one
UPDATE automation_agent_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot), completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot, created_at, completed_at;
