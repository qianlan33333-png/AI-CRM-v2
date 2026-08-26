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

-- name: InsertOutboundCampaignProviderAttemptReceipt :exec
INSERT INTO public.outbound_campaign_provider_attempt_receipts(
  external_effect_id,attempt_number,completion,provider_receipt_digest,business_call_dispatched,real_external_call_executed,
  provider_message_id,provider_code,provider_result_received,delivery_proven,reconciliation_evidence_digest
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(external_effect_id,attempt_number,completion) DO NOTHING;

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
  AND receipt.provider_result_received AND receipt.provider_message_id IS NOT NULL
ORDER BY receipt.created_at DESC, receipt.id DESC
LIMIT 1
FOR KEY SHARE OF dispatch,receipt;
