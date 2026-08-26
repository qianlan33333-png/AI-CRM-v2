-- name: AudiencePackageExists :one
SELECT EXISTS (
  SELECT 1
  FROM public.ai_audience_package_metadata
  WHERE segment_id = $1 AND lifecycle <> 'archived'
)::boolean;

-- name: ListAudienceSendRecords :many
SELECT dispatch.id, dispatch.state,
       COALESCE(MAX(receipt.attempt_number), 0)::int AS technical_attempt_count,
       COALESCE(CASE
         WHEN dispatch.block_reason IS NOT NULL THEN dispatch.block_reason
         WHEN dispatch.state = 'retryable_failed' THEN 'retryable_provider_failure'
         WHEN dispatch.state = 'final_failed' THEN 'final_provider_failure'
         WHEN dispatch.state = 'outcome_unknown' THEN 'outcome_unknown'
       END, '')::text AS failure_classification,
       COALESCE(bool_or(receipt.provider_result_received), FALSE)::boolean AS provider_result_received,
       (count(receipt.id) > 0)::boolean AS receipt_present,
       COALESCE(bool_or(receipt.delivery_proven), FALSE)::boolean AS delivery_proven,
       COALESCE(bool_or(receipt.business_call_dispatched), FALSE)::boolean AS business_call_dispatched,
       COALESCE(bool_or(receipt.real_external_call_executed), FALSE)::boolean AS real_external_call_executed,
       dispatch.created_at, dispatch.updated_at
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id = dispatch.handoff_id
JOIN public.cloud_campaign_touch_plans AS plan
  ON plan.id = handoff.plan_id
 AND plan.source_kind = 'ai_audience_package_members'
 AND plan.audience_package_id = sqlc.arg(package_id)
LEFT JOIN public.outbound_campaign_provider_attempt_receipts AS receipt
  ON receipt.external_effect_id = dispatch.external_effect_id
GROUP BY dispatch.id
ORDER BY dispatch.created_at DESC, dispatch.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAudienceSendRecords :one
SELECT count(*)::bigint
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id = dispatch.handoff_id
JOIN public.cloud_campaign_touch_plans AS plan
  ON plan.id = handoff.plan_id
 AND plan.source_kind = 'ai_audience_package_members'
 AND plan.audience_package_id = $1;

-- name: GetAudienceSendRecord :one
SELECT dispatch.id, dispatch.state,
       COALESCE(MAX(receipt.attempt_number), 0)::int AS technical_attempt_count,
       COALESCE(CASE
         WHEN dispatch.block_reason IS NOT NULL THEN dispatch.block_reason
         WHEN dispatch.state = 'retryable_failed' THEN 'retryable_provider_failure'
         WHEN dispatch.state = 'final_failed' THEN 'final_provider_failure'
         WHEN dispatch.state = 'outcome_unknown' THEN 'outcome_unknown'
       END, '')::text AS failure_classification,
       COALESCE(bool_or(receipt.provider_result_received), FALSE)::boolean AS provider_result_received,
       (count(receipt.id) > 0)::boolean AS receipt_present,
       COALESCE(bool_or(receipt.delivery_proven), FALSE)::boolean AS delivery_proven,
       COALESCE(bool_or(receipt.business_call_dispatched), FALSE)::boolean AS business_call_dispatched,
       COALESCE(bool_or(receipt.real_external_call_executed), FALSE)::boolean AS real_external_call_executed,
       dispatch.created_at, dispatch.updated_at
FROM public.outbound_campaign_dispatches AS dispatch
JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id = dispatch.handoff_id
JOIN public.cloud_campaign_touch_plans AS plan
  ON plan.id = handoff.plan_id
 AND plan.source_kind = 'ai_audience_package_members'
 AND plan.audience_package_id = sqlc.arg(package_id)
LEFT JOIN public.outbound_campaign_provider_attempt_receipts AS receipt
  ON receipt.external_effect_id = dispatch.external_effect_id
WHERE dispatch.id = sqlc.arg(record_id)
GROUP BY dispatch.id;
