-- name: GetOperationCycleReportReceipt :one
SELECT id, actor_scope, key_digest, payload_digest, strategy_key, run_key, accepted_revision, projection_made
FROM operation_cycle_report_receipts WHERE actor_scope = $1 AND key_digest = $2;

-- name: ReserveOperationCycleReportReceipt :one
INSERT INTO operation_cycle_report_receipts (id, actor_scope, key_digest, payload_digest, strategy_key, run_key, accepted_revision, projection_made)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (actor_scope, key_digest) DO UPDATE SET key_digest = EXCLUDED.key_digest
RETURNING id, payload_digest, strategy_key, run_key, accepted_revision, projection_made, (xmax = 0) AS inserted;

-- name: GetOperationCycleStrategy :one
SELECT strategy_key, title, status, version, definition, snapshot, updated_at
FROM operation_cycle_strategies WHERE strategy_key = $1;

-- name: ListOperationCycleStrategies :many
SELECT strategy_key, title, status, version, definition, snapshot, updated_at
FROM operation_cycle_strategies ORDER BY updated_at DESC, strategy_key DESC LIMIT $1 OFFSET $2;

-- name: CountOperationCycleStrategies :one
SELECT count(*) FROM operation_cycle_strategies;

-- name: UpsertOperationCycleStrategy :exec
INSERT INTO operation_cycle_strategies (strategy_key, title, status, version, definition, snapshot, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (strategy_key) DO UPDATE SET title = EXCLUDED.title, status = EXCLUDED.status, version = EXCLUDED.version,
  definition = EXCLUDED.definition, snapshot = EXCLUDED.snapshot, updated_at = EXCLUDED.updated_at;

-- name: GetOperationCycleRun :one
SELECT run_key, strategy_key, snapshot_revision, snapshot, received_at
FROM operation_cycle_runs WHERE run_key = $1;

-- name: ListOperationCycleRuns :many
SELECT run_key, strategy_key, snapshot_revision, snapshot, received_at
FROM operation_cycle_runs WHERE strategy_key = $1 ORDER BY received_at DESC, run_key DESC LIMIT $2 OFFSET $3;

-- name: CountOperationCycleRuns :one
SELECT count(*) FROM operation_cycle_runs WHERE strategy_key = $1;

-- name: UpsertOperationCycleRun :exec
INSERT INTO operation_cycle_runs (run_key, strategy_key, snapshot_revision, snapshot, received_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_key) DO UPDATE SET strategy_key = EXCLUDED.strategy_key, snapshot_revision = EXCLUDED.snapshot_revision,
  snapshot = EXCLUDED.snapshot, received_at = EXCLUDED.received_at;

-- name: GetOperationCycleRunner :one
SELECT runner_id, principal_id, connector_version, codex_version, compatibility_status, binding_keys, last_heartbeat_at
FROM operation_cycle_runners WHERE runner_id = $1 FOR UPDATE;

-- name: UpsertOperationCycleRunner :exec
INSERT INTO operation_cycle_runners (runner_id, principal_id, connector_version, codex_version, compatibility_status, binding_keys, last_heartbeat_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (runner_id) DO UPDATE SET principal_id = EXCLUDED.principal_id, connector_version = EXCLUDED.connector_version,
  codex_version = EXCLUDED.codex_version, compatibility_status = EXCLUDED.compatibility_status,
  binding_keys = EXCLUDED.binding_keys, last_heartbeat_at = EXCLUDED.last_heartbeat_at;

-- name: ListFreshOperationCycleRunners :many
SELECT runner_id, principal_id, connector_version, codex_version, compatibility_status, binding_keys, last_heartbeat_at
FROM operation_cycle_runners WHERE compatibility_status = 'ready' AND last_heartbeat_at >= $1 ORDER BY runner_id;

-- name: FindOperationCycleActionByKey :one
SELECT request_id, strategy_key, run_key, action_key, action_title, strategy_version, runner_id, status, parent_request_id,
  thread_id, turn_id, final_result, failure_code, created_by, created_at, updated_at, completed_at
FROM operation_cycle_action_requests WHERE idempotency_key_digest = $1;

-- name: FindActiveOperationCycleAction :one
SELECT request_id, strategy_key, run_key, action_key, action_title, strategy_version, runner_id, status, parent_request_id,
  thread_id, turn_id, final_result, failure_code, created_by, created_at, updated_at, completed_at
FROM operation_cycle_action_requests WHERE strategy_key = $1 AND status IN ('queued', 'claimed', 'thread_bound', 'turn_started') LIMIT 1;

-- name: GetOperationCycleAction :one
SELECT request_id, strategy_key, run_key, action_key, action_title, strategy_version, runner_id, status, parent_request_id,
  thread_id, turn_id, final_result, failure_code, created_by, created_at, updated_at, completed_at
FROM operation_cycle_action_requests WHERE request_id = $1;

-- name: GetOperationCycleActionForUpdate :one
SELECT request_id, strategy_key, run_key, action_key, action_title, strategy_version, runner_id, status, parent_request_id,
  thread_id, turn_id, final_result, failure_code, created_by, created_at, updated_at, completed_at, lease_token_hash, lease_expires_at
FROM operation_cycle_action_requests WHERE request_id = $1 FOR UPDATE;

-- name: GetQueuedOperationCycleActionForRunner :one
SELECT request_id, strategy_key, run_key, action_key, action_title, strategy_version, runner_id, status, parent_request_id,
  thread_id, turn_id, final_result, failure_code, created_by, created_at, updated_at, completed_at, lease_token_hash, lease_expires_at
FROM operation_cycle_action_requests WHERE runner_id = $1 AND status = 'queued' ORDER BY created_at, request_id LIMIT 1 FOR UPDATE SKIP LOCKED;

-- name: ReserveOperationCycleAction :one
INSERT INTO operation_cycle_action_requests (request_id, strategy_key, run_key, action_key, action_title, strategy_version,
  runner_id, status, parent_request_id, created_by, created_at, updated_at, idempotency_key_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'queued', NULLIF($8, ''), $9, $10, $10, $11)
ON CONFLICT (idempotency_key_digest) DO UPDATE SET idempotency_key_digest = EXCLUDED.idempotency_key_digest
RETURNING request_id, strategy_key, run_key, action_key, action_title, strategy_version, runner_id, status, parent_request_id,
  thread_id, turn_id, final_result, failure_code, created_by, created_at, updated_at, completed_at, (xmax = 0) AS inserted;

-- name: ClaimOperationCycleAction :execrows
UPDATE operation_cycle_action_requests SET status = 'claimed', lease_token_hash = $2, lease_expires_at = $3, updated_at = $4
WHERE request_id = $1 AND status = 'queued';

-- name: UpdateOperationCycleAction :execrows
UPDATE operation_cycle_action_requests SET status = $2, thread_id = NULLIF($3, ''), turn_id = NULLIF($4, ''), final_result = $5,
  failure_code = NULLIF($6, ''), completed_at = $7, updated_at = $8
WHERE request_id = $1;

-- name: GetOperationCycleActionEvent :one
SELECT payload_digest FROM operation_cycle_action_request_events WHERE request_id = $1 AND event_id = $2;

-- name: InsertOperationCycleActionEvent :exec
INSERT INTO operation_cycle_action_request_events (request_id, event_id, event_type, payload_digest, occurred_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListOperationCycleActions :many
SELECT request_id, strategy_key, run_key, action_key, action_title, strategy_version, runner_id, status, parent_request_id,
  thread_id, turn_id, final_result, failure_code, created_by, created_at, updated_at, completed_at
FROM operation_cycle_action_requests WHERE strategy_key = $1 ORDER BY created_at DESC, request_id DESC LIMIT $2 OFFSET $3;

-- name: GetOperationCycleProposal :one
SELECT proposal_id, strategy_key, base_strategy_version, status, proposal, created_by, decided_by, created_at, decided_at
FROM operation_cycle_strategy_proposals WHERE proposal_id = $1;

-- name: GetOperationCycleProposalByActorKey :one
SELECT proposal_id, strategy_key, base_strategy_version, status, proposal, created_by, decided_by, created_at, decided_at
FROM operation_cycle_strategy_proposals WHERE created_by = $1 AND idempotency_key_digest = $2;

-- name: GetOperationCycleProposalForUpdate :one
SELECT proposal_id, strategy_key, base_strategy_version, status, proposal, created_by, decided_by, created_at, decided_at
FROM operation_cycle_strategy_proposals WHERE proposal_id = $1 FOR UPDATE;

-- name: ReserveOperationCycleProposal :one
INSERT INTO operation_cycle_strategy_proposals (proposal_id, strategy_key, base_strategy_version, status, proposal, created_by, created_at, idempotency_key_digest)
VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7)
ON CONFLICT (created_by, idempotency_key_digest) DO UPDATE SET idempotency_key_digest = EXCLUDED.idempotency_key_digest
RETURNING proposal_id, strategy_key, base_strategy_version, status, proposal, created_by, decided_by, created_at, decided_at, (xmax = 0) AS inserted;

-- name: DecideOperationCycleProposal :execrows
UPDATE operation_cycle_strategy_proposals SET status = $2, decided_by = $3, decided_at = $4
WHERE proposal_id = $1 AND status = 'pending';

-- name: ListOperationCycleProposals :many
SELECT proposal_id, strategy_key, base_strategy_version, status, proposal, created_by, decided_by, created_at, decided_at
FROM operation_cycle_strategy_proposals WHERE strategy_key = $1 ORDER BY created_at DESC, proposal_id DESC LIMIT $2 OFFSET $3;

-- name: CountOperationCycleProposals :one
SELECT count(*) FROM operation_cycle_strategy_proposals WHERE strategy_key = $1;
