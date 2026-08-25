-- name: LockChannelAcquisitionEntrantBinding :one
SELECT binding.effect_id, binding.channel_id, binding.asset_kind, binding.asset_version,
       binding.corp_id, binding.correlation_key, binding.assignee_wecom_userids,
       binding.state,
       CASE
         WHEN binding.state = 'executed' THEN (
           SELECT attempt.completed_at FROM channel_acquisition_asset_attempt_facts AS attempt
           WHERE attempt.effect_id = binding.effect_id AND attempt.state = 'executed'
           ORDER BY attempt.completed_at DESC LIMIT 1
         )
         WHEN binding.state = 'reconciled' AND binding.reconcile_resolution = 'provider_applied' THEN (
           SELECT reconciliation.reconciled_at FROM channel_acquisition_asset_reconciliation_facts AS reconciliation
           WHERE reconciliation.effect_id = binding.effect_id AND reconciliation.resolution = 'provider_applied'
           ORDER BY reconciliation.reconciled_at DESC LIMIT 1
         )
       END::timestamptz AS published_at
FROM channel_acquisition_asset_bindings AS binding
WHERE binding.effect_id = sqlc.arg(effect_id)::bigint
  AND binding.channel_id = sqlc.arg(channel_id)::bigint
  AND binding.asset_kind = sqlc.arg(asset_kind)::text
  AND binding.asset_version = sqlc.arg(asset_version)::bigint
FOR UPDATE;

-- name: InsertChannelAcquisitionEntrantReceipt :one
INSERT INTO channel_acquisition_entrant_receipts(
  inbox_id, input_digest, status, effect_id, channel_id, asset_kind, asset_version,
  customer_id, customer_event_id, customer_event_occurred_at, occurred_at,
  reconciled_at, reconcile_reason
) VALUES (
  sqlc.arg(inbox_id)::bigint, sqlc.arg(input_digest)::text, sqlc.arg(status)::text,
  sqlc.narg(effect_id)::bigint, sqlc.narg(channel_id)::bigint, sqlc.narg(asset_kind)::text, sqlc.narg(asset_version)::bigint,
  sqlc.narg(customer_id)::bigint, sqlc.narg(customer_event_id)::bigint, sqlc.narg(customer_event_occurred_at)::timestamptz,
  sqlc.arg(occurred_at)::timestamptz, sqlc.narg(reconciled_at)::timestamptz, sqlc.arg(reconcile_reason)::text
)
ON CONFLICT (inbox_id) DO NOTHING
RETURNING id, inbox_id, input_digest, status, effect_id, channel_id, asset_kind, asset_version,
          customer_id, customer_event_id, customer_event_occurred_at, occurred_at;

-- name: LockChannelAcquisitionEntrantReceipt :one
SELECT id, inbox_id, input_digest, status, effect_id, channel_id, asset_kind, asset_version,
       customer_id, customer_event_id, customer_event_occurred_at, occurred_at
FROM channel_acquisition_entrant_receipts
WHERE inbox_id = sqlc.arg(inbox_id)::bigint
FOR UPDATE;

-- name: LockChannelAcquisitionEntrantReceiptKey :exec
SELECT pg_advisory_xact_lock(hashtextextended('ch03.entrant:' || sqlc.arg(inbox_id)::text, 0));

-- name: TransitionChannelAcquisitionEntrantPending :one
UPDATE channel_acquisition_entrant_receipts
SET status = 'attributed', customer_id = sqlc.arg(customer_id)::bigint,
    customer_event_id = sqlc.arg(customer_event_id)::bigint,
    customer_event_occurred_at = sqlc.arg(customer_event_occurred_at)::timestamptz,
    updated_at = now()
WHERE inbox_id = sqlc.arg(inbox_id)::bigint
  AND input_digest = sqlc.arg(input_digest)::text
  AND status = 'pending_identity'
RETURNING id, inbox_id, input_digest, status, effect_id, channel_id, asset_kind, asset_version,
          customer_id, customer_event_id, customer_event_occurred_at, occurred_at;
