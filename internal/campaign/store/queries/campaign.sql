-- The generated query package keeps every initiation read/write attached to
-- the caller's Campaign UnitOfWork transaction in postgres.go.
-- name: LockCloudCampaignDeleteReferences :exec
LOCK TABLE public.cloud_campaign_local_plans,
  public.cloud_campaign_local_commands,
  public.cloud_campaign_touch_plans,
  public.cloud_campaign_touch_plan_receipts IN SHARE MODE;

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
