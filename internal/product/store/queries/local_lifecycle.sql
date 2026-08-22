-- Product-local WeChat-pay lifecycle queries. The command payload is a
-- closed JSON object produced by the Product app port; it is not a browser
-- contract and contains no provider token or payment request.

-- name: UpdateLocalProductLifecycle :execrows
WITH command AS (
  SELECT sqlc.arg(command)::jsonb AS payload
)
UPDATE products AS product
SET legacy_admin_projection = command.payload->'legacy_admin_projection',
    updated_at = (command.payload->>'updated_at')::timestamptz,
    version = product.version + 1
FROM command
WHERE product.id = (command.payload->>'product_id')::bigint
  AND product.version = (command.payload->>'expected_version')::bigint;

-- Only a draft with no local entitlement, order projection, or other Product
-- fact may be physically deleted. Product images may cascade with the draft;
-- financial/order/entitlement facts never do.
-- name: DeleteLocalProductIfSafe :one
WITH command AS (
  SELECT sqlc.arg(command)::jsonb AS payload
), doomed AS (
  DELETE FROM products AS product
  USING command
  WHERE product.id = (command.payload->>'product_id')::bigint
    AND product.version = (command.payload->>'expected_version')::bigint
    AND product.legacy_admin_projection->>'status' = 'draft'
    AND COALESCE((product.legacy_admin_projection->>'enabled')::boolean, false) = false
    AND NOT EXISTS (
      SELECT 1
      FROM product_local_entitlements entitlement
      WHERE entitlement.product_id = product.id
    )
    AND NOT EXISTS (
      SELECT 1
      FROM coupon_targets coupon_target
      WHERE coupon_target.product_id = product.id
    )
    AND NOT EXISTS (
      SELECT 1
      FROM order_list_projections order_projection
      WHERE order_projection.product_id = product.id
    )
    AND NOT EXISTS (
      SELECT 1
      FROM service_period_member_views member_view
      WHERE member_view.service_product_id = product.id
    )
    AND NOT EXISTS (
      SELECT 1
      FROM service_period_member_grid_collaborators collaborator
      WHERE collaborator.service_product_id = product.id
    )
  RETURNING product.id
), counter AS (
  UPDATE product_catalog_counters
  SET total_products = total_products - 1
  WHERE singleton = TRUE AND EXISTS (SELECT 1 FROM doomed)
  RETURNING total_products
)
SELECT count(*)::bigint AS deleted_count
FROM doomed;
