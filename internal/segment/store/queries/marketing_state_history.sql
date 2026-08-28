-- name: CreateHistoricalMarketingStateSnapshot :one
INSERT INTO segment_v1_marketing_state_snapshots (source_key_digest, source_payload_digest, source_field_digest, source_id, person_source_id, external_userid_digest, automation_key, main_stage, sub_stage, activated, converted, eligible_for_conversion, lifecycle_status, last_activation_at, last_conversion_marked_at, last_message_at, last_batch_source_id, last_batch_status, last_batch_window_start, last_batch_window_end, last_trigger_message_at, entered_at, exited_at, exit_reason, state_payload_digest, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27) RETURNING *;

-- name: GetHistoricalMarketingStateSnapshot :one
SELECT * FROM segment_v1_marketing_state_snapshots WHERE id = $1;

-- name: ListHistoricalMarketingStateSnapshot :many
SELECT * FROM segment_v1_marketing_state_snapshots ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalMarketingStateSnapshot :one
SELECT count(*) FROM segment_v1_marketing_state_snapshots;

-- name: CreateHistoricalMarketingStateChange :one
INSERT INTO segment_v1_marketing_state_changes (source_key_digest, source_payload_digest, source_field_digest, source_id, person_source_id, batch_source_id, external_userid_digest, automation_key, main_stage, sub_stage, activated, converted, eligible_for_conversion, lifecycle_status, last_activation_at, last_conversion_marked_at, last_message_at, exit_reason, change_reason, state_payload_digest, recorded_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22) RETURNING *;

-- name: GetHistoricalMarketingStateChange :one
SELECT * FROM segment_v1_marketing_state_changes WHERE id = $1;

-- name: ListHistoricalMarketingStateChange :many
SELECT * FROM segment_v1_marketing_state_changes ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalMarketingStateChange :one
SELECT count(*) FROM segment_v1_marketing_state_changes;

-- name: CreateHistoricalValueSegmentSnapshot :one
INSERT INTO segment_v1_value_segment_snapshots (source_key_digest, source_payload_digest, source_field_digest, source_id, external_userid_digest, segment, segment_rank, score, scoring_version, submission_source_id, matched_question_ids_digest, state_payload_digest, computed_reason, evaluated_at, computed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING *;

-- name: GetHistoricalValueSegmentSnapshot :one
SELECT * FROM segment_v1_value_segment_snapshots WHERE id = $1;

-- name: ListHistoricalValueSegmentSnapshot :many
SELECT * FROM segment_v1_value_segment_snapshots ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalValueSegmentSnapshot :one
SELECT count(*) FROM segment_v1_value_segment_snapshots;

-- name: CreateHistoricalValueSegmentChange :one
INSERT INTO segment_v1_value_segment_changes (source_key_digest, source_payload_digest, source_field_digest, source_id, external_userid_digest, segment, segment_rank, score, scoring_version, submission_source_id, matched_question_ids_digest, state_payload_digest, change_reason, evaluated_at, recorded_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *;

-- name: GetHistoricalValueSegmentChange :one
SELECT * FROM segment_v1_value_segment_changes WHERE id = $1;

-- name: ListHistoricalValueSegmentChange :many
SELECT * FROM segment_v1_value_segment_changes ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalValueSegmentChange :one
SELECT count(*) FROM segment_v1_value_segment_changes;
