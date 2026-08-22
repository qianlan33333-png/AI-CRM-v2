-- name: ListProducts :many
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.version, p.local_lifecycle, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images pi WHERE pi.product_id = p.id
) images ON true
WHERE (sqlc.narg(after_id)::bigint IS NULL OR p.id > sqlc.narg(after_id)::bigint)
  AND COALESCE(p.legacy_admin_projection->>'status', '') NOT IN (
    'service_period_draft',
    'service_period_enabled',
    'service_period_disabled',
    'service_period_archived'
  )
ORDER BY p.id
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListProductsOffset :many
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.version, p.local_lifecycle, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images pi WHERE pi.product_id = p.id
) images ON true
WHERE COALESCE(p.legacy_admin_projection->>'status', '') NOT IN (
  'service_period_draft',
  'service_period_enabled',
  'service_period_disabled',
  'service_period_archived'
)
ORDER BY p.id
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountProducts :one
SELECT COUNT(*)::bigint
FROM products AS p
WHERE COALESCE(p.legacy_admin_projection->>'status', '') NOT IN (
  'service_period_draft',
  'service_period_enabled',
  'service_period_disabled',
  'service_period_archived'
);

-- name: GetProduct :one
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.version, p.local_lifecycle, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images pi WHERE pi.product_id = p.id
) images ON true
WHERE p.id = sqlc.arg(product_id)::bigint
  AND COALESCE(p.legacy_admin_projection->>'status', '') NOT IN (
    'service_period_draft',
    'service_period_enabled',
    'service_period_disabled',
    'service_period_archived'
  )
;

-- name: GetProductForUpdate :one
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.version, p.local_lifecycle, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images pi WHERE pi.product_id = p.id
) images ON true
WHERE p.id = sqlc.arg(product_id)::bigint
  AND COALESCE(p.legacy_admin_projection->>'status', '') NOT IN (
    'service_period_draft',
    'service_period_enabled',
    'service_period_disabled',
    'service_period_archived'
  )
FOR UPDATE OF p;

-- name: CreateProduct :one
INSERT INTO products (product_code, name, description, price_minor, currency, stock_quantity, created_by, created_at, updated_at, local_lifecycle, legacy_admin_projection)
VALUES (sqlc.arg(product_code)::text, sqlc.arg(name)::text, sqlc.arg(description)::text, sqlc.arg(price_minor)::bigint, sqlc.arg(currency)::char(3), sqlc.arg(stock_quantity)::integer, sqlc.arg(created_by)::bigint, sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz,
        CASE
          WHEN sqlc.arg(legacy_admin_projection)::jsonb->>'status' IN ('active', 'enabled')
            AND sqlc.arg(legacy_admin_projection)::jsonb->>'enabled' = 'true' THEN 'enabled'
          WHEN sqlc.arg(legacy_admin_projection)::jsonb->>'status' IN ('disabled', 'inactive')
            AND sqlc.arg(legacy_admin_projection)::jsonb->>'enabled' <> 'true' THEN 'disabled'
          ELSE 'draft'
        END,
        sqlc.arg(legacy_admin_projection)::jsonb)
RETURNING id, product_code, name, description, price_minor, currency, stock_quantity, created_by, created_at, updated_at, version, local_lifecycle, legacy_admin_projection;

-- name: UpdateProduct :one
UPDATE products
SET name = sqlc.arg(name)::text,
    description = sqlc.arg(description)::text,
    price_minor = sqlc.arg(price_minor)::bigint,
    currency = sqlc.arg(currency)::char(3),
    stock_quantity = sqlc.arg(stock_quantity)::integer,
    updated_at = sqlc.arg(updated_at)::timestamptz,
    version = version + 1
WHERE id = sqlc.arg(product_id)::bigint
  AND version = sqlc.arg(expected_version)::bigint
  AND COALESCE(legacy_admin_projection->>'status', '') NOT IN (
    'service_period_draft',
    'service_period_enabled',
    'service_period_disabled',
    'service_period_archived'
  )
RETURNING id, product_code, name, description, price_minor, currency, stock_quantity, created_by, created_at, updated_at, version, local_lifecycle, legacy_admin_projection;

-- name: IncrementProductCount :one
UPDATE product_catalog_counters SET total_products = total_products + 1
WHERE singleton = TRUE
RETURNING total_products;

-- name: InsertProductImage :exec
INSERT INTO product_images (product_id, position, image_url)
VALUES (sqlc.arg(product_id)::bigint, sqlc.arg(position)::integer, sqlc.arg(image_url)::text);

-- name: ReserveProductOperationReceipt :one
INSERT INTO product_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetProductOperationReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM product_operation_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteProductOperationReceipt :one
UPDATE product_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb, completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: ReserveEntitlementOperationReceipt :one
INSERT INTO entitlement_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetEntitlementOperationReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM entitlement_operation_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteEntitlementOperationReceipt :one
UPDATE entitlement_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb, completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: CreateProductLocalEntitlement :one
INSERT INTO product_local_entitlements
  (product_id, order_id, customer_id, state, version, granted_by, granted_at)
VALUES
  (sqlc.arg(product_id)::bigint, sqlc.arg(order_id)::bigint, sqlc.arg(customer_id)::bigint,
   'active', 1, sqlc.arg(granted_by)::bigint, sqlc.arg(granted_at)::timestamptz)
RETURNING id, product_id, order_id, customer_id, state, version, granted_at, revoked_at;

-- name: GetProductLocalEntitlement :one
SELECT id, product_id, order_id, customer_id, state, version, granted_at, revoked_at
FROM product_local_entitlements
WHERE id = sqlc.arg(entitlement_id)::bigint;

-- name: GetProductLocalEntitlementForUpdate :one
SELECT id, product_id, order_id, customer_id, state, version, granted_at, revoked_at
FROM product_local_entitlements
WHERE id = sqlc.arg(entitlement_id)::bigint
FOR UPDATE;

-- name: ListProductLocalEntitlements :many
SELECT id, product_id, order_id, customer_id, state, version, granted_at, revoked_at
FROM product_local_entitlements
WHERE product_id = sqlc.arg(product_id)::bigint
ORDER BY id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: RevokeProductLocalEntitlement :one
UPDATE product_local_entitlements
SET state = 'revoked', version = version + 1,
    revoked_by = sqlc.arg(revoked_by)::bigint, revoked_at = sqlc.arg(revoked_at)::timestamptz
WHERE id = sqlc.arg(entitlement_id)::bigint
  AND state = 'active'
  AND version = sqlc.arg(expected_version)::bigint
RETURNING id, product_id, order_id, customer_id, state, version, granted_at, revoked_at;
