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

-- name: ListCampaignTouchPlanIndex :many
SELECT plan.id, plan.campaign_code, plan.campaign_version, plan.source_kind,
       plan.customer_selection_id, plan.customer_selection_version, plan.segment_id,
       plan.audience_package_id, plan.audience_package_version, plan.member_snapshot_watermark,
       plan.source_digest, plan.target_digest, plan.content_digest, plan.target_count, plan.content_step_count,
       plan.candidate_count, plan.active_customer_count, plan.inactive_excluded_count,
       plan.policy_excluded_count, plan.owner_actor_id, plan.created_at,
       plan.local_only, plan.provider_execution_eligible, plan.runtime_executed,
       plan.real_external_call_executed, plan.delivery_proven,
       review.status AS review_status, review.version AS review_version
FROM public.cloud_campaign_touch_plans AS plan
JOIN public.cloud_campaign_touch_plan_reviews AS review ON review.plan_id = plan.id AND review.campaign_code = plan.campaign_code
WHERE (sqlc.narg(review_status)::text IS NULL OR review.status = sqlc.narg(review_status)::text)
  AND (
    sqlc.narg(after_created_at)::timestamptz IS NULL
    OR (plan.created_at, plan.id) < (sqlc.narg(after_created_at)::timestamptz, sqlc.narg(after_id)::text)
  )
ORDER BY plan.created_at DESC, plan.id DESC
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

-- name: ReserveCampaignTouchPlanRecipientReviewReceipt :one
INSERT INTO public.cloud_campaign_touch_plan_recipient_review_receipts (
  actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, customer_id, created_at
) VALUES (
  sqlc.arg(actor_id), sqlc.arg(operation), sqlc.arg(key_digest), sqlc.arg(payload_digest), sqlc.arg(plan_id), sqlc.arg(campaign_code), sqlc.arg(customer_id), sqlc.arg(created_at)
)
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING id, actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, customer_id, event_id, state, result_snapshot;

-- name: GetCampaignTouchPlanRecipientReviewReceiptForUpdate :one
SELECT id, actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, customer_id, event_id, state, result_snapshot
FROM public.cloud_campaign_touch_plan_recipient_review_receipts
WHERE actor_id = sqlc.arg(actor_id) AND key_digest = sqlc.arg(key_digest)
FOR UPDATE;

-- name: LockCampaignTouchPlanRecipientReview :one
SELECT review.plan_id, review.campaign_code, review.customer_id, review.message_override,
       review.status, review.version, review.updated_by_actor_id, review.updated_at
FROM public.cloud_campaign_touch_plan_recipient_reviews AS review
JOIN public.cloud_campaign_touch_plans AS plan ON plan.id = review.plan_id
WHERE plan.campaign_code = sqlc.arg(campaign_code) AND review.plan_id = sqlc.arg(plan_id) AND review.customer_id = sqlc.arg(customer_id)
FOR UPDATE OF review;

-- name: SaveCampaignTouchPlanRecipientReview :one
INSERT INTO public.cloud_campaign_touch_plan_recipient_reviews (
  plan_id, campaign_code, customer_id, message_override, status, version, updated_by_actor_id, updated_at
) VALUES (
  sqlc.arg(plan_id), sqlc.arg(campaign_code), sqlc.arg(customer_id), sqlc.arg(message_override), sqlc.arg(status), sqlc.arg(version), sqlc.arg(updated_by_actor_id), sqlc.arg(updated_at)
)
ON CONFLICT (plan_id, customer_id) DO UPDATE SET
  message_override = EXCLUDED.message_override,
  status = EXCLUDED.status,
  version = EXCLUDED.version,
  updated_by_actor_id = EXCLUDED.updated_by_actor_id,
  updated_at = EXCLUDED.updated_at
WHERE public.cloud_campaign_touch_plan_recipient_reviews.version = sqlc.arg(expected_version)
RETURNING plan_id, campaign_code, customer_id, message_override, status, version, updated_by_actor_id, updated_at;

-- name: GetCampaignTouchPlanRecipientReview :one
SELECT review.plan_id, review.campaign_code, review.customer_id, review.message_override,
       review.status, review.version, review.updated_by_actor_id, review.updated_at
FROM public.cloud_campaign_touch_plan_recipient_reviews AS review
JOIN public.cloud_campaign_touch_plans AS plan ON plan.id = review.plan_id
WHERE plan.campaign_code = sqlc.arg(campaign_code) AND review.plan_id = sqlc.arg(plan_id) AND review.customer_id = sqlc.arg(customer_id);

-- name: CompleteCampaignTouchPlanRecipientReviewReceipt :one
UPDATE public.cloud_campaign_touch_plan_recipient_review_receipts
SET state = 'completed', event_id = sqlc.arg(event_id), result_snapshot = sqlc.arg(result_snapshot), completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, customer_id, event_id, state, result_snapshot;

-- name: ListLatestCampaignMemberStatuses :many
WITH selected AS (
  SELECT (
    SELECT plan.id
    FROM public.cloud_campaign_touch_plans AS plan
    WHERE plan.campaign_code = campaign.campaign_code
    ORDER BY plan.created_at DESC, plan.id DESC
    LIMIT 1
  ) AS plan_id
  FROM public.cloud_campaigns AS campaign
  WHERE campaign.campaign_code = sqlc.arg(campaign_code)
), projected AS (
  SELECT selected.plan_id,
         target.customer_id,
         COALESCE(review.status, 'pending_review') AS status
  FROM selected
  JOIN public.cloud_campaign_touch_plan_targets AS target
    ON target.plan_id = selected.plan_id
  LEFT JOIN public.cloud_campaign_touch_plan_recipient_reviews AS review
    ON review.plan_id = target.plan_id
   AND review.customer_id = target.customer_id
  WHERE sqlc.narg(status_filter)::text IS NULL
     OR COALESCE(review.status, 'pending_review') = sqlc.narg(status_filter)
), page AS (
  SELECT plan_id, customer_id, status
  FROM projected
  ORDER BY customer_id
  LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset)
)
SELECT COALESCE(selected.plan_id, '') AS plan_id,
       (SELECT count(*) FROM projected) AS total,
       page.customer_id,
       page.status
FROM selected
LEFT JOIN page ON TRUE
ORDER BY page.customer_id;

-- name: LockApprovedCampaignTouchPlanHandoff :one
SELECT plan.id, plan.campaign_code, plan.campaign_version, plan.source_kind,
       plan.customer_selection_id, plan.customer_selection_version, plan.segment_id,
       plan.audience_package_id, plan.audience_package_version, plan.member_snapshot_watermark,
       plan.source_digest, plan.target_digest, plan.content_digest, plan.target_count, plan.content_step_count,
       plan.candidate_count, plan.active_customer_count, plan.inactive_excluded_count,
       plan.policy_excluded_count, plan.owner_actor_id, plan.created_at AS plan_created_at,
       plan.local_only, plan.provider_execution_eligible, plan.runtime_executed,
       plan.real_external_call_executed, plan.delivery_proven,
       review.version AS review_version, review.reviewed_at,
       handoff.status AS handoff_status, handoff.created_at AS handoff_created_at,
       handoff.local_only AS handoff_local_only,
       handoff.provider_execution_eligible AS handoff_provider_execution_eligible,
       handoff.real_external_call_executed AS handoff_real_external_call_executed,
       handoff.delivery_proven AS handoff_delivery_proven
FROM public.cloud_campaign_touch_plans AS plan
JOIN public.cloud_campaign_touch_plan_reviews AS review ON review.plan_id = plan.id
JOIN public.cloud_campaign_touch_plan_handoffs AS handoff ON handoff.plan_id = plan.id
WHERE plan.id = sqlc.arg(plan_id) AND plan.campaign_code = sqlc.arg(campaign_code)
  AND review.campaign_code = plan.campaign_code AND review.status = 'approved'
  AND handoff.review_version = review.version
FOR KEY SHARE OF plan, review, handoff;

-- name: ListApprovedCampaignTouchPlanTargets :many
SELECT target.customer_id
FROM public.cloud_campaign_touch_plan_targets AS target
WHERE target.plan_id = sqlc.arg(plan_id)
  AND EXISTS (
    SELECT 1 FROM public.cloud_campaign_touch_plans AS plan
    JOIN public.cloud_campaign_touch_plan_reviews AS review ON review.plan_id = plan.id
    JOIN public.cloud_campaign_touch_plan_handoffs AS handoff ON handoff.plan_id = plan.id
    WHERE plan.id = target.plan_id AND plan.campaign_code = sqlc.arg(campaign_code)
      AND review.campaign_code = plan.campaign_code AND review.status = 'approved'
      AND handoff.review_version = review.version
  )
ORDER BY target.customer_id ASC;

-- name: ListApprovedCampaignTouchPlanSteps :many
SELECT step.step_index, step.delay_minutes, step.content
FROM public.cloud_campaign_touch_plan_steps AS step
WHERE step.plan_id = sqlc.arg(plan_id)
  AND EXISTS (
    SELECT 1 FROM public.cloud_campaign_touch_plans AS plan
    JOIN public.cloud_campaign_touch_plan_reviews AS review ON review.plan_id = plan.id
    JOIN public.cloud_campaign_touch_plan_handoffs AS handoff ON handoff.plan_id = plan.id
    WHERE plan.id = step.plan_id AND plan.campaign_code = sqlc.arg(campaign_code)
      AND review.campaign_code = plan.campaign_code AND review.status = 'approved'
      AND handoff.review_version = review.version
  )
ORDER BY step.step_index ASC;
