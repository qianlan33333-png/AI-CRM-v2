-- name: CreateHistoricalAutomationSOP :one
INSERT INTO automation_v1_sop_history (
  source_id, source_key_digest, source_payload_digest, pool_key, day_index,
  content_masked, images_digest, original_enabled, created_at, updated_at
) VALUES (
  sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(pool_key), sqlc.arg(day_index),
  sqlc.arg(content_masked), sqlc.arg(images_digest), sqlc.arg(original_enabled), sqlc.arg(created_at), sqlc.arg(updated_at)
) RETURNING *;

-- name: GetHistoricalAutomationSOP :one
SELECT * FROM automation_v1_sop_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalAutomationSOPs :many
SELECT * FROM automation_v1_sop_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalAutomationSOPs :one
SELECT count(*)::bigint FROM automation_v1_sop_history;

-- name: CreateHistoricalAutomationConfig :one
INSERT INTO automation_v1_agent_config_history (
  source_id, source_key_digest, source_payload_digest, agent_code, display_name,
  scenario_code, original_enabled, draft_version, published_version, published_at,
  last_modified_at, last_modified_source, submitted_for_publish, submitted_at,
  created_at, updated_at, actors_digest, config_digest
) VALUES (
  sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(agent_code), sqlc.arg(display_name),
  sqlc.arg(scenario_code), sqlc.arg(original_enabled), sqlc.arg(draft_version), sqlc.arg(published_version), sqlc.arg(published_at),
  sqlc.arg(last_modified_at), sqlc.arg(last_modified_source), sqlc.arg(submitted_for_publish), sqlc.arg(submitted_at),
  sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(actors_digest), sqlc.arg(config_digest)
) RETURNING *;

-- name: GetHistoricalAutomationConfig :one
SELECT * FROM automation_v1_agent_config_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalAutomationConfigs :many
SELECT * FROM automation_v1_agent_config_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalAutomationConfigs :one
SELECT count(*)::bigint FROM automation_v1_agent_config_history;

-- name: CreateHistoricalAutomationPrompt :one
INSERT INTO automation_v1_prompt_history (
  source_id, source_key_digest, source_payload_digest, agent_code, display_name,
  original_enabled, version, created_at, updated_at, prompt_digest
) VALUES (
  sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(agent_code), sqlc.arg(display_name),
  sqlc.arg(original_enabled), sqlc.arg(version), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(prompt_digest)
) RETURNING *;

-- name: GetHistoricalAutomationPrompt :one
SELECT * FROM automation_v1_prompt_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalAutomationPrompts :many
SELECT * FROM automation_v1_prompt_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalAutomationPrompts :one
SELECT count(*)::bigint FROM automation_v1_prompt_history;

-- name: CreateHistoricalAutomationAgent :one
INSERT INTO automation_v1_agent_history (
  source_id, source_key_digest, source_payload_digest, program_source_id, workflow_source_id,
  node_source_id, task_source_id, agent_code, agent_name, original_type, original_status,
  sort_order, original_enabled, created_at, updated_at, archived_at, actors_digest, configuration_digest
) VALUES (
  sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(program_source_id), sqlc.arg(workflow_source_id),
  sqlc.arg(node_source_id), sqlc.arg(task_source_id), sqlc.arg(agent_code), sqlc.arg(agent_name), sqlc.arg(original_type), sqlc.arg(original_status),
  sqlc.arg(sort_order), sqlc.arg(original_enabled), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(archived_at), sqlc.arg(actors_digest), sqlc.arg(configuration_digest)
) RETURNING *;

-- name: GetHistoricalAutomationAgent :one
SELECT * FROM automation_v1_agent_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalAutomationAgents :many
SELECT * FROM automation_v1_agent_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalAutomationAgents :one
SELECT count(*)::bigint FROM automation_v1_agent_history;
