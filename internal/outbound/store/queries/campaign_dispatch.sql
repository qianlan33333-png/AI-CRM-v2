-- name: LockOutboundCampaignHandoffForDispatch :one
SELECT id, campaign_code, plan_id, review_version, source_digest, target_digest, content_digest, accepted_at
FROM public.outbound_campaign_handoffs
WHERE campaign_code = $1 AND plan_id = $2
FOR UPDATE;

-- name: ReadOutboundCampaignHandoffForDispatch :one
SELECT id
FROM public.outbound_campaign_handoffs
WHERE campaign_code = $1 AND plan_id = $2;

-- name: ListOutboundCampaignDispatchCandidates :many
SELECT link.customer_id, step.step_index, step.content
FROM public.outbound_campaign_handoff_customer_tasks AS link
JOIN public.outbound_campaign_handoff_steps AS step ON step.handoff_id = link.handoff_id
WHERE link.handoff_id = $1
ORDER BY link.customer_id, step.step_index;

-- name: IsOutboundCampaignDispatchRecipientApproved :one
SELECT EXISTS (
  SELECT 1
  FROM public.outbound_campaign_handoffs AS handoff
  JOIN public.cloud_campaign_touch_plan_recipient_reviews AS review
    ON review.plan_id = handoff.plan_id
   AND review.campaign_code = handoff.campaign_code
  JOIN public.outbound_campaign_handoff_customer_tasks AS task
    ON task.handoff_id = handoff.id
   AND task.customer_id = review.customer_id
  WHERE handoff.id = sqlc.arg(handoff_id)
    AND review.customer_id = sqlc.arg(customer_id)
    AND review.status = 'approved'
);

-- name: InsertOutboundCampaignDispatch :one
INSERT INTO public.outbound_campaign_dispatches(handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(handoff_id,customer_id,step_index) DO UPDATE SET updated_at=public.outbound_campaign_dispatches.updated_at
RETURNING id,handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason,created_at,updated_at;

-- name: InsertOutboundCampaignDispatchWithAudienceSnapshot :one
INSERT INTO public.outbound_campaign_dispatches(
  handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason,
  sender_userid_snapshot,external_userid_snapshot
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(handoff_id,customer_id,step_index) DO UPDATE
SET updated_at=public.outbound_campaign_dispatches.updated_at
RETURNING id,handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason,
          sender_userid_snapshot,external_userid_snapshot,created_at,updated_at;

-- name: ReserveOutboundCampaignDispatchReceipt :one
INSERT INTO public.outbound_campaign_dispatch_receipts(actor_id,handoff_id,key_digest,payload_digest,result_snapshot)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(actor_id,key_digest) DO UPDATE SET actor_id=public.outbound_campaign_dispatch_receipts.actor_id
RETURNING id,actor_id,handoff_id,key_digest,payload_digest,result_snapshot,created_at;

-- name: LoadOutboundCampaignDispatchReceipt :one
SELECT id,actor_id,handoff_id,key_digest,payload_digest,result_snapshot,created_at
FROM public.outbound_campaign_dispatch_receipts WHERE actor_id=$1 AND key_digest=$2 FOR UPDATE;

-- name: ListOutboundCampaignDispatchReconciliation :many
SELECT dispatch.state, count(*)::bigint AS count
FROM public.outbound_campaign_dispatches AS dispatch
WHERE dispatch.handoff_id=$1
GROUP BY dispatch.state;

-- name: ReadOutboundCampaignDispatchEvidence :one
SELECT COALESCE(bool_or(receipt.business_call_dispatched), FALSE)::boolean AS business_call_dispatched,
       COALESCE(bool_or(receipt.real_external_call_executed), FALSE)::boolean AS real_external_call_executed
FROM public.outbound_campaign_dispatches AS dispatch
LEFT JOIN public.outbound_campaign_provider_attempt_receipts AS receipt
  ON receipt.external_effect_id = dispatch.external_effect_id
WHERE dispatch.handoff_id=$1;

-- name: ReadOutboundCampaignDispatchDeliveryEvidence :one
SELECT COALESCE(bool_or(receipt.delivery_proven), FALSE)::boolean AS delivery_proven
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_provider_attempt_receipts AS receipt ON receipt.external_effect_id=dispatch.external_effect_id
WHERE dispatch.handoff_id=$1;

-- name: LoadOutboundCampaignDispatchByEffect :one
SELECT id,handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason,created_at,updated_at
FROM public.outbound_campaign_dispatches WHERE external_effect_id=$1;

-- name: LoadOutboundCampaignDispatchProviderRequest :one
SELECT dispatch.id,dispatch.handoff_id,dispatch.customer_id,dispatch.step_index,dispatch.payload_digest,step.content,
       COALESCE(plan.source_kind,'') AS source_kind,plan.audience_package_id,
       dispatch.sender_userid_snapshot,dispatch.external_userid_snapshot
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=dispatch.handoff_id
LEFT JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
JOIN public.outbound_campaign_handoff_steps AS step
  ON step.handoff_id = dispatch.handoff_id AND step.step_index = dispatch.step_index
WHERE dispatch.payload_digest=$1;

-- name: ReadOutboundCampaignAudiencePackage :one
SELECT COALESCE(plan.source_kind,'') AS source_kind,plan.audience_package_id
FROM public.outbound_campaign_handoffs AS handoff
LEFT JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
WHERE handoff.id=$1
FOR KEY SHARE OF handoff;

-- name: UpdateOutboundCampaignDispatchState :exec
UPDATE public.outbound_campaign_dispatches SET state=$2, updated_at=now()
WHERE external_effect_id=$1 AND state <> 'blocked';

-- name: InsertOutboundCampaignProviderAttemptReceipt :one
INSERT INTO public.outbound_campaign_provider_attempt_receipts(
  external_effect_id,attempt_number,completion,provider_receipt_digest,business_call_dispatched,real_external_call_executed,
  provider_message_id,provider_code,provider_result_received,delivery_proven,reconciliation_evidence_digest
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(external_effect_id,attempt_number,completion) DO UPDATE
SET external_effect_id=public.outbound_campaign_provider_attempt_receipts.external_effect_id
RETURNING id,external_effect_id,attempt_number,completion,provider_receipt_digest,
          business_call_dispatched,real_external_call_executed,provider_message_id,provider_code,
          provider_result_received,delivery_proven,reconciliation_evidence_digest,created_at;

-- name: LoadOutboundCampaignDispatchAttemptRecovery :one
SELECT effect.state AS effect_state,effect.attempt_count,effect.generation,effect.lease_fence,effect.lease_expires_at,
       attempt.number AS attempt_number,attempt.generation AS attempt_generation,attempt.fence AS attempt_fence,
       attempt.started_at,
       receipt.completion,receipt.provider_receipt_digest,receipt.business_call_dispatched,
       receipt.real_external_call_executed,receipt.provider_message_id,receipt.provider_code,
       receipt.provider_result_received,receipt.delivery_proven,receipt.reconciliation_evidence_digest
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.external_effects AS effect ON effect.id=dispatch.external_effect_id
LEFT JOIN public.external_effect_attempts AS attempt
  ON attempt.effect_id=effect.id AND attempt.number=effect.attempt_count
LEFT JOIN public.outbound_campaign_provider_attempt_receipts AS receipt
  ON receipt.external_effect_id=effect.id AND receipt.attempt_number=effect.attempt_count
 AND receipt.completion <> 'reconciled'
WHERE dispatch.external_effect_id=$1
LIMIT 1;

-- name: ReadOutboundCampaignDispatchSourceKind :one
SELECT COALESCE(plan.source_kind,'') AS source_kind
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=dispatch.handoff_id
LEFT JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
WHERE dispatch.external_effect_id=$1
FOR KEY SHARE OF dispatch;

-- name: LoadOutboundAudienceCampaignDispatchReconciliationEvidence :one
SELECT receipt.provider_message_id,dispatch.sender_userid_snapshot,dispatch.external_userid_snapshot,receipt.provider_receipt_digest,
       receipt.business_call_dispatched,receipt.real_external_call_executed
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=dispatch.handoff_id
JOIN public.cloud_campaign_touch_plans AS plan ON plan.id=handoff.plan_id
JOIN public.outbound_campaign_provider_attempt_receipts AS receipt ON receipt.external_effect_id=dispatch.external_effect_id
WHERE dispatch.external_effect_id=$1 AND plan.source_kind='ai_audience_package_members'
  AND receipt.completion <> 'reconciled'
  AND receipt.provider_result_received AND receipt.provider_message_id IS NOT NULL
ORDER BY receipt.created_at DESC, receipt.id DESC
LIMIT 1
FOR KEY SHARE OF dispatch,receipt;
