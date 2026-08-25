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
FOR UPDATE OF p, c;

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
