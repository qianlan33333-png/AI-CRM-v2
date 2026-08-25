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

-- name: ListAdminChannelAcquisitionEntrantReceipts :many
SELECT r.id, r.channel_id, r.effect_id, r.asset_kind, r.asset_version, r.status,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconciled_at, r.reconcile_reason, r.created_at, r.updated_at
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id = r.inbox_id
JOIN channel_acquisition_asset_bindings b ON (b.effect_id,b.channel_id,b.asset_kind,b.asset_version) = (r.effect_id,r.channel_id,r.asset_kind,r.asset_version)
JOIN admin_users a ON a.id = sqlc.arg(actor_id)::bigint AND a.is_active AND a.login_enabled AND a.wecom_corp_id = i.corp_id AND a.wecom_corp_id = b.corp_id
WHERE r.channel_id = sqlc.arg(channel_id)::bigint
  AND (sqlc.arg(after_receipt_id)::bigint <= 0 OR r.id < sqlc.arg(after_receipt_id)::bigint)
ORDER BY r.id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: GetAdminChannelAcquisitionEntrantReceipt :one
SELECT r.id, r.channel_id, r.effect_id, r.asset_kind, r.asset_version, r.status,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconciled_at, r.reconcile_reason, r.created_at, r.updated_at
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id = r.inbox_id
JOIN channel_acquisition_asset_bindings b ON (b.effect_id,b.channel_id,b.asset_kind,b.asset_version) = (r.effect_id,r.channel_id,r.asset_kind,r.asset_version)
JOIN admin_users a ON a.id = sqlc.arg(actor_id)::bigint AND a.is_active AND a.login_enabled AND a.wecom_corp_id = i.corp_id AND a.wecom_corp_id = b.corp_id
WHERE r.channel_id = sqlc.arg(channel_id)::bigint
  AND r.id = sqlc.arg(receipt_id)::bigint;

-- name: ListAdminUnassignedChannelAcquisitionEntrantReceipts :many
SELECT r.id, r.channel_id, r.effect_id, r.asset_kind, r.asset_version, r.status,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconciled_at, r.reconcile_reason, r.created_at, r.updated_at
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id = r.inbox_id
JOIN admin_users a ON a.id = sqlc.arg(actor_id)::bigint AND a.is_active AND a.login_enabled AND a.wecom_corp_id = i.corp_id
WHERE r.effect_id IS NULL AND r.channel_id IS NULL AND r.asset_kind IS NULL AND r.asset_version IS NULL
  AND r.status IN ('unmatched_asset', 'ambiguous_asset', 'ignored')
  AND (sqlc.arg(after_receipt_id)::bigint <= 0 OR r.id < sqlc.arg(after_receipt_id)::bigint)
ORDER BY r.id DESC
LIMIT sqlc.arg(page_limit)::integer;

-- name: GetAdminUnassignedChannelAcquisitionEntrantReceipt :one
SELECT r.id, r.channel_id, r.effect_id, r.asset_kind, r.asset_version, r.status,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconciled_at, r.reconcile_reason, r.created_at, r.updated_at
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id = r.inbox_id
JOIN admin_users a ON a.id = sqlc.arg(actor_id)::bigint AND a.is_active AND a.login_enabled AND a.wecom_corp_id = i.corp_id
WHERE r.effect_id IS NULL AND r.channel_id IS NULL AND r.asset_kind IS NULL AND r.asset_version IS NULL
  AND r.status IN ('unmatched_asset', 'ambiguous_asset', 'ignored')
  AND r.id = sqlc.arg(receipt_id)::bigint;

-- name: LockAdminChannelAcquisitionEntrantReceipt :one
SELECT r.id, r.status, r.effect_id, r.channel_id, r.asset_kind, r.asset_version,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconcile_actor_id, r.reconcile_key_digest, r.reconcile_payload_digest,
       i.external_userid, i.external_contact_wecom_userid, i.corp_id
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id=r.inbox_id
JOIN admin_users a ON a.id=sqlc.arg(actor_id)::bigint AND a.is_active AND a.login_enabled AND a.wecom_corp_id=i.corp_id
WHERE r.id=sqlc.arg(receipt_id)::bigint
  AND r.channel_id=sqlc.arg(channel_id)::bigint
  AND EXISTS (
    SELECT 1 FROM channel_acquisition_asset_bindings current_binding
    WHERE (current_binding.effect_id,current_binding.channel_id,current_binding.asset_kind,current_binding.asset_version) = (r.effect_id,r.channel_id,r.asset_kind,r.asset_version)
      AND current_binding.corp_id=i.corp_id
  )
FOR UPDATE OF r;

-- name: LockAdminUnassignedChannelAcquisitionEntrantReceipt :one
SELECT r.id, r.status, r.effect_id, r.channel_id, r.asset_kind, r.asset_version,
       r.customer_id, r.customer_event_id, r.occurred_at, r.reconcile_actor_id, r.reconcile_key_digest, r.reconcile_payload_digest,
       i.external_userid, i.external_contact_wecom_userid, i.corp_id
FROM channel_acquisition_entrant_receipts r
JOIN wecom_contact_inbox i ON i.id=r.inbox_id
JOIN admin_users a ON a.id=sqlc.arg(actor_id)::bigint AND a.is_active AND a.login_enabled AND a.wecom_corp_id=i.corp_id
WHERE r.id=sqlc.arg(receipt_id)::bigint
  AND (
    (r.effect_id IS NULL AND r.channel_id IS NULL AND r.asset_kind IS NULL AND r.asset_version IS NULL)
    OR (r.status='reconciled' AND r.reconcile_actor_id=sqlc.arg(actor_id)::bigint AND r.reconcile_key_digest=sqlc.arg(key_digest)::text AND r.reconcile_payload_digest=sqlc.arg(payload_digest)::text)
  )
FOR UPDATE OF r;

-- name: FindAdminChannelAcquisitionEntrantReconcileKey :one
SELECT id
FROM channel_acquisition_entrant_receipts
WHERE reconcile_actor_id=sqlc.arg(actor_id)::bigint
  AND reconcile_key_digest=sqlc.arg(key_digest)::text;

-- name: LockAdminChannelAcquisitionEntrantTargetBinding :one
SELECT b.channel_id, b.asset_kind, b.asset_version,
       CASE WHEN b.state = 'executed' THEN (SELECT max(f.completed_at) FROM channel_acquisition_asset_attempt_facts f WHERE f.effect_id=b.effect_id AND f.state='executed')
            WHEN b.state = 'reconciled' AND b.reconcile_resolution='provider_applied' THEN (SELECT max(f.reconciled_at) FROM channel_acquisition_asset_reconciliation_facts f WHERE f.effect_id=b.effect_id AND f.resolution='provider_applied') END::timestamptz AS published_at,
       b.assignee_wecom_userids
FROM channel_acquisition_asset_bindings b
WHERE b.effect_id=sqlc.arg(effect_id)::bigint AND b.corp_id=sqlc.arg(corp_id)::text
  AND (sqlc.arg(channel_id)::bigint=0 OR b.channel_id=sqlc.arg(channel_id)::bigint)
FOR UPDATE OF b;

-- name: LockAdminChannelAcquisitionEntrantCustomer :one
SELECT id
FROM customers
WHERE id=sqlc.arg(customer_id)::bigint AND NOT is_deleted
FOR UPDATE;

-- name: LockAdminChannelAcquisitionEntrantIdentity :one
SELECT customer_id
FROM identities
WHERE kind='wecom_external_userid'
  AND scope='wecom-corp:' || sqlc.arg(corp_id)::text
  AND normalized_value=sqlc.arg(external_userid)::text
  AND assurance='verified'
FOR UPDATE;

-- name: CompleteAdminChannelAcquisitionEntrantReconciliation :execrows
UPDATE channel_acquisition_entrant_receipts
SET status='reconciled', effect_id=sqlc.arg(effect_id)::bigint, channel_id=sqlc.arg(channel_id)::bigint,
    asset_kind=sqlc.arg(asset_kind)::text, asset_version=sqlc.arg(asset_version)::bigint,
    customer_id=sqlc.arg(customer_id)::bigint, customer_event_id=sqlc.arg(customer_event_id)::bigint,
    customer_event_occurred_at=sqlc.arg(event_at)::timestamptz, reconciled_at=sqlc.arg(event_at)::timestamptz,
    reconcile_reason=sqlc.arg(reason)::text, reconcile_actor_id=sqlc.arg(actor_id)::bigint,
    reconcile_key_digest=sqlc.arg(key_digest)::text, reconcile_payload_digest=sqlc.arg(payload_digest)::text,
    updated_at=sqlc.arg(event_at)::timestamptz
WHERE id=sqlc.arg(receipt_id)::bigint AND status=sqlc.arg(prior_status)::text;

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
