-- name: LockCustomerContactPolicyKey :exec
SELECT pg_advisory_xact_lock(
  hashtextextended('contact:customer-contact-policy:' || sqlc.arg(customer_id)::bigint::text, 0)
);

-- name: LockCustomerContactPolicyKeys :exec
WITH ordered_customer_ids AS MATERIALIZED (
  SELECT customer_id
  FROM unnest(sqlc.arg(customer_ids)::bigint[]) AS requested(customer_id)
  ORDER BY customer_id
)
SELECT pg_advisory_xact_lock(
  hashtextextended('contact:customer-contact-policy:' || customer_id::text, 0)
)
FROM ordered_customer_ids;

-- name: LockActiveCustomerForContactPolicy :one
SELECT id
FROM public.customers
WHERE id = sqlc.arg(customer_id)::bigint AND NOT is_deleted
FOR SHARE;

-- name: LockActiveCustomerContactPolicies :many
SELECT c.id AS customer_id,
       p.customer_id AS policy_customer_id,
       p.reason_code,
       p.suppressed_until,
       p.version
FROM public.customers AS c
LEFT JOIN public.customer_contact_policies AS p ON p.customer_id = c.id
WHERE c.id = ANY(sqlc.arg(customer_ids)::bigint[])
  AND NOT c.is_deleted
ORDER BY c.id
FOR SHARE OF c;

-- name: GetCustomerContactPolicy :one
SELECT customer_id, reason_code, suppressed_until, version, created_at, updated_at
FROM public.customer_contact_policies
WHERE customer_id = sqlc.arg(customer_id)::bigint;

-- name: InsertCustomerContactPolicy :one
INSERT INTO public.customer_contact_policies (
  customer_id, reason_code, suppressed_until, created_at, updated_at
)
VALUES (
  sqlc.arg(customer_id)::bigint,
  sqlc.arg(reason_code)::text,
  sqlc.narg(suppressed_until)::timestamptz,
  sqlc.arg(changed_at)::timestamptz,
  sqlc.arg(changed_at)::timestamptz
)
RETURNING customer_id, reason_code, suppressed_until, version, created_at, updated_at;

-- name: UpdateCustomerContactPolicy :one
UPDATE public.customer_contact_policies
SET reason_code = sqlc.arg(reason_code)::text,
    suppressed_until = sqlc.narg(suppressed_until)::timestamptz,
    version = version + 1,
    updated_at = sqlc.arg(changed_at)::timestamptz
WHERE customer_id = sqlc.arg(customer_id)::bigint
  AND version = sqlc.arg(expected_version)::bigint
RETURNING customer_id, reason_code, suppressed_until, version, created_at, updated_at;

-- name: DeleteCustomerContactPolicy :execrows
DELETE FROM public.customer_contact_policies
WHERE customer_id = sqlc.arg(customer_id)::bigint
  AND version = sqlc.arg(expected_version)::bigint;

-- name: ReserveCustomerContactPolicyReceipt :one
INSERT INTO public.customer_contact_policy_operation_receipts (
  operation, actor_scope, key_digest, payload_digest, created_at
)
VALUES (
  sqlc.arg(operation)::text,
  sqlc.arg(actor_scope)::text,
  sqlc.arg(key_digest)::bytea,
  sqlc.arg(payload_digest)::bytea,
  sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetCustomerContactPolicyReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM public.customer_contact_policy_operation_receipts
WHERE operation = sqlc.arg(operation)::text
  AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea
FOR UPDATE;

-- name: CompleteCustomerContactPolicyReceipt :one
UPDATE public.customer_contact_policy_operation_receipts
SET state = 'completed',
    result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'reserved'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;
