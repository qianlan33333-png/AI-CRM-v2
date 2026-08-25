-- name: ReadProductExternalPushConfiguration :one
SELECT p.id AS product_id,
       COALESCE(c.product_kind, sqlc.arg(product_kind)::text) AS product_kind,
       COALESCE(c.enabled, FALSE) AS enabled,
       COALESCE(c.configuration_reference, '') AS configuration_reference,
       COALESCE(c.updated_at, p.updated_at) AS updated_at
FROM public.products AS p
LEFT JOIN public.product_external_push_configurations AS c ON c.product_id = p.id
WHERE p.id = sqlc.arg(product_id)::bigint;

-- name: LockProductExternalPushConfiguration :one
SELECT p.id AS product_id,
       COALESCE(c.product_kind, sqlc.arg(product_kind)::text) AS product_kind,
       COALESCE(c.enabled, FALSE) AS enabled,
       COALESCE(c.configuration_reference, '') AS configuration_reference,
       COALESCE(c.updated_at, p.updated_at) AS updated_at
FROM public.products AS p
LEFT JOIN public.product_external_push_configurations AS c ON c.product_id = p.id
WHERE p.id = sqlc.arg(product_id)::bigint
FOR UPDATE OF p;

-- name: SaveProductExternalPushConfiguration :one
INSERT INTO public.product_external_push_configurations
  (product_id, product_kind, enabled, configuration_reference, updated_at)
VALUES (
  sqlc.arg(product_id)::bigint,
  sqlc.arg(product_kind)::text,
  sqlc.arg(enabled)::boolean,
  sqlc.arg(configuration_reference)::text,
  sqlc.arg(updated_at)::timestamptz
)
ON CONFLICT (product_id) DO UPDATE
SET product_kind = EXCLUDED.product_kind,
    enabled = EXCLUDED.enabled,
    configuration_reference = EXCLUDED.configuration_reference,
    updated_at = EXCLUDED.updated_at
RETURNING product_id, product_kind, enabled, configuration_reference, updated_at;

-- name: CreateProductExternalPushTestBinding :one
WITH accepted_effect AS (
  SELECT id
  FROM public.external_effects
  WHERE id = sqlc.arg(external_effect_id)::bigint
    AND owner = 'product'
    AND kind = 'product_external_push_test'
    AND state = sqlc.arg(state)::text
)
INSERT INTO public.product_external_push_test_bindings
  (product_id, product_kind, operation_receipt_id, external_effect_id,
   configuration_digest, state, created_at)
SELECT sqlc.arg(product_id)::bigint,
       sqlc.arg(product_kind)::text,
       sqlc.arg(operation_receipt_id)::bigint,
       accepted_effect.id,
       sqlc.arg(configuration_digest)::bytea,
       sqlc.arg(state)::text,
       sqlc.arg(created_at)::timestamptz
FROM accepted_effect
RETURNING product_id, product_kind, external_effect_id, state,
          provider_accepted, delivery_proven, real_external_call_executed,
          auto_retry_allowed, created_at;

-- name: ProductExternalPushTestExists :one
SELECT EXISTS (
  SELECT 1
  FROM public.product_external_push_test_bindings
  WHERE product_id = sqlc.arg(product_id)::bigint
    AND product_kind = sqlc.arg(product_kind)::text
    AND configuration_digest = sqlc.arg(configuration_digest)::bytea
);

-- name: CommerceExternalPushAcceptanceServerVersion :one
SELECT current_setting('server_version_num')::text AS server_version_num;

-- name: ReadCommerceExternalPushAcceptanceFacts :one
SELECT
  (SELECT count(*) FROM public.product_external_push_configurations WHERE product_id=sqlc.arg(product_id)::bigint)::bigint AS configurations,
  (SELECT count(*) FROM public.product_operation_receipts WHERE operation IN ('external_push_save','external_push_test') AND state='completed')::bigint AS product_receipts,
  (SELECT count(*) FROM public.external_effects WHERE owner='product' AND kind='product_external_push_test' AND state='accepted')::bigint AS effects,
  (SELECT count(*) FROM public.external_effect_receipts AS r JOIN public.external_effects AS e ON e.id=r.effect_id WHERE e.owner='product' AND r.operation='accept' AND r.state='accepted')::bigint AS effect_receipts,
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=sqlc.arg(product_id)::bigint AND state='accepted')::bigint AS bindings,
  (SELECT count(*) FROM public.external_effect_attempts AS a JOIN public.external_effects AS e ON e.id=a.effect_id WHERE e.owner='product')::bigint AS attempts,
  (SELECT count(*) FROM public.external_effects WHERE owner='product' AND river_job_id IS NOT NULL)::bigint AS river_bound_effects,
  (SELECT count(*) FROM public.product_external_push_test_bindings WHERE product_id=sqlc.arg(product_id)::bigint AND (provider_accepted OR delivery_proven OR real_external_call_executed OR auto_retry_allowed))::bigint AS provider_or_delivery;
