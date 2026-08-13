-- name: UpsertNormalizedIdentity :one
INSERT INTO identities (
  kind,
  scope,
  normalized_value,
  normalizer_version,
  assurance,
  source,
  review_fingerprint,
  fingerprint_key_version
) VALUES (
  sqlc.arg(kind)::text,
  sqlc.arg(scope)::text,
  sqlc.arg(normalized_value)::text,
  1,
  'declared',
  'identity.normalizer',
  decode('00000000000000000000000000000000', 'hex'),
  1
)
ON CONFLICT (kind, scope, normalized_value) DO UPDATE
SET normalized_value = EXCLUDED.normalized_value
RETURNING id, (xmax = 0) AS created;

-- name: LookupNormalizedIdentity :one
SELECT
  i.customer_id AS identity_customer_id,
  c.is_deleted AS customer_is_deleted
FROM identities AS i
LEFT JOIN customers AS c ON c.id = i.customer_id
WHERE i.kind = sqlc.arg(kind)::text
  AND i.scope = sqlc.arg(scope)::text
  AND i.normalized_value = sqlc.arg(normalized_value)::text;
