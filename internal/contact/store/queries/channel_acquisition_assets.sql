-- name: LockChannelAcquisitionSnapshot :one
SELECT id AS channel_id, GREATEST(1::bigint, floor(extract(epoch FROM updated_at) * 1000000)::bigint) AS channel_revision,
       code AS channel_code, name AS channel_name, status,
       COALESCE(config ->> 'scene_value', '')::text AS scene_value,
       COALESCE(ARRAY(SELECT jsonb_array_elements_text(COALESCE(config -> 'assignee_wecom_userids', '[]'::jsonb)) ORDER BY 1), '{}')::text[] AS assignee_wecom_userids
FROM channels WHERE id = sqlc.arg(channel_id)::bigint FOR UPDATE;

-- name: ReserveChannelAcquisitionAssetActorReceipt :one
INSERT INTO channel_acquisition_asset_actor_receipts(operation, actor_id, key_digest, payload_digest, state, created_at)
VALUES (sqlc.arg(operation)::text, sqlc.arg(actor_id)::bigint, sqlc.arg(key_digest)::text, sqlc.arg(payload_digest)::text, 'in_progress', sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_id, key_digest) DO NOTHING
RETURNING id, operation, actor_id, key_digest, payload_digest, state, result_effect_id, replacement_effect_id, created_at, completed_at;

-- name: LockChannelAcquisitionAssetActorReceipt :one
SELECT id, operation, actor_id, key_digest, payload_digest, state, result_effect_id, replacement_effect_id, created_at, completed_at
FROM channel_acquisition_asset_actor_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_id = sqlc.arg(actor_id)::bigint AND key_digest = sqlc.arg(key_digest)::text FOR UPDATE;

-- name: CompleteChannelAcquisitionAssetActorReceipt :one
UPDATE channel_acquisition_asset_actor_receipts
SET state = 'completed', result_effect_id = sqlc.arg(result_effect_id)::bigint, replacement_effect_id = sqlc.narg(replacement_effect_id)::bigint, completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_id, key_digest, payload_digest, state, result_effect_id, replacement_effect_id, created_at, completed_at;

-- name: NextChannelAcquisitionAssetVersion :one
SELECT (COALESCE((
  SELECT asset_version FROM channel_acquisition_asset_bindings
  WHERE channel_id = sqlc.arg(channel_id)::bigint AND asset_kind = sqlc.arg(asset_kind)::text
  ORDER BY asset_version DESC LIMIT 1
), 0::bigint) + 1)::bigint AS asset_version;

-- name: InsertChannelAcquisitionAssetBinding :one
INSERT INTO channel_acquisition_asset_bindings (
  effect_id, channel_id, asset_kind, asset_version, supersedes_version, channel_revision, channel_code, channel_name, channel_status, scene_value, assignee_wecom_userids,
  snapshot_digest, idempotency_digest, envelope_fingerprint, corp_id, correlation_key, state, accept_receipt_id, accept_receipt_digest, generation, created_at, updated_at
) VALUES (
  sqlc.arg(effect_id)::bigint, sqlc.arg(channel_id)::bigint, sqlc.arg(asset_kind)::text, sqlc.arg(asset_version)::bigint, sqlc.arg(supersedes_version)::bigint, sqlc.arg(channel_revision)::bigint, sqlc.arg(channel_code)::text, sqlc.arg(channel_name)::text, 'active', sqlc.arg(scene_value)::text, sqlc.arg(assignee_wecom_userids)::text[],
  sqlc.arg(snapshot_digest)::text, sqlc.arg(idempotency_digest)::text, sqlc.arg(envelope_fingerprint)::text, sqlc.arg(corp_id)::text, sqlc.arg(correlation_key)::text, 'accepted', sqlc.arg(accept_receipt_id)::bigint, sqlc.arg(accept_receipt_digest)::text, sqlc.arg(generation)::bigint, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
)
RETURNING *;

-- name: ResolveChannelAcquisitionAssetCorrelation :many
SELECT effect_id, channel_id, asset_kind, asset_version
FROM channel_acquisition_asset_bindings AS binding
WHERE corp_id = sqlc.arg(corp_id)::text AND correlation_key = sqlc.arg(correlation_key)::text
  AND (
    (state = 'executed' AND EXISTS (
      SELECT 1 FROM channel_acquisition_asset_attempt_facts AS attempt
      WHERE attempt.effect_id = binding.effect_id AND attempt.state = 'executed'
        AND attempt.completed_at <= sqlc.arg(occurred_at)::timestamptz
    )) OR
    (state = 'reconciled' AND reconcile_resolution = 'provider_applied' AND EXISTS (
      SELECT 1 FROM channel_acquisition_asset_reconciliation_facts AS reconciliation
      WHERE reconciliation.effect_id = binding.effect_id AND reconciliation.resolution = 'provider_applied'
        AND reconciliation.reconciled_at <= sqlc.arg(occurred_at)::timestamptz
    ))
  )
ORDER BY effect_id
LIMIT 2;

-- name: UpsertCurrentChannelAcquisitionAsset :exec
INSERT INTO channel_acquisition_asset_current(channel_id, asset_kind, effect_id, asset_version, updated_at)
VALUES (sqlc.arg(channel_id)::bigint, sqlc.arg(asset_kind)::text, sqlc.arg(effect_id)::bigint, sqlc.arg(asset_version)::bigint, sqlc.arg(updated_at)::timestamptz)
ON CONFLICT (channel_id, asset_kind) DO UPDATE SET effect_id = EXCLUDED.effect_id, asset_version = EXCLUDED.asset_version, updated_at = EXCLUDED.updated_at;

-- name: MarkChannelAcquisitionAssetQueued :one
UPDATE channel_acquisition_asset_bindings
SET state = 'queued', queue_receipt_id = sqlc.arg(queue_receipt_id)::bigint, queue_receipt_digest = sqlc.arg(queue_receipt_digest)::text, river_job_id = sqlc.arg(river_job_id)::bigint, generation = sqlc.arg(generation)::bigint, updated_at = sqlc.arg(updated_at)::timestamptz
WHERE effect_id = sqlc.arg(effect_id)::bigint AND state = 'accepted'
RETURNING *;

-- name: LockChannelAcquisitionAssetBinding :one
SELECT * FROM channel_acquisition_asset_bindings WHERE effect_id = sqlc.arg(effect_id)::bigint FOR UPDATE;

-- name: LockChannelAcquisitionAssetBindingForChannel :one
SELECT * FROM channel_acquisition_asset_bindings
WHERE channel_id = sqlc.arg(channel_id)::bigint AND effect_id = sqlc.arg(effect_id)::bigint
FOR UPDATE;

-- name: ReadChannelAcquisitionAssetChannel :one
SELECT EXISTS(SELECT 1 FROM channels WHERE id = sqlc.arg(channel_id)::bigint AND status <> 'archived') AS exists;

-- name: GetChannelAcquisitionAsset :one
SELECT binding.effect_id, binding.channel_id, binding.asset_kind, binding.asset_version, binding.supersedes_version, binding.state,
       binding.accept_receipt_id, binding.queue_receipt_id, binding.attempt_receipt_digest, binding.reconcile_receipt_id,
       binding.created_at, binding.updated_at, binding.reconciled_at, COALESCE(result.asset_url, '')::text AS asset_url
FROM channel_acquisition_asset_bindings AS binding
LEFT JOIN channel_acquisition_asset_provider_results AS result USING (effect_id)
WHERE binding.channel_id = sqlc.arg(channel_id)::bigint AND binding.effect_id = sqlc.arg(effect_id)::bigint;

-- name: ListChannelAcquisitionAssets :many
SELECT binding.effect_id, binding.channel_id, binding.asset_kind, binding.asset_version, binding.supersedes_version, binding.state,
       binding.accept_receipt_id, binding.queue_receipt_id, binding.attempt_receipt_digest, binding.reconcile_receipt_id,
       binding.created_at, binding.updated_at, binding.reconciled_at, COALESCE(result.asset_url, '')::text AS asset_url
FROM channel_acquisition_asset_bindings AS binding
LEFT JOIN channel_acquisition_asset_provider_results AS result USING (effect_id)
WHERE binding.channel_id = sqlc.arg(channel_id)::bigint
  AND (sqlc.arg(after_effect_id)::bigint = 0 OR binding.effect_id < sqlc.arg(after_effect_id)::bigint)
ORDER BY binding.effect_id DESC
LIMIT sqlc.arg(result_limit)::int;

-- name: ListExpiredChannelAcquisitionAssetAttempts :many
SELECT effect_id, generation
FROM channel_acquisition_asset_bindings
WHERE state = 'attempted' AND lease_expires_at <= sqlc.arg(expired_at)::timestamptz
ORDER BY lease_expires_at, effect_id
LIMIT sqlc.arg(candidate_limit)::int
FOR UPDATE SKIP LOCKED;

-- name: MarkChannelAcquisitionAssetAttempted :one
UPDATE channel_acquisition_asset_bindings
SET state = 'attempted', fence = sqlc.arg(fence)::bigint, lease_expires_at = sqlc.arg(lease_expires_at)::timestamptz, updated_at = sqlc.arg(updated_at)::timestamptz
WHERE effect_id = sqlc.arg(effect_id)::bigint AND state = 'queued' AND generation = sqlc.arg(generation)::bigint AND fence = 0
RETURNING *;

-- name: InsertChannelAcquisitionAssetAttemptFact :one
INSERT INTO channel_acquisition_asset_attempt_facts(effect_id, generation, fence, state, receipt_id, receipt_digest, provider_call_attempted, real_external_call_executed, completed_at)
VALUES (sqlc.arg(effect_id)::bigint, sqlc.arg(generation)::bigint, sqlc.arg(fence)::bigint, sqlc.arg(state)::text, sqlc.arg(receipt_id)::bigint, sqlc.arg(receipt_digest)::text, sqlc.arg(provider_call_attempted)::boolean, sqlc.arg(real_external_call_executed)::boolean, sqlc.arg(completed_at)::timestamptz)
RETURNING id;

-- name: InsertChannelAcquisitionAssetObservedResult :exec
INSERT INTO channel_acquisition_asset_observed_results(attempt_fact_id, effect_id, outcome, asset_reference_digest, observed_at)
VALUES (sqlc.arg(attempt_fact_id)::bigint, sqlc.arg(effect_id)::bigint, sqlc.arg(outcome)::text, sqlc.narg(asset_reference_digest)::text, sqlc.arg(observed_at)::timestamptz);

-- name: CompleteChannelAcquisitionAssetAttempt :one
UPDATE channel_acquisition_asset_bindings
SET state = sqlc.arg(state)::text, attempt_receipt_id = sqlc.arg(attempt_receipt_id)::bigint, attempt_receipt_digest = sqlc.arg(attempt_receipt_digest)::text,
    provider_asset_reference_digest = sqlc.narg(provider_asset_reference_digest)::text, provider_call_attempted = sqlc.arg(provider_call_attempted)::boolean,
    real_external_call_executed = sqlc.arg(real_external_call_executed)::boolean, updated_at = sqlc.arg(updated_at)::timestamptz
WHERE effect_id = sqlc.arg(effect_id)::bigint AND state = 'attempted' AND generation = sqlc.arg(generation)::bigint AND fence = sqlc.arg(fence)::bigint
RETURNING *;

-- name: InsertChannelAcquisitionAssetProviderResult :exec
INSERT INTO channel_acquisition_asset_provider_results(effect_id, provider_asset_id, asset_url, created_at)
VALUES (sqlc.arg(effect_id)::bigint, sqlc.arg(provider_asset_id)::text, sqlc.arg(asset_url)::text, sqlc.arg(created_at)::timestamptz);

-- name: InsertChannelAcquisitionAssetReconciliationFact :exec
INSERT INTO channel_acquisition_asset_reconciliation_facts(effect_id, generation, fence, receipt_id, receipt_digest, evidence_digest, resolution, reconciled_at)
VALUES (sqlc.arg(effect_id)::bigint, sqlc.arg(generation)::bigint, sqlc.arg(fence)::bigint, sqlc.arg(receipt_id)::bigint, sqlc.arg(receipt_digest)::text, sqlc.arg(evidence_digest)::text, sqlc.arg(resolution)::text, sqlc.arg(reconciled_at)::timestamptz);

-- name: CompleteChannelAcquisitionAssetReconcile :one
UPDATE channel_acquisition_asset_bindings
SET state = 'reconciled', reconcile_receipt_id = sqlc.arg(reconcile_receipt_id)::bigint, reconcile_receipt_digest = sqlc.arg(reconcile_receipt_digest)::text,
    reconcile_evidence_digest = sqlc.arg(reconcile_evidence_digest)::text, reconcile_resolution = sqlc.arg(reconcile_resolution)::text, reconciled_at = sqlc.arg(reconciled_at)::timestamptz, updated_at = sqlc.arg(updated_at)::timestamptz
WHERE effect_id = sqlc.arg(effect_id)::bigint AND state = 'outcome_unknown' AND generation = sqlc.arg(generation)::bigint AND fence = sqlc.arg(fence)::bigint
RETURNING *;
