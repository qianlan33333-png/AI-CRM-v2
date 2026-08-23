-- The generated query package keeps every initiation read/write attached to
-- the caller's Campaign UnitOfWork transaction in postgres.go.
-- name: LockCloudCampaignDeleteReferences :exec
LOCK TABLE public.cloud_campaign_local_plans,
  public.cloud_campaign_local_commands,
  public.cloud_campaign_touch_plans,
  public.cloud_campaign_touch_plan_receipts,
  public.cloud_campaign_touch_plan_reviews,
  public.cloud_campaign_touch_plan_review_receipts,
  public.cloud_campaign_touch_plan_handoffs IN SHARE MODE;

-- name: LockCampaignDraftForTouchPlan :one
SELECT campaign_code, version, approval_status, runtime_status
FROM public.cloud_campaigns
WHERE campaign_code = sqlc.arg(campaign_code)
FOR UPDATE;

-- name: ListCampaignStepsForTouchPlan :many
SELECT step_index, delay_minutes, content
FROM public.cloud_campaign_steps
WHERE campaign_code = sqlc.arg(campaign_code)
ORDER BY step_index ASC;

-- name: InsertCampaignTouchPlan :exec
INSERT INTO public.cloud_campaign_touch_plans (
  id, campaign_code, campaign_version, source_kind,
  customer_selection_id, customer_selection_version, segment_id,
  audience_package_id, audience_package_version, member_snapshot_watermark,
  source_digest, target_digest, content_digest, target_count, content_step_count,
  candidate_count, active_customer_count, inactive_excluded_count,
  policy_excluded_count, owner_actor_id, created_at
) VALUES (
  sqlc.arg(id), sqlc.arg(campaign_code), sqlc.arg(campaign_version), sqlc.arg(source_kind),
  sqlc.narg(customer_selection_id), sqlc.narg(customer_selection_version), sqlc.narg(segment_id),
  sqlc.narg(audience_package_id), sqlc.narg(audience_package_version), sqlc.narg(member_snapshot_watermark),
  sqlc.arg(source_digest), sqlc.arg(target_digest), sqlc.arg(content_digest), sqlc.arg(target_count), sqlc.arg(content_step_count),
  sqlc.arg(candidate_count), sqlc.arg(active_customer_count), sqlc.arg(inactive_excluded_count),
  sqlc.arg(policy_excluded_count), sqlc.arg(owner_actor_id), sqlc.arg(created_at)
);

-- name: InsertCampaignTouchPlanTargets :exec
INSERT INTO public.cloud_campaign_touch_plan_targets (plan_id, customer_id)
SELECT sqlc.arg(plan_id), customer_id
FROM unnest(sqlc.arg(customer_ids)::bigint[]) AS items(customer_id);

-- name: InsertCampaignTouchPlanStep :exec
INSERT INTO public.cloud_campaign_touch_plan_steps (plan_id, step_index, delay_minutes, content)
VALUES (sqlc.arg(plan_id), sqlc.arg(step_index), sqlc.arg(delay_minutes), sqlc.arg(content));

-- name: ReserveCampaignTouchPlanReceipt :one
INSERT INTO public.cloud_campaign_touch_plan_receipts (
  actor_id, key_digest, payload_digest, plan_id, created_at
) VALUES (
  sqlc.arg(actor_id), sqlc.arg(key_digest), sqlc.arg(payload_digest), sqlc.arg(plan_id), now()
)
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING id, actor_id, key_digest, payload_digest, plan_id, event_id, state;

-- name: GetCampaignTouchPlanReceiptForUpdate :one
SELECT id, actor_id, key_digest, payload_digest, plan_id, event_id, state
FROM public.cloud_campaign_touch_plan_receipts
WHERE actor_id = sqlc.arg(actor_id) AND key_digest = sqlc.arg(key_digest)
FOR UPDATE;

-- name: CompleteCampaignTouchPlanReceipt :one
UPDATE public.cloud_campaign_touch_plan_receipts
SET state = 'completed', event_id = sqlc.arg(event_id), completed_at = now()
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, actor_id, key_digest, payload_digest, plan_id, event_id, state;

-- name: ListCampaignTouchPlanSummaries :many
SELECT id, campaign_code, campaign_version, source_kind,
       customer_selection_id, customer_selection_version, segment_id,
       audience_package_id, audience_package_version, member_snapshot_watermark,
       source_digest, target_digest, content_digest, target_count, content_step_count,
       candidate_count, active_customer_count, inactive_excluded_count,
       policy_excluded_count, owner_actor_id, created_at,
       local_only, provider_execution_eligible, runtime_executed,
       real_external_call_executed, delivery_proven
FROM public.cloud_campaign_touch_plans
WHERE campaign_code = sqlc.arg(campaign_code)
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(after_created_at)::timestamptz, sqlc.narg(after_id)::text)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetCampaignTouchPlan :one
SELECT id, campaign_code, campaign_version, source_kind,
       customer_selection_id, customer_selection_version, segment_id,
       audience_package_id, audience_package_version, member_snapshot_watermark,
       source_digest, target_digest, content_digest, target_count, content_step_count,
       candidate_count, active_customer_count, inactive_excluded_count,
       policy_excluded_count, owner_actor_id, created_at,
       local_only, provider_execution_eligible, runtime_executed,
       real_external_call_executed, delivery_proven
FROM public.cloud_campaign_touch_plans
WHERE campaign_code = sqlc.arg(campaign_code) AND id = sqlc.arg(id);

-- name: ListCampaignTouchPlanTargets :many
SELECT customer_id
FROM public.cloud_campaign_touch_plan_targets
WHERE plan_id = sqlc.arg(plan_id)
ORDER BY customer_id ASC;

-- name: ListCampaignTouchPlanSteps :many
SELECT step_index, delay_minutes, content
FROM public.cloud_campaign_touch_plan_steps
WHERE plan_id = sqlc.arg(plan_id)
ORDER BY step_index ASC;

-- name: ReserveCampaignTouchPlanReviewReceipt :one
INSERT INTO public.cloud_campaign_touch_plan_review_receipts (
  actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, created_at
) VALUES (
  sqlc.arg(actor_id), sqlc.arg(operation), sqlc.arg(key_digest), sqlc.arg(payload_digest), sqlc.arg(plan_id), sqlc.arg(campaign_code), sqlc.arg(created_at)
)
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING id, actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, event_id, handoff_event_id, state, result_snapshot;

-- name: GetCampaignTouchPlanReviewReceiptForUpdate :one
SELECT id, actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, event_id, handoff_event_id, state, result_snapshot
FROM public.cloud_campaign_touch_plan_review_receipts
WHERE actor_id = sqlc.arg(actor_id) AND key_digest = sqlc.arg(key_digest)
FOR UPDATE;

-- name: LockCampaignTouchPlanReview :one
SELECT review.plan_id, review.campaign_code, review.status, review.version,
       review.submitted_by_actor_id, review.submitted_at,
       review.reviewed_by_actor_id, review.reviewed_at, review.confirmation_digest
FROM public.cloud_campaign_touch_plans AS plan
JOIN public.cloud_campaign_touch_plan_reviews AS review ON review.plan_id = plan.id
WHERE plan.id = sqlc.arg(plan_id) AND plan.campaign_code = sqlc.arg(campaign_code)
FOR KEY SHARE OF plan FOR UPDATE OF review;

-- name: SaveCampaignTouchPlanReview :one
UPDATE public.cloud_campaign_touch_plan_reviews
SET status = sqlc.arg(status), version = sqlc.arg(version),
    submitted_by_actor_id = sqlc.narg(submitted_by_actor_id), submitted_at = sqlc.narg(submitted_at),
    reviewed_by_actor_id = sqlc.narg(reviewed_by_actor_id), reviewed_at = sqlc.narg(reviewed_at),
    confirmation_digest = sqlc.narg(confirmation_digest)
WHERE plan_id = sqlc.arg(plan_id) AND version = sqlc.arg(expected_version)
RETURNING plan_id, campaign_code, status, version, submitted_by_actor_id, submitted_at, reviewed_by_actor_id, reviewed_at, confirmation_digest;

-- name: GetCampaignTouchPlanReview :one
SELECT plan_id, campaign_code, status, version, submitted_by_actor_id, submitted_at,
       reviewed_by_actor_id, reviewed_at, confirmation_digest
FROM public.cloud_campaign_touch_plan_reviews
WHERE plan_id = sqlc.arg(plan_id) AND campaign_code = sqlc.arg(campaign_code);

-- name: InsertCampaignTouchPlanHandoff :exec
INSERT INTO public.cloud_campaign_touch_plan_handoffs (
  plan_id, review_version, status, local_only, provider_execution_eligible,
  real_external_call_executed, delivery_proven, created_at
) VALUES (
  sqlc.arg(plan_id), sqlc.arg(review_version), sqlc.arg(status), sqlc.arg(local_only), sqlc.arg(provider_execution_eligible),
  sqlc.arg(real_external_call_executed), sqlc.arg(delivery_proven), sqlc.arg(created_at)
);

-- name: GetCampaignTouchPlanHandoff :one
SELECT plan_id, review_version, status, local_only, provider_execution_eligible,
       real_external_call_executed, delivery_proven, created_at
FROM public.cloud_campaign_touch_plan_handoffs AS handoff
WHERE plan_id = sqlc.arg(plan_id) AND EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plans WHERE id = handoff.plan_id AND campaign_code = sqlc.arg(campaign_code));

-- name: CompleteCampaignTouchPlanReviewReceipt :one
UPDATE public.cloud_campaign_touch_plan_review_receipts
SET state = 'completed', event_id = sqlc.arg(event_id), handoff_event_id = sqlc.narg(handoff_event_id), result_snapshot = sqlc.arg(result_snapshot),
    completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, event_id, handoff_event_id, state, result_snapshot;

-- name: ListCampaignTouchPlanReviewRecipients :many
SELECT plan_id, customer_id
FROM public.cloud_campaign_touch_plan_targets
WHERE plan_id = sqlc.arg(plan_id) AND customer_id > sqlc.arg(after_customer_id) AND EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plans WHERE id = sqlc.arg(plan_id) AND campaign_code = sqlc.arg(campaign_code))
ORDER BY customer_id ASC
LIMIT sqlc.arg(page_limit);

-- name: GetCampaignTouchPlanReviewRecipient :one
SELECT plan_id, customer_id
FROM public.cloud_campaign_touch_plan_targets
WHERE plan_id = sqlc.arg(plan_id) AND customer_id = sqlc.arg(customer_id) AND EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plans WHERE id = sqlc.arg(plan_id) AND campaign_code = sqlc.arg(campaign_code));
