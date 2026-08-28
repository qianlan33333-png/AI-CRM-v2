-- name: CreateHistoricalMarketingAutomationConfig :one
INSERT INTO automation_v1_marketing_config_history (source_key_digest, source_payload_digest, source_field_digest, source_id, automation_key, automation_name, target_event, channel_type, original_status, do_not_start_after_hour, created_at, updated_at, config_payload_digest)
VALUES (sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_field_digest), sqlc.arg(source_id), sqlc.arg(automation_key), sqlc.arg(automation_name), sqlc.arg(target_event), sqlc.arg(channel_type), sqlc.arg(original_status), sqlc.arg(do_not_start_after_hour), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(config_payload_digest)) RETURNING *;

-- name: GetHistoricalMarketingAutomationConfig :one
SELECT * FROM automation_v1_marketing_config_history WHERE id=$1;

-- name: ListHistoricalMarketingAutomationConfig :many
SELECT * FROM automation_v1_marketing_config_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalMarketingAutomationConfig :one
SELECT count(*) FROM automation_v1_marketing_config_history;

-- name: CreateHistoricalMarketingAutomationRule :one
INSERT INTO automation_v1_marketing_rule_history (source_key_digest, source_payload_digest, source_field_digest, source_id, config_id, config_source_id, questionnaire_source_id, question_source_id, rule_code, rule_name, answer_match_type, score_delta, segment_hint, stage_hint, original_active, sort_order, created_at, updated_at, answer_match_value_digest, rule_payload_digest)
VALUES (sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_field_digest), sqlc.arg(source_id), sqlc.arg(config_id), sqlc.arg(config_source_id), sqlc.narg(questionnaire_source_id), sqlc.narg(question_source_id), sqlc.arg(rule_code), sqlc.arg(rule_name), sqlc.arg(answer_match_type), sqlc.arg(score_delta), sqlc.arg(segment_hint), sqlc.arg(stage_hint), sqlc.arg(original_active), sqlc.arg(sort_order), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(answer_match_value_digest), sqlc.arg(rule_payload_digest)) RETURNING *;

-- name: GetHistoricalMarketingAutomationRule :one
SELECT * FROM automation_v1_marketing_rule_history WHERE id=$1;

-- name: ListHistoricalMarketingAutomationRule :many
SELECT * FROM automation_v1_marketing_rule_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalMarketingAutomationRule :one
SELECT count(*) FROM automation_v1_marketing_rule_history;
