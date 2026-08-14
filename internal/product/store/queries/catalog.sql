-- name: ListProducts :many
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images pi WHERE pi.product_id = p.id
) images ON true
WHERE (sqlc.narg(after_id)::bigint IS NULL OR p.id > sqlc.narg(after_id)::bigint)
ORDER BY p.id
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListProductsOffset :many
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images pi WHERE pi.product_id = p.id
) images ON true
ORDER BY p.id
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountProducts :one
SELECT total_products FROM product_catalog_counters WHERE singleton = TRUE;

-- name: GetProduct :one
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images pi WHERE pi.product_id = p.id
) images ON true
WHERE p.id = sqlc.arg(product_id)::bigint
;

-- name: CreateProduct :one
INSERT INTO products (product_code, name, description, price_minor, currency, stock_quantity, created_by, created_at, updated_at, legacy_admin_projection)
VALUES (sqlc.arg(product_code)::text, sqlc.arg(name)::text, sqlc.arg(description)::text, sqlc.arg(price_minor)::bigint, sqlc.arg(currency)::char(3), sqlc.arg(stock_quantity)::integer, sqlc.arg(created_by)::bigint, sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz, sqlc.arg(legacy_admin_projection)::jsonb)
RETURNING id, product_code, name, description, price_minor, currency, stock_quantity, created_by, created_at, updated_at, legacy_admin_projection;

-- name: IncrementProductCount :one
UPDATE product_catalog_counters SET total_products = total_products + 1
WHERE singleton = TRUE
RETURNING total_products;

-- name: InsertProductImage :exec
INSERT INTO product_images (product_id, position, image_url)
VALUES (sqlc.arg(product_id)::bigint, sqlc.arg(position)::integer, sqlc.arg(image_url)::text);

-- name: ReserveProductOperationReceipt :one
INSERT INTO product_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES ('create', sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetProductOperationReceipt :one
SELECT id, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM product_operation_receipts
WHERE operation = 'create' AND actor_scope = sqlc.arg(actor_scope)::text AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteProductOperationReceipt :one
UPDATE product_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb, completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, actor_scope, key_digest, payload_digest, state, result_snapshot;
