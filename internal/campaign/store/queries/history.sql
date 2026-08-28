-- name: CreateHistoricalCampaignSegment :one
INSERT INTO public.campaign_v1_history_segments (source_id, campaign_source_id, segment_source_id, source_parent_state, code, priority, label, created_at, source_payload_digest)
VALUES (sqlc.arg('source_id'), sqlc.arg('campaign_source_id'), sqlc.arg('segment_source_id'), sqlc.arg('source_parent_state'), sqlc.arg('code'), sqlc.arg('priority'), sqlc.arg('label'), sqlc.arg('created_at'), sqlc.arg('source_payload_digest'))
RETURNING *;

-- name: GetHistoricalCampaignSegment :one
SELECT * FROM public.campaign_v1_history_segments WHERE id = $1;

-- name: ListHistoricalCampaignSegments :many
SELECT * FROM public.campaign_v1_history_segments
WHERE (sqlc.narg('campaign_source_id')::bigint IS NULL OR campaign_source_id = sqlc.narg('campaign_source_id')::bigint)
ORDER BY id LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: CountHistoricalCampaignSegments :one
SELECT count(*) FROM public.campaign_v1_history_segments
WHERE (sqlc.narg('campaign_source_id')::bigint IS NULL OR campaign_source_id = sqlc.narg('campaign_source_id')::bigint);

-- name: CreateHistoricalCampaignMember :one
INSERT INTO public.campaign_v1_history_members (source_id, campaign_source_id, campaign_segment_source_id, segment_source_id, member_source_id, segment_history_id, customer_id, joined_at, anchor_date, current_step_index, next_due_at, original_status, stop_reason, last_step_sent_at, retry_count, created_at, updated_at, source_payload_digest)
VALUES (sqlc.arg('source_id'), sqlc.arg('campaign_source_id'), sqlc.arg('campaign_segment_source_id'), sqlc.arg('segment_source_id'), sqlc.arg('member_source_id'), sqlc.arg('segment_history_id'), sqlc.narg('customer_id'), sqlc.arg('joined_at'), sqlc.arg('anchor_date'), sqlc.arg('current_step_index'), sqlc.narg('next_due_at'), sqlc.arg('original_status'), sqlc.arg('stop_reason'), sqlc.narg('last_step_sent_at'), sqlc.arg('retry_count'), sqlc.arg('created_at'), sqlc.arg('updated_at'), sqlc.arg('source_payload_digest'))
RETURNING *;

-- name: GetHistoricalCampaignMember :one
SELECT * FROM public.campaign_v1_history_members WHERE id = $1;

-- name: ListHistoricalCampaignMembers :many
SELECT * FROM public.campaign_v1_history_members
WHERE (sqlc.narg('segment_history_id')::bigint IS NULL OR segment_history_id = sqlc.narg('segment_history_id')::bigint) AND (sqlc.narg('customer_id')::bigint IS NULL OR customer_id = sqlc.narg('customer_id')::bigint)
ORDER BY id LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: CountHistoricalCampaignMembers :one
SELECT count(*) FROM public.campaign_v1_history_members
WHERE (sqlc.narg('segment_history_id')::bigint IS NULL OR segment_history_id = sqlc.narg('segment_history_id')::bigint) AND (sqlc.narg('customer_id')::bigint IS NULL OR customer_id = sqlc.narg('customer_id')::bigint);

-- name: CreateHistoricalBroadcastPlan :one
INSERT INTO public.campaign_v1_history_broadcast_plans (source_id, source_plan_id, campaign_source_id, segment_source_id, display_name, intent, content_strategy, content_template_masked, max_recipients, candidate_count, skipped_count, requires_manual_copy, original_status, original_review_status, original_run_status, committed_at, expires_at, created_at, updated_at, runtime_digest, source_payload_digest)
VALUES (sqlc.arg('source_id'), sqlc.arg('source_plan_id'), sqlc.narg('campaign_source_id'), sqlc.narg('segment_source_id'), sqlc.arg('display_name'), sqlc.arg('intent'), sqlc.arg('content_strategy'), sqlc.arg('content_template_masked'), sqlc.arg('max_recipients'), sqlc.arg('candidate_count'), sqlc.arg('skipped_count'), sqlc.arg('requires_manual_copy'), sqlc.arg('original_status'), sqlc.arg('original_review_status'), sqlc.arg('original_run_status'), sqlc.narg('committed_at'), sqlc.narg('expires_at'), sqlc.arg('created_at'), sqlc.arg('updated_at'), sqlc.arg('runtime_digest'), sqlc.arg('source_payload_digest'))
RETURNING *;

-- name: GetHistoricalBroadcastPlan :one
SELECT * FROM public.campaign_v1_history_broadcast_plans WHERE id = $1;

-- name: ListHistoricalBroadcastPlans :many
SELECT * FROM public.campaign_v1_history_broadcast_plans
ORDER BY id LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: CountHistoricalBroadcastPlans :one
SELECT count(*) FROM public.campaign_v1_history_broadcast_plans;

-- name: CreateHistoricalBroadcastRecipient :one
INSERT INTO public.campaign_v1_history_broadcast_recipients (source_id, plan_history_id, customer_id, display_name, planned_message_count, original_approval_status, original_send_status, approved_at, rejected_at, created_at, updated_at, source_payload_digest)
VALUES (sqlc.arg('source_id'), sqlc.arg('plan_history_id'), sqlc.narg('customer_id'), sqlc.arg('display_name'), sqlc.arg('planned_message_count'), sqlc.arg('original_approval_status'), sqlc.arg('original_send_status'), sqlc.narg('approved_at'), sqlc.narg('rejected_at'), sqlc.arg('created_at'), sqlc.arg('updated_at'), sqlc.arg('source_payload_digest'))
RETURNING *;

-- name: GetHistoricalBroadcastRecipient :one
SELECT * FROM public.campaign_v1_history_broadcast_recipients WHERE id = $1;

-- name: ListHistoricalBroadcastRecipients :many
SELECT * FROM public.campaign_v1_history_broadcast_recipients
WHERE plan_history_id = sqlc.arg('plan_history_id')::bigint
ORDER BY id LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: CountHistoricalBroadcastRecipients :one
SELECT count(*) FROM public.campaign_v1_history_broadcast_recipients
WHERE plan_history_id = sqlc.arg('plan_history_id')::bigint;

-- name: CreateHistoricalBroadcastMessage :one
INSERT INTO public.campaign_v1_history_broadcast_messages (source_id, plan_history_id, recipient_history_id, customer_id, sequence_index, day_offset, original_send_time, content_masked, original_status, sent_at, created_at, updated_at, content_payload_digest, attachments_digest, source_payload_digest)
VALUES (sqlc.arg('source_id'), sqlc.arg('plan_history_id'), sqlc.arg('recipient_history_id'), sqlc.narg('customer_id'), sqlc.arg('sequence_index'), sqlc.arg('day_offset'), sqlc.arg('original_send_time'), sqlc.arg('content_masked'), sqlc.arg('original_status'), sqlc.narg('sent_at'), sqlc.arg('created_at'), sqlc.arg('updated_at'), sqlc.arg('content_payload_digest'), sqlc.arg('attachments_digest'), sqlc.arg('source_payload_digest'))
RETURNING *;

-- name: GetHistoricalBroadcastMessage :one
SELECT * FROM public.campaign_v1_history_broadcast_messages WHERE id = $1;

-- name: ListHistoricalBroadcastMessages :many
SELECT * FROM public.campaign_v1_history_broadcast_messages
WHERE recipient_history_id = sqlc.arg('recipient_history_id')::bigint
ORDER BY id LIMIT sqlc.arg('page_limit')::integer OFFSET sqlc.arg('page_offset')::integer;

-- name: CountHistoricalBroadcastMessages :one
SELECT count(*) FROM public.campaign_v1_history_broadcast_messages
WHERE recipient_history_id = sqlc.arg('recipient_history_id')::bigint;
