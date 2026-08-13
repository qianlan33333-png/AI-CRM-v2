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

-- name: ReserveBindReceipt :one
INSERT INTO identity_operation_receipts (
  operation,
  idempotency_scope,
  key_digest,
  command_schema_version,
  payload_hmac,
  payload_hmac_key_version,
  result_schema_version
) VALUES (
  'bind',
  'identity.bind.v1',
  sqlc.arg(key_digest)::bytea,
  1,
  sqlc.arg(payload_hmac)::bytea,
  1,
  1
)
ON CONFLICT (operation, idempotency_scope, key_digest) DO NOTHING
RETURNING id;

-- name: LoadBindReceipt :one
SELECT
  payload_hmac,
  state,
  result_status,
  result_customer_id
FROM identity_operation_receipts
WHERE operation = 'bind'
  AND idempotency_scope = 'identity.bind.v1'
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteBindReceipt :execrows
UPDATE identity_operation_receipts
SET
  state = 'completed',
  result_status = sqlc.arg(result_status)::text,
  result_customer_id = sqlc.narg(result_customer_id)::bigint,
  completed_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'in_progress';

-- name: LockActiveBindCustomer :one
SELECT id
FROM customers
WHERE id = sqlc.arg(customer_id)::bigint
  AND is_deleted = FALSE
FOR UPDATE;

-- name: LockIdentityForBind :one
SELECT id, customer_id
FROM identities
WHERE kind = sqlc.arg(kind)::text
  AND scope = sqlc.arg(scope)::text
  AND normalized_value = sqlc.arg(normalized_value)::text
FOR UPDATE;

-- name: BindFloatingIdentity :one
UPDATE identities
SET customer_id = sqlc.arg(customer_id)::bigint, bound_at = now()
WHERE id = sqlc.arg(identity_id)::bigint
  AND customer_id IS NULL
RETURNING id;
