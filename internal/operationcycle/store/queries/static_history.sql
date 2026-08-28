-- name: CreateHistoricalCycleStrategy :one
INSERT INTO operation_cycle_v1_strategy_history (source_id, source_key_digest, source_payload_digest, strategy_key, title, description, cadence, timezone, original_status, current_version, created_at, updated_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(strategy_key), sqlc.arg(title), sqlc.arg(description), sqlc.arg(cadence), sqlc.arg(timezone), sqlc.arg(original_status), sqlc.arg(current_version), sqlc.arg(created_at), sqlc.arg(updated_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, strategy_key, title, description, cadence, timezone, original_status, current_version, created_at, updated_at;

-- name: GetHistoricalCycleStrategy :one
SELECT id, source_id, source_key_digest, source_payload_digest, strategy_key, title, description, cadence, timezone, original_status, current_version, created_at, updated_at FROM operation_cycle_v1_strategy_history WHERE id=sqlc.arg(id);

-- name: ListHistoricalCycleStrategy :many
SELECT id, source_id, source_key_digest, source_payload_digest, strategy_key, title, description, cadence, timezone, original_status, current_version, created_at, updated_at FROM operation_cycle_v1_strategy_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalCycleStrategy :one
SELECT count(*)::bigint FROM operation_cycle_v1_strategy_history;

-- name: CreateHistoricalCycleVersion :one
INSERT INTO operation_cycle_v1_version_history (source_id, source_key_digest, source_payload_digest, strategy_source_id, strategy_history_id, version, label, objective, version_hash, effective_from, original_governance, confirmed_at, operation_skill_hash, created_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(strategy_source_id), sqlc.arg(strategy_history_id), sqlc.arg(version), sqlc.arg(label), sqlc.arg(objective), sqlc.arg(version_hash), sqlc.narg(effective_from), sqlc.arg(original_governance), sqlc.narg(confirmed_at), sqlc.arg(operation_skill_hash), sqlc.arg(created_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, strategy_source_id, strategy_history_id, version, label, objective, version_hash, effective_from, original_governance, confirmed_at, operation_skill_hash, created_at;

-- name: GetHistoricalCycleVersion :one
SELECT id, source_id, source_key_digest, source_payload_digest, strategy_source_id, strategy_history_id, version, label, objective, version_hash, effective_from, original_governance, confirmed_at, operation_skill_hash, created_at FROM operation_cycle_v1_version_history WHERE id=sqlc.arg(id);

-- name: ListHistoricalCycleVersion :many
SELECT id, source_id, source_key_digest, source_payload_digest, strategy_source_id, strategy_history_id, version, label, objective, version_hash, effective_from, original_governance, confirmed_at, operation_skill_hash, created_at FROM operation_cycle_v1_version_history WHERE (sqlc.narg(strategy_history_id)::bigint IS NULL OR strategy_history_id=sqlc.narg(strategy_history_id)::bigint) ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalCycleVersion :one
SELECT count(*)::bigint FROM operation_cycle_v1_version_history WHERE (sqlc.narg(strategy_history_id)::bigint IS NULL OR strategy_history_id=sqlc.narg(strategy_history_id)::bigint);

-- name: CreateHistoricalCycleDocument :one
INSERT INTO operation_cycle_v1_document_history (source_id, source_key_digest, source_payload_digest, strategy_version_source_id, version_history_id, schema_version, execution_guide_sha256, execution_guide_generated_at, copy_guide_sha256, copy_guide_generated_at, measurement_guide_sha256, measurement_guide_generated_at, document_pack_hash, created_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(strategy_version_source_id), sqlc.arg(version_history_id), sqlc.arg(schema_version), sqlc.arg(execution_guide_sha256), sqlc.narg(execution_guide_generated_at), sqlc.arg(copy_guide_sha256), sqlc.narg(copy_guide_generated_at), sqlc.arg(measurement_guide_sha256), sqlc.narg(measurement_guide_generated_at), sqlc.arg(document_pack_hash), sqlc.arg(created_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, strategy_version_source_id, version_history_id, schema_version, execution_guide_sha256, execution_guide_generated_at, copy_guide_sha256, copy_guide_generated_at, measurement_guide_sha256, measurement_guide_generated_at, document_pack_hash, created_at;

-- name: GetHistoricalCycleDocument :one
SELECT id, source_id, source_key_digest, source_payload_digest, strategy_version_source_id, version_history_id, schema_version, execution_guide_sha256, execution_guide_generated_at, copy_guide_sha256, copy_guide_generated_at, measurement_guide_sha256, measurement_guide_generated_at, document_pack_hash, created_at FROM operation_cycle_v1_document_history WHERE id=sqlc.arg(id);

-- name: ListHistoricalCycleDocument :many
SELECT id, source_id, source_key_digest, source_payload_digest, strategy_version_source_id, version_history_id, schema_version, execution_guide_sha256, execution_guide_generated_at, copy_guide_sha256, copy_guide_generated_at, measurement_guide_sha256, measurement_guide_generated_at, document_pack_hash, created_at FROM operation_cycle_v1_document_history WHERE (sqlc.narg(version_history_id)::bigint IS NULL OR version_history_id=sqlc.narg(version_history_id)::bigint) ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalCycleDocument :one
SELECT count(*)::bigint FROM operation_cycle_v1_document_history WHERE (sqlc.narg(version_history_id)::bigint IS NULL OR version_history_id=sqlc.narg(version_history_id)::bigint);
