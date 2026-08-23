-- name: ReserveOutboundCampaignHandoffReceipt :one
INSERT INTO public.outbound_campaign_handoff_receipts (
  actor_id, key_digest, payload_digest, campaign_code, plan_id, created_at
) VALUES (
  sqlc.arg(actor_id), sqlc.arg(key_digest), sqlc.arg(payload_digest), sqlc.arg(campaign_code), sqlc.arg(plan_id), sqlc.arg(created_at)
)
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING id, actor_id, key_digest, payload_digest, campaign_code, plan_id, handoff_id, event_id, state, result_snapshot;

-- name: GetOutboundCampaignHandoffReceiptForUpdate :one
SELECT id, actor_id, key_digest, payload_digest, campaign_code, plan_id, handoff_id, event_id, state, result_snapshot
FROM public.outbound_campaign_handoff_receipts
WHERE actor_id = sqlc.arg(actor_id) AND key_digest = sqlc.arg(key_digest)
FOR UPDATE;

-- name: InsertOutboundCampaignHandoff :one
INSERT INTO public.outbound_campaign_handoffs (
  campaign_code, plan_id, review_version, source_digest, target_digest, content_digest,
  target_count, step_count, status, accepted_by_actor_id, accepted_at
) VALUES (
  sqlc.arg(campaign_code), sqlc.arg(plan_id), sqlc.arg(review_version), sqlc.arg(source_digest), sqlc.arg(target_digest), sqlc.arg(content_digest),
  sqlc.arg(target_count), sqlc.arg(step_count), 'held', sqlc.arg(accepted_by_actor_id), sqlc.arg(accepted_at)
)
RETURNING id;

-- name: InsertOutboundCampaignHandoffStep :exec
INSERT INTO public.outbound_campaign_handoff_steps (handoff_id, step_index, delay_minutes, content)
VALUES (sqlc.arg(handoff_id), sqlc.arg(step_index), sqlc.arg(delay_minutes), sqlc.arg(content));

-- name: InsertOutboundCampaignHandoffCustomerLinks :exec
INSERT INTO public.outbound_campaign_handoff_customer_tasks (handoff_id, customer_id, state, eligibility)
SELECT sqlc.arg(handoff_id), customer_id, 'held', 'not_evaluated'
FROM unnest(sqlc.arg(customer_ids)::bigint[]) AS items(customer_id);

-- name: GetOutboundCampaignHandoffHeader :one
SELECT id, campaign_code, plan_id, review_version, source_digest, target_digest, content_digest,
       target_count, step_count, status, accepted_by_actor_id, accepted_at,
       local_only, provider_execution_eligible, real_external_call_executed, delivery_proven
FROM public.outbound_campaign_handoffs
WHERE campaign_code = sqlc.arg(campaign_code) AND plan_id = sqlc.arg(plan_id);

-- name: ListOutboundCampaignHandoffSteps :many
SELECT step_index, delay_minutes, content
FROM public.outbound_campaign_handoff_steps
WHERE handoff_id = sqlc.arg(handoff_id)
ORDER BY step_index ASC;

-- name: ListOutboundCampaignHandoffCustomerLinks :many
SELECT customer_id, state, eligibility, outbound_task_id
FROM public.outbound_campaign_handoff_customer_tasks
WHERE handoff_id = sqlc.arg(handoff_id)
ORDER BY customer_id ASC;

-- name: GetOutboundCampaignHandoffSummary :one
SELECT handoff.id, handoff.campaign_code, handoff.plan_id, handoff.review_version,
       handoff.status, handoff.target_count, handoff.step_count, handoff.accepted_at,
       handoff.local_only, handoff.provider_execution_eligible,
       handoff.real_external_call_executed, handoff.delivery_proven,
       count(*) FILTER (WHERE link.state = 'held')::integer AS held_count,
       count(*) FILTER (WHERE link.state = 'blocked')::integer AS blocked_count,
       count(*) FILTER (WHERE link.state = 'pending')::integer AS pending_count,
       count(*) FILTER (WHERE link.eligibility = 'not_evaluated')::integer AS not_evaluated_count,
       count(*) FILTER (WHERE link.eligibility = 'eligible')::integer AS eligible_count,
       count(*) FILTER (WHERE link.eligibility = 'inactive')::integer AS inactive_count,
       count(*) FILTER (WHERE link.eligibility = 'contact_policy')::integer AS contact_policy_count
FROM public.outbound_campaign_handoffs AS handoff
JOIN public.outbound_campaign_handoff_customer_tasks AS link ON link.handoff_id = handoff.id
WHERE handoff.campaign_code = sqlc.arg(campaign_code) AND handoff.plan_id = sqlc.arg(plan_id)
GROUP BY handoff.id;

-- name: CompleteOutboundCampaignHandoffReceipt :one
UPDATE public.outbound_campaign_handoff_receipts
SET state = 'completed', handoff_id = sqlc.arg(handoff_id), event_id = sqlc.arg(event_id),
    result_snapshot = sqlc.arg(result_snapshot), completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, actor_id, key_digest, payload_digest, campaign_code, plan_id, handoff_id, event_id, state, result_snapshot;
