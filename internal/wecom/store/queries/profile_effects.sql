-- name: InsertWeComContactProfileEffect :one
INSERT INTO public.wecom_contact_profile_effects (
  effect_id, legacy_receipt_id, actor_id, corp_id, staff_userid, external_userid,
  remark, description, idempotency_digest, envelope_fingerprint, state,
  accept_receipt_id, generation, updated_at
)
SELECT
  sqlc.arg(effect_id), sqlc.arg(legacy_receipt_id), sqlc.arg(actor_id), sqlc.arg(corp_id),
  sqlc.arg(staff_userid), sqlc.arg(external_userid), sqlc.arg(remark), sqlc.arg(description),
  sqlc.arg(idempotency_digest), sqlc.arg(envelope_fingerprint), 'accepted',
  sqlc.arg(accept_receipt_id), sqlc.arg(generation), sqlc.arg(updated_at)
FROM public.external_effects AS effect
JOIN public.external_effect_receipts AS receipt
  ON receipt.id = sqlc.arg(accept_receipt_id)
 AND receipt.effect_id = effect.id
 AND receipt.operation = 'accept'
 AND receipt.state = 'accepted'
WHERE effect.id = sqlc.arg(effect_id)
  AND effect.owner = 'wecom'
  AND effect.kind = 'wecom_profile_sync'
  AND effect.state = 'accepted'
  AND effect.generation = sqlc.arg(generation)
  AND effect.envelope_fingerprint = sqlc.arg(envelope_fingerprint)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetWeComContactProfileEffect :one
SELECT * FROM public.wecom_contact_profile_effects WHERE effect_id = $1;

-- name: GetWeComContactProfileEffectByIdempotency :one
SELECT * FROM public.wecom_contact_profile_effects
WHERE actor_id = $1 AND idempotency_digest = $2;

-- name: MarkWeComContactProfileEffectQueued :one
UPDATE public.wecom_contact_profile_effects AS binding
SET state = 'queued', queue_receipt_id = sqlc.arg(queue_receipt_id),
    river_job_id = sqlc.arg(river_job_id), generation = sqlc.arg(generation),
    fence = 0, lease_expires_at = NULL, updated_at = sqlc.arg(updated_at)
FROM public.external_effects AS effect, public.external_effect_receipts AS receipt
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.state = 'accepted'
  AND effect.id = binding.effect_id
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_profile_sync'
  AND effect.state = 'queued' AND effect.generation = sqlc.arg(generation)
  AND effect.river_job_id = sqlc.arg(river_job_id)
  AND receipt.id = sqlc.arg(queue_receipt_id)
  AND receipt.effect_id = binding.effect_id
  AND receipt.operation = 'queue' AND receipt.state = 'queued'
RETURNING binding.*;

-- name: RecordWeComContactProfileEffectClaim :one
UPDATE public.wecom_contact_profile_effects AS binding
SET generation = sqlc.arg(generation), fence = sqlc.arg(fence),
    lease_expires_at = sqlc.arg(lease_expires_at), updated_at = sqlc.arg(updated_at)
FROM public.external_effects AS effect
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.state = 'queued'
  AND effect.id = binding.effect_id
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_profile_sync'
  AND effect.state = 'queued' AND effect.generation = sqlc.arg(generation)
  AND effect.lease_fence = sqlc.arg(fence)
  AND effect.lease_expires_at = sqlc.arg(lease_expires_at)
RETURNING binding.*;

-- name: CompleteWeComContactProfileEffectAttempt :one
UPDATE public.wecom_contact_profile_effects AS binding
SET state = sqlc.arg(state), attempt_receipt_id = sqlc.arg(attempt_receipt_id),
    attempt_receipt_digest = sqlc.arg(attempt_receipt_digest),
    attempt_completed_at = attempt.completed_at,
    provider_call_attempted = sqlc.arg(provider_call_attempted),
    real_external_call_executed = sqlc.arg(real_external_call_executed),
    updated_at = sqlc.arg(updated_at)
FROM public.external_effects AS effect,
     public.external_effect_receipts AS receipt,
     public.external_effect_attempts AS attempt
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.state = 'queued'
  AND binding.generation = sqlc.arg(generation)
  AND binding.fence = sqlc.arg(fence)
  AND effect.id = binding.effect_id
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_profile_sync'
  AND effect.state = sqlc.arg(state)
  AND effect.generation = sqlc.arg(generation)
  AND effect.lease_fence = sqlc.arg(fence)
  AND effect.updated_at = sqlc.arg(updated_at)
  AND receipt.id = sqlc.arg(attempt_receipt_id)
  AND receipt.effect_id = binding.effect_id
  AND receipt.operation = 'complete_attempt'
  AND receipt.state = sqlc.arg(state)
  AND receipt.command_digest = sqlc.arg(attempt_receipt_digest)
  AND attempt.effect_id = binding.effect_id
  AND attempt.generation = sqlc.arg(generation)
  AND attempt.fence = sqlc.arg(fence)
  AND attempt.completion = sqlc.arg(state)
  AND attempt.receipt_digest = sqlc.arg(attempt_receipt_digest)
  AND attempt.completed_at IS NOT NULL
RETURNING binding.*;

-- name: CompleteWeComContactProfileEffectReconcile :one
UPDATE public.wecom_contact_profile_effects AS binding
SET state = 'reconciled', reconcile_receipt_id = sqlc.arg(reconcile_receipt_id),
    reconcile_receipt_digest = sqlc.arg(reconcile_receipt_digest),
    reconcile_evidence_digest = sqlc.arg(reconcile_evidence_digest),
    reconcile_resolution = sqlc.arg(reconcile_resolution),
    reconciled_at = reconciliation.recorded_at, updated_at = sqlc.arg(updated_at)
FROM public.external_effects AS effect,
     public.external_effect_receipts AS receipt,
     public.external_effect_reconciliations AS reconciliation
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.state = 'outcome_unknown'
  AND binding.generation = sqlc.arg(generation)
  AND binding.fence = sqlc.arg(fence)
  AND effect.id = binding.effect_id
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_profile_sync'
  AND effect.state = 'reconciled'
  AND effect.generation = sqlc.arg(generation)
  AND effect.lease_fence = sqlc.arg(fence)
  AND effect.updated_at = sqlc.arg(updated_at)
  AND receipt.id = sqlc.arg(reconcile_receipt_id)
  AND receipt.effect_id = binding.effect_id
  AND receipt.operation = 'reconcile'
  AND receipt.state = 'reconciled'
  AND receipt.command_digest = sqlc.arg(reconcile_receipt_digest)
  AND reconciliation.effect_id = binding.effect_id
  AND reconciliation.generation = sqlc.arg(generation)
  AND reconciliation.fence = sqlc.arg(fence)
  AND reconciliation.evidence_digest = sqlc.arg(reconcile_evidence_digest)
RETURNING binding.*;
