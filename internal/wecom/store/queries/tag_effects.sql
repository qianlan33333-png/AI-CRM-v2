-- name: InsertWeComTagEffect :one
INSERT INTO public.wecom_tag_effects (
  effect_id, legacy_receipt_id, actor_id, corp_id, operation, sync_trigger,
  external_userid, provider_tag_ids, idempotency_digest, envelope_fingerprint,
  state, accept_receipt_id, generation, updated_at
)
SELECT
  sqlc.arg(effect_id), sqlc.arg(legacy_receipt_id), sqlc.arg(actor_id), sqlc.arg(corp_id),
  sqlc.arg(operation), sqlc.arg(sync_trigger), sqlc.arg(external_userid),
  sqlc.arg(provider_tag_ids), sqlc.arg(idempotency_digest), sqlc.arg(envelope_fingerprint),
  'accepted', sqlc.arg(accept_receipt_id), sqlc.arg(generation), sqlc.arg(updated_at)
FROM public.external_effects AS effect
JOIN public.external_effect_receipts AS receipt
  ON receipt.id = sqlc.arg(accept_receipt_id)
 AND receipt.effect_id = effect.id
 AND receipt.operation = 'accept'
 AND receipt.state = 'accepted'
WHERE effect.id = sqlc.arg(effect_id)
  AND effect.owner = 'wecom'
  AND effect.kind = 'wecom_tag_sync'
  AND effect.state = 'accepted'
  AND effect.generation = sqlc.arg(generation)
  AND effect.envelope_fingerprint = sqlc.arg(envelope_fingerprint)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetWeComTagEffect :one
SELECT * FROM public.wecom_tag_effects WHERE effect_id = $1;

-- name: GetWeComTagEffectByIdempotency :one
SELECT * FROM public.wecom_tag_effects
WHERE actor_id = $1 AND idempotency_digest = $2;

-- name: MarkWeComTagEffectQueued :one
UPDATE public.wecom_tag_effects AS binding
SET state = 'queued', queue_receipt_id = sqlc.arg(queue_receipt_id),
    river_job_id = sqlc.arg(river_job_id), generation = sqlc.arg(generation),
    fence = 0, lease_expires_at = NULL, updated_at = sqlc.arg(updated_at)
FROM public.external_effects AS effect, public.external_effect_receipts AS receipt
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.state = 'accepted'
  AND effect.id = binding.effect_id
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_tag_sync'
  AND effect.state = 'queued' AND effect.generation = sqlc.arg(generation)
  AND effect.river_job_id = sqlc.arg(river_job_id)
  AND receipt.id = sqlc.arg(queue_receipt_id)
  AND receipt.effect_id = binding.effect_id
  AND receipt.operation = 'queue' AND receipt.state = 'queued'
RETURNING binding.*;

-- name: RecordWeComTagEffectClaim :one
UPDATE public.wecom_tag_effects AS binding
SET generation = sqlc.arg(generation), fence = sqlc.arg(fence),
    lease_expires_at = sqlc.arg(lease_expires_at), updated_at = sqlc.arg(updated_at)
FROM public.external_effects AS effect
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.state = 'queued'
  AND effect.id = binding.effect_id
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_tag_sync'
  AND effect.state = 'queued' AND effect.generation = sqlc.arg(generation)
  AND effect.lease_fence = sqlc.arg(fence)
  AND effect.lease_expires_at = sqlc.arg(lease_expires_at)
RETURNING binding.*;

-- name: CompleteWeComTagEffectAttempt :one
UPDATE public.wecom_tag_effects AS binding
SET state = sqlc.arg(state), attempt_receipt_id = sqlc.arg(attempt_receipt_id),
    attempt_receipt_digest = sqlc.arg(attempt_receipt_digest),
    attempt_completed_at = attempt.completed_at, updated_at = sqlc.arg(updated_at)
FROM public.external_effects AS effect,
     public.external_effect_receipts AS receipt,
     public.external_effect_attempts AS attempt
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.state = 'queued'
  AND binding.generation = sqlc.arg(generation)
  AND binding.fence = sqlc.arg(fence)
  AND effect.id = binding.effect_id
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_tag_sync'
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

-- name: CompleteWeComTagEffectReconcile :one
UPDATE public.wecom_tag_effects AS binding
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
  AND effect.owner = 'wecom' AND effect.kind = 'wecom_tag_sync'
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

-- name: InsertWeComTagCatalogSnapshot :one
INSERT INTO public.wecom_tag_catalog_snapshots (effect_id, corp_id, receipt_digest, observed_at)
SELECT binding.effect_id, binding.corp_id, sqlc.arg(receipt_digest), sqlc.arg(observed_at)
FROM public.wecom_tag_effects AS binding
WHERE binding.effect_id = sqlc.arg(effect_id)
  AND binding.operation = 'catalog_sync'
  AND binding.state = 'executed'
  AND binding.attempt_receipt_digest = sqlc.arg(receipt_digest)
RETURNING id;

-- name: InsertWeComTagCatalogGroup :exec
INSERT INTO public.wecom_tag_catalog_groups
  (snapshot_id, provider_group_id, name, provider_order)
VALUES ($1, $2, $3, $4);

-- name: InsertWeComTagCatalogTag :exec
INSERT INTO public.wecom_tag_catalog_tags
  (snapshot_id, provider_tag_id, provider_group_id, name, provider_order)
VALUES ($1, $2, $3, $4, $5);

-- name: UpsertWeComTagGroupProjection :one
INSERT INTO public.tag_groups (wecom_group_id, name, sort_order)
VALUES (sqlc.arg(provider_group_id), sqlc.arg(name), sqlc.arg(provider_order))
ON CONFLICT (wecom_group_id) DO UPDATE
SET name = EXCLUDED.name, sort_order = EXCLUDED.sort_order
RETURNING id;

-- name: UpsertWeComTagProjection :exec
INSERT INTO public.tags (group_id, wecom_tag_id, name, sort_order)
VALUES (sqlc.arg(group_id), sqlc.arg(provider_tag_id), sqlc.arg(name), sqlc.arg(provider_order))
ON CONFLICT (wecom_tag_id) DO UPDATE
SET group_id = EXCLUDED.group_id, name = EXCLUDED.name, sort_order = EXCLUDED.sort_order;

-- name: ArchiveMissingWeComTagProjections :exec
UPDATE public.tags
SET name = 'archived:' || id::text
WHERE wecom_tag_id IS NOT NULL
  AND wecom_tag_id <> ALL(sqlc.arg(provider_tag_ids)::text[])
  AND name NOT LIKE 'archived:%';

-- name: ArchiveMissingWeComTagGroupProjections :exec
UPDATE public.tag_groups
SET name = 'archived:' || id::text
WHERE wecom_group_id IS NOT NULL
  AND wecom_group_id <> ALL(sqlc.arg(provider_group_ids)::text[])
  AND name NOT LIKE 'archived:%';
