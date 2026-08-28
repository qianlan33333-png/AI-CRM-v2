-- name: CreateHistoricalLegacyMarketingState :one
INSERT INTO segment_v1_legacy_marketing_states (source_key_digest, source_payload_digest, source_field_digest, source_id, external_userid_digest, scenario_key, marketing_phase, phase_label, phase_reason, lifecycle_status, last_batch_source_id, last_batch_status, last_batch_window_start, last_batch_window_end, last_trigger_message_at, entered_at, exited_at, exit_reason, state_payload_digest, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21) RETURNING *;

-- name: GetHistoricalLegacyMarketingState :one
SELECT * FROM segment_v1_legacy_marketing_states WHERE id = $1;

-- name: ListHistoricalLegacyMarketingState :many
SELECT * FROM segment_v1_legacy_marketing_states ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalLegacyMarketingState :one
SELECT count(*) FROM segment_v1_legacy_marketing_states;

-- name: CreateHistoricalLegacyMarketingValue :one
INSERT INTO segment_v1_legacy_marketing_values (source_key_digest, source_payload_digest, source_field_digest, source_id, external_userid_digest, scenario_key, value_segment, segment_label, score, score_breakdown_digest, state_payload_digest, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING *;

-- name: GetHistoricalLegacyMarketingValue :one
SELECT * FROM segment_v1_legacy_marketing_values WHERE id = $1;

-- name: ListHistoricalLegacyMarketingValue :many
SELECT * FROM segment_v1_legacy_marketing_values ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalLegacyMarketingValue :one
SELECT count(*) FROM segment_v1_legacy_marketing_values;
