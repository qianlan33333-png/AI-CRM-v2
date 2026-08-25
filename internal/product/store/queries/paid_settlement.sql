-- name: LockProductForPaidSettlement :one
SELECT id, version, product_code, name, price_minor, currency,
  CASE
    WHEN legacy_admin_projection->>'status' = 'service_period_enabled'
      AND legacy_admin_projection->>'enabled' = 'true' THEN 'service_period'
    ELSE 'ordinary'
  END::text AS product_kind
FROM public.products
WHERE id = sqlc.arg(product_id)
  AND version = sqlc.arg(product_version)
  AND (
    sqlc.arg(product_kind)::text = 'ordinary' AND local_lifecycle = 'enabled'
    OR sqlc.arg(product_kind)::text = 'service_period'
      AND legacy_admin_projection->>'status' = 'service_period_enabled'
      AND legacy_admin_projection->>'enabled' = 'true'
  )
FOR SHARE;

-- name: CreatePE01AcceptanceProduct :one
INSERT INTO products (
  product_code, name, description, price_minor, currency, stock_quantity,
  created_by, created_at, updated_at, local_lifecycle
) VALUES (
  sqlc.arg(product_code), 'PE01 ordinary', 'fake-provider acceptance',
  1990, 'CNY', 1, 1, sqlc.arg(created_at), sqlc.arg(created_at), 'enabled'
)
RETURNING id;

-- name: CreatePaidOrderEntitlement :one
INSERT INTO public.product_local_entitlements (
  product_id, order_id, customer_id, state, version, granted_by,
  granted_at, source, settlement_receipt_digest, granted_actor_scope
) VALUES (
  sqlc.arg(product_id), sqlc.arg(order_id), sqlc.arg(customer_id),
  'active', 1, NULL, sqlc.arg(granted_at), 'paid_order',
  sqlc.arg(settlement_receipt_digest), 'provider:wechat'
)
RETURNING id, product_id, order_id, customer_id, state, version, granted_at, revoked_at;

-- name: LockPaidOrderEntitlement :one
SELECT id, product_id, order_id, customer_id, state, version,
  settlement_receipt_digest, granted_at, revoked_at
FROM public.product_local_entitlements
WHERE order_id = sqlc.arg(order_id) AND source = 'paid_order'
FOR UPDATE;

-- name: RevokePaidOrderEntitlement :one
UPDATE public.product_local_entitlements
SET state = 'revoked', version = version + 1, revoked_at = sqlc.arg(revoked_at),
    revoked_actor_scope = 'provider:wechat'
WHERE order_id = sqlc.arg(order_id) AND source = 'paid_order'
  AND state = 'active' AND version = sqlc.arg(expected_version)
  AND settlement_receipt_digest = sqlc.arg(settlement_receipt_digest)
RETURNING id, product_id, order_id, customer_id, state, version, granted_at, revoked_at;

-- name: CreatePaidOrderServicePeriodMember :one
INSERT INTO public.service_period_members (
  member_ref, service_product_id, customer_id, state, source, starts_at,
  expires_at, remark, alliance, version, created_at, updated_at,
  pe01_lineage_version, paid_order_id, entitlement_id, settlement_receipt_digest
) VALUES (
  sqlc.arg(member_ref), sqlc.arg(product_id), sqlc.arg(customer_id),
  'active', 'paid_order', sqlc.arg(starts_at), NULL, NULL, NULL, 1,
  sqlc.arg(starts_at), sqlc.arg(starts_at), 'pe01/v1', sqlc.arg(order_id),
  sqlc.arg(entitlement_id), sqlc.arg(settlement_receipt_digest)
)
RETURNING member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, version, created_at, updated_at;

-- name: LockPaidOrderServicePeriodMember :one
SELECT member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, version, created_at, updated_at
FROM public.service_period_members
WHERE paid_order_id = sqlc.arg(order_id) AND pe01_lineage_version = 'pe01/v1'
FOR UPDATE;

-- name: RemovePaidOrderServicePeriodMember :one
UPDATE public.service_period_members
SET state = 'removed', removed_at = sqlc.arg(removed_at),
    version = version + 1, updated_at = sqlc.arg(removed_at)
WHERE paid_order_id = sqlc.arg(order_id) AND pe01_lineage_version = 'pe01/v1'
  AND state <> 'removed' AND version = sqlc.arg(expected_version)
  AND settlement_receipt_digest = sqlc.arg(settlement_receipt_digest)
RETURNING member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, version, created_at, updated_at;
