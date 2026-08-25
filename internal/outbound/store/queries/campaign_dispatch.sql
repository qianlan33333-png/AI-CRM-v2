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

-- name: LoadOutboundCampaignDispatchByEffect :one
SELECT id,handoff_id,customer_id,step_index,external_effect_id,recipient_digest,payload_digest,state,block_reason,created_at,updated_at
FROM public.outbound_campaign_dispatches WHERE external_effect_id=$1;

-- name: UpdateOutboundCampaignDispatchState :exec
UPDATE public.outbound_campaign_dispatches SET state=$2, updated_at=now()
WHERE external_effect_id=$1 AND state <> 'blocked';

-- name: InsertOutboundCampaignProviderAttemptReceipt :exec
INSERT INTO public.outbound_campaign_provider_attempt_receipts(external_effect_id,attempt_number,completion,provider_receipt_digest)
VALUES($1,$2,$3,$4)
ON CONFLICT(external_effect_id,attempt_number,completion) DO NOTHING;
