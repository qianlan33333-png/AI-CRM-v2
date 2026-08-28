-- name: CreateHistoricalCampaignDefinition :one
INSERT INTO public.campaign_v1_definition_history (source_id, code, display_name, intent, anchor_mode, anchor_date, review_status, run_status, approved_at, started_at, finished_at, paused_at, paused_reason, created_at, updated_at, original_disposition, original_reason, private_digest, source_key_digest, source_payload_digest, source_field_digest, redacted_roots)
VALUES (sqlc.arg('source_id'), sqlc.arg('code'), sqlc.arg('display_name'), sqlc.arg('intent'), sqlc.arg('anchor_mode'), sqlc.arg('anchor_date'), sqlc.arg('review_status'), sqlc.arg('run_status'), sqlc.narg('approved_at'), sqlc.narg('started_at'), sqlc.narg('finished_at'), sqlc.narg('paused_at'), sqlc.arg('paused_reason'), sqlc.arg('created_at'), sqlc.arg('updated_at'), sqlc.arg('original_disposition'), sqlc.arg('original_reason'), sqlc.arg('private_digest'), sqlc.arg('source_key_digest'), sqlc.arg('source_payload_digest'), sqlc.arg('source_field_digest'), sqlc.arg('redacted_roots'))
RETURNING *;

-- name: GetHistoricalCampaignDefinition :one
SELECT * FROM public.campaign_v1_definition_history WHERE id = $1;

-- name: ListHistoricalCampaignDefinitions :many
SELECT * FROM public.campaign_v1_definition_history
ORDER BY id LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: CountHistoricalCampaignDefinitions :one
SELECT count(*) FROM public.campaign_v1_definition_history;

-- name: GetCurrentCampaignDefinitionHistoryParent :one
SELECT campaign_code,approval_status,runtime_status,version FROM public.cloud_campaigns
WHERE campaign_code=$1;

-- name: CreateHistoricalCampaignDefinitionStep :one
INSERT INTO public.campaign_v1_definition_step_history (source_id, campaign_source_id, segment_source_id, history_definition_id, current_campaign_code, source_parent_state, step_index, day_offset, send_time, timezone, content_masked, stop_on_reply, skip_recent_days, created_at, updated_at, original_disposition, original_reason, content_digest, private_digest, source_key_digest, source_payload_digest, source_field_digest, redacted_roots)
VALUES (sqlc.arg('source_id'), sqlc.arg('campaign_source_id'), sqlc.arg('segment_source_id'), sqlc.narg('history_definition_id'), sqlc.narg('current_campaign_code'), sqlc.arg('source_parent_state'), sqlc.arg('step_index'), sqlc.arg('day_offset'), sqlc.arg('send_time'), sqlc.arg('timezone'), sqlc.arg('content_masked'), sqlc.arg('stop_on_reply'), sqlc.arg('skip_recent_days'), sqlc.arg('created_at'), sqlc.arg('updated_at'), sqlc.arg('original_disposition'), sqlc.arg('original_reason'), sqlc.arg('content_digest'), sqlc.arg('private_digest'), sqlc.arg('source_key_digest'), sqlc.arg('source_payload_digest'), sqlc.arg('source_field_digest'), sqlc.arg('redacted_roots'))
RETURNING *;

-- name: GetHistoricalCampaignDefinitionStep :one
SELECT * FROM public.campaign_v1_definition_step_history WHERE id = $1;

-- name: ListHistoricalCampaignDefinitionSteps :many
SELECT * FROM public.campaign_v1_definition_step_history
WHERE (sqlc.narg('campaign_source_id')::bigint IS NULL OR campaign_source_id = sqlc.narg('campaign_source_id')::bigint)
ORDER BY id LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: CountHistoricalCampaignDefinitionSteps :one
SELECT count(*) FROM public.campaign_v1_definition_step_history
WHERE (sqlc.narg('campaign_source_id')::bigint IS NULL OR campaign_source_id = sqlc.narg('campaign_source_id')::bigint);
