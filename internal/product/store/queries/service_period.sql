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
  AND product.legacy_admin_projection->>'status' IN (
    'service_period_draft',
    'service_period_enabled',
    'service_period_disabled',
    'service_period_archived'
  );
