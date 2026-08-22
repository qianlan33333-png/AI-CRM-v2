-- name: ServicePeriodProductExists :one
SELECT EXISTS (
  SELECT 1
  FROM public.products
  WHERE id = sqlc.arg(product_id)
    AND (
      legacy_admin_projection->>'status' = 'service_period_enabled'
      AND legacy_admin_projection->>'enabled' = 'true'
      OR legacy_admin_projection->>'status' IN (
        'service_period_draft',
        'service_period_disabled',
        'service_period_archived'
      )
      AND legacy_admin_projection->>'enabled' = 'false'
    )
);

-- name: LockServiceProductForMemberAdd :one
SELECT true
FROM public.products
WHERE id = sqlc.arg(product_id)
  AND legacy_admin_projection->>'status' = 'service_period_enabled'
  AND legacy_admin_projection->>'enabled' = 'true'
FOR SHARE;

-- name: ServicePeriodMemberCustomerExists :one
SELECT EXISTS (
  SELECT 1 FROM public.customers WHERE id = sqlc.arg(customer_id)
);

-- name: GetServicePeriodMember :one
SELECT member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, remark, alliance,
  version, created_at, updated_at
FROM public.service_period_members
WHERE service_product_id = sqlc.arg(product_id)
  AND member_ref = sqlc.arg(member_ref);

-- name: LockServicePeriodMember :one
SELECT member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, remark, alliance,
  version, created_at, updated_at
FROM public.service_period_members
WHERE service_product_id = sqlc.arg(product_id)
  AND member_ref = sqlc.arg(member_ref)
FOR UPDATE;

-- name: CreateServicePeriodMember :one
INSERT INTO public.service_period_members (
  member_ref, service_product_id, customer_id, state, source, starts_at,
  expires_at, remark, alliance, version, created_at, updated_at
) VALUES (
  sqlc.arg(member_ref), sqlc.arg(product_id), sqlc.arg(customer_id), 'active',
  sqlc.arg(source), sqlc.arg(starts_at), sqlc.narg(expires_at),
  sqlc.narg(remark), sqlc.narg(alliance), 1, sqlc.arg(created_at), sqlc.arg(created_at)
)
RETURNING member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, remark, alliance,
  version, created_at, updated_at;

-- name: TransitionServicePeriodMember :one
UPDATE public.service_period_members
SET state = sqlc.arg(target_state),
    expired_at = CASE WHEN sqlc.arg(target_state) = 'expired' THEN sqlc.arg(transitioned_at) ELSE expired_at END,
    removed_at = CASE WHEN sqlc.arg(target_state) = 'removed' THEN sqlc.arg(transitioned_at) ELSE removed_at END,
    version = version + 1,
    updated_at = sqlc.arg(transitioned_at)
WHERE service_product_id = sqlc.arg(product_id)
  AND member_ref = sqlc.arg(member_ref)
  AND version = sqlc.arg(expected_version)
  AND (
    sqlc.arg(target_state) = 'expired' AND state = 'active'
    OR sqlc.arg(target_state) = 'removed' AND state <> 'removed'
  )
RETURNING member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, remark, alliance,
  version, created_at, updated_at;

-- name: UpdateServicePeriodMemberFields :one
UPDATE public.service_period_members
SET remark = sqlc.narg(remark),
    alliance = sqlc.narg(alliance),
    version = version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE service_product_id = sqlc.arg(product_id)
  AND member_ref = sqlc.arg(member_ref)
  AND version = sqlc.arg(expected_version)
  AND state <> 'removed'
RETURNING member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, remark, alliance,
  version, created_at, updated_at;

-- name: ListServicePeriodMembers :many
SELECT member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, remark, alliance,
  version, created_at, updated_at
FROM public.service_period_members
WHERE service_product_id = sqlc.arg(product_id)
  AND (sqlc.narg(state)::text IS NULL OR state = sqlc.narg(state))
  AND (sqlc.narg(source)::text IS NULL OR source = sqlc.narg(source))
  AND (
    sqlc.narg(after_updated_at)::timestamptz IS NULL
    OR (updated_at, member_ref) < (sqlc.narg(after_updated_at), sqlc.narg(after_member_ref)::text)
  )
ORDER BY updated_at DESC, member_ref DESC
LIMIT sqlc.arg(row_limit);

-- name: ReserveServicePeriodMemberReceipt :one
INSERT INTO public.service_period_member_operation_receipts (
  operation, actor_scope, key_digest, payload_digest, state, created_at
) VALUES (
  sqlc.arg(operation), sqlc.arg(actor_scope), sqlc.arg(key_digest),
  sqlc.arg(payload_digest), 'reserved', sqlc.arg(created_at)
)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state,
  result_snapshot, created_at;

-- name: GetServicePeriodMemberReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state,
  result_snapshot, created_at
FROM public.service_period_member_operation_receipts
WHERE operation = sqlc.arg(operation)
  AND actor_scope = sqlc.arg(actor_scope)
  AND key_digest = sqlc.arg(key_digest);

-- name: CompleteServicePeriodMemberReceipt :one
UPDATE public.service_period_member_operation_receipts
SET state = 'completed',
    result_snapshot = sqlc.arg(result_snapshot),
    completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(receipt_id)
  AND state = 'reserved'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state,
  result_snapshot, created_at;
