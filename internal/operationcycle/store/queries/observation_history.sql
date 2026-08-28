-- name: CreateHistoricalCycleMetric :one
INSERT INTO operation_cycle_v1_metric_history (source_id, source_key_digest, source_payload_digest, source_field_digest, run_source_id, metric_key, label, numerator, denominator, value, unit, observation_window, data_source, data_quality, limitations_json, is_causal, value_status, last_snapshot_source_id, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
RETURNING *;

-- name: GetHistoricalCycleMetric :one
SELECT * FROM operation_cycle_v1_metric_history WHERE id=$1;

-- name: ListHistoricalCycleMetric :many
SELECT * FROM operation_cycle_v1_metric_history ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalCycleMetric :one
SELECT count(*) FROM operation_cycle_v1_metric_history;

-- name: CreateHistoricalCycleReference :one
INSERT INTO operation_cycle_v1_reference_history (source_id, source_key_digest, source_payload_digest, source_field_digest, run_source_id, reference_key, reference_type, label, source_system, reference_source_id, href, evidence_hash, data_status, last_snapshot_source_id, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
RETURNING *;

-- name: GetHistoricalCycleReference :one
SELECT * FROM operation_cycle_v1_reference_history WHERE id=$1;

-- name: ListHistoricalCycleReference :many
SELECT * FROM operation_cycle_v1_reference_history ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalCycleReference :one
SELECT count(*) FROM operation_cycle_v1_reference_history;
