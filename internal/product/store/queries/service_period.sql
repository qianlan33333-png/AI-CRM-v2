-- name: UpdateServicePeriodProductProjection :execrows
WITH command AS (
  SELECT sqlc.arg(command)::jsonb AS payload
)
UPDATE products AS product
SET name = command.payload->>'name',
    description = command.payload->>'description',
    price_minor = (command.payload->>'price_minor')::bigint,
    currency = (command.payload->>'currency')::char(3),
    stock_quantity = (command.payload->>'stock_quantity')::integer,
    legacy_admin_projection = command.payload->'legacy_admin_projection',
    updated_at = (command.payload->>'updated_at')::timestamptz,
    version = product.version + 1
FROM command
WHERE product.id = (command.payload->>'product_id')::bigint
  AND product.version = (command.payload->>'expected_version')::bigint
  AND (
    product.legacy_admin_projection->>'status' = 'service_period_enabled'
    AND product.legacy_admin_projection->>'enabled' = 'true'
    OR product.legacy_admin_projection->>'status' IN (
      'service_period_draft',
      'service_period_disabled',
      'service_period_archived'
    ) AND product.legacy_admin_projection->>'enabled' = 'false'
  );
-- name: ListServicePeriodProductRows :many
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.version, p.local_lifecycle, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products AS p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images AS pi
  WHERE pi.product_id = p.id
) AS images ON true
WHERE (
  p.legacy_admin_projection->>'status' = 'service_period_enabled'
  AND p.legacy_admin_projection->>'enabled' = 'true'
  OR p.legacy_admin_projection->>'status' IN (
    'service_period_draft',
    'service_period_disabled',
    'service_period_archived'
  ) AND p.legacy_admin_projection->>'enabled' = 'false'
)
ORDER BY p.id
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountServicePeriodProductRows :one
SELECT COUNT(*)::bigint
FROM products AS p
WHERE (
  p.legacy_admin_projection->>'status' = 'service_period_enabled'
  AND p.legacy_admin_projection->>'enabled' = 'true'
  OR p.legacy_admin_projection->>'status' IN (
    'service_period_draft',
    'service_period_disabled',
    'service_period_archived'
  ) AND p.legacy_admin_projection->>'enabled' = 'false'
);

-- name: GetServicePeriodProductRow :one
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.version, p.local_lifecycle, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products AS p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images AS pi
  WHERE pi.product_id = p.id
) AS images ON true
WHERE p.id = sqlc.arg(product_id)::bigint
  AND (
    p.legacy_admin_projection->>'status' = 'service_period_enabled'
    AND p.legacy_admin_projection->>'enabled' = 'true'
    OR p.legacy_admin_projection->>'status' IN (
      'service_period_draft',
      'service_period_disabled',
      'service_period_archived'
    ) AND p.legacy_admin_projection->>'enabled' = 'false'
  );

-- name: GetServicePeriodProductRowForUpdate :one
SELECT p.id, p.product_code, p.name, p.description, p.price_minor, p.currency, p.stock_quantity, p.created_by, p.created_at, p.updated_at, p.version, p.local_lifecycle, p.legacy_admin_projection,
       COALESCE(images.items, '[]'::jsonb) AS images
FROM products AS p
LEFT JOIN LATERAL (
  SELECT jsonb_agg(pi.image_url ORDER BY pi.position) AS items
  FROM product_images AS pi
  WHERE pi.product_id = p.id
) AS images ON true
WHERE p.id = sqlc.arg(product_id)::bigint
  AND (
    p.legacy_admin_projection->>'status' = 'service_period_enabled'
    AND p.legacy_admin_projection->>'enabled' = 'true'
    OR p.legacy_admin_projection->>'status' IN (
      'service_period_draft',
      'service_period_disabled',
      'service_period_archived'
    ) AND p.legacy_admin_projection->>'enabled' = 'false'
  )
FOR UPDATE OF p;
