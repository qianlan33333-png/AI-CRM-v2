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

-- name: LookupMessageArchiveUnionIDCustomers :many
SELECT DISTINCT i.customer_id
FROM identities AS i
JOIN customers AS c ON c.id = i.customer_id
WHERE i.kind = 'unionid'
  AND i.normalized_value = sqlc.arg(normalized_value)::text
  AND i.customer_id IS NOT NULL
  AND c.is_deleted = FALSE
ORDER BY i.customer_id
LIMIT 2;

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

-- name: LoadCustomerMergeAudit :one
SELECT primary_customer_id, policy_version
FROM customer_merges
WHERE id = sqlc.arg(merge_audit_id)::bigint;

-- name: LoadBindReceipt :one
SELECT
  payload_hmac,
  state,
  result_status,
  result_customer_id,
  result_merge_audit_id,
  result_pending_event_id,
  result_policy_version
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
  result_merge_audit_id = sqlc.narg(result_merge_audit_id)::bigint,
  result_pending_event_id = sqlc.narg(result_pending_event_id)::bigint,
  result_policy_version = sqlc.narg(result_policy_version)::text,
  completed_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'in_progress';

-- name: ReserveIngestReceipt :one
INSERT INTO identity_operation_receipts (
  operation,
  idempotency_scope,
  key_digest,
  command_schema_version,
  payload_hmac,
  payload_hmac_key_version,
  result_schema_version
) VALUES (
  'ingest',
  'identity.ingest.v1',
  sqlc.arg(key_digest)::bytea,
  1,
  sqlc.arg(payload_hmac)::bytea,
  1,
  1
)
ON CONFLICT (operation, idempotency_scope, key_digest) DO NOTHING
RETURNING id;

-- name: LoadIngestReceipt :one
SELECT
  payload_hmac,
  state,
  result_status,
  result_customer_id,
  result_pending_event_id,
  result_event_id,
  result_policy_version
FROM identity_operation_receipts
WHERE operation = 'ingest'
  AND idempotency_scope = 'identity.ingest.v1'
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteIngestReceipt :execrows
UPDATE identity_operation_receipts
SET
  state = 'completed',
  result_status = sqlc.arg(result_status)::text,
  result_customer_id = sqlc.narg(result_customer_id)::bigint,
  result_pending_event_id = sqlc.narg(result_pending_event_id)::bigint,
  result_event_id = sqlc.narg(result_event_id)::bigint,
  result_policy_version = 'identity_ingest_attribution_v1',
  completed_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'in_progress';

-- name: LoadPendingIngest :one
SELECT kind
FROM pending_events
WHERE id = sqlc.arg(pending_event_id)::bigint
  AND kind IN ('attribution', 'conflict')
  AND payload IS NOT NULL
  AND jsonb_typeof(payload) = 'object'
  AND policy_version = 'identity_ingest_attribution_v1'
  AND version >= 1;

-- name: InsertPendingIngest :one
INSERT INTO pending_events (
  kind,
  identity_ids,
  candidate_customer_ids,
  event_type,
  payload,
  source,
  idempotency_key,
  occurred_at,
  policy_version
) VALUES (
  sqlc.arg(kind)::text,
  sqlc.arg(identity_ids)::bigint[],
  '{}',
  sqlc.arg(event_type)::text,
  sqlc.arg(payload)::jsonb,
  sqlc.arg(source)::text,
  sqlc.arg(idempotency_key)::text,
  sqlc.arg(occurred_at)::timestamptz,
  'identity_ingest_attribution_v1'
)
RETURNING id;

-- name: ClaimPendingReplay :one
SELECT id, kind, identity_ids, event_type, payload, source, idempotency_key, occurred_at, version
FROM pending_events
WHERE state = 'pending'
  AND kind IN ('attribution', 'conflict')
  AND payload IS NOT NULL
  AND jsonb_typeof(payload) = 'object'
  AND policy_version = 'identity_ingest_attribution_v1'
ORDER BY version, id
FOR UPDATE SKIP LOCKED
LIMIT 1;

-- name: LockPendingReplayIdentities :many
SELECT id, kind, scope, normalized_value, normalizer_version
FROM identities
WHERE id = ANY(sqlc.arg(identity_ids)::bigint[])
ORDER BY id
FOR UPDATE;

-- name: CompletePendingReplay :execrows
UPDATE pending_events
SET state = 'replayed', version = version + 1, resolved_at = now()
WHERE id = sqlc.arg(pending_event_id)::bigint
  AND version = sqlc.arg(expected_version)::bigint
  AND state = 'pending'
  AND kind IN ('attribution', 'conflict')
  AND policy_version = 'identity_ingest_attribution_v1';

-- name: DeferPendingReplay :execrows
UPDATE pending_events
SET version = version + 1
WHERE id = sqlc.arg(pending_event_id)::bigint
  AND version = sqlc.arg(expected_version)::bigint
  AND state = 'pending'
  AND kind IN ('attribution', 'conflict')
  AND policy_version = 'identity_ingest_attribution_v1';

-- name: LockActiveBindCustomer :one
SELECT id
FROM customers
WHERE id = sqlc.arg(customer_id)::bigint
  AND is_deleted = FALSE
FOR UPDATE;

-- name: LockActiveBindCustomersForMerge :many
SELECT id
FROM customers
WHERE id = ANY(sqlc.arg(customer_ids)::bigint[])
  AND is_deleted = FALSE
ORDER BY id
FOR UPDATE;

-- name: HasVerifiedWeComIdentityForBindCustomer :one
SELECT EXISTS (
  SELECT 1
  FROM identities
  WHERE customer_id = sqlc.arg(customer_id)::bigint
    AND kind = 'wecom_external_userid'
    AND assurance = 'verified'
) AS has_verified_wecom_identity;

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

-- name: InsertVerifiedPhoneMergeReview :one
INSERT INTO pending_events (
  kind,
  identity_ids,
  candidate_customer_ids,
  source,
  policy_version,
  review_fingerprint,
  fingerprint_key_version
) VALUES (
  'merge_review',
  sqlc.arg(identity_ids)::bigint[],
  sqlc.arg(candidate_customer_ids)::bigint[],
  'identity.bind',
  'verified_phone_manual_review_v1',
  sqlc.arg(review_fingerprint)::bytea,
  1
)
RETURNING id;

-- name: LoadBindMergeReview :one
SELECT id
FROM pending_events
WHERE id = sqlc.arg(review_id)::bigint
  AND kind = 'merge_review'
  AND policy_version = 'verified_phone_manual_review_v1'
  AND cardinality(identity_ids) = 1
  AND cardinality(candidate_customer_ids) = 2
  AND candidate_customer_ids[1] < candidate_customer_ids[2]
  AND review_fingerprint IS NOT NULL
  AND octet_length(review_fingerprint) = 16
  AND fingerprint_key_version = 1
  AND version >= 1;

-- name: RebindIdentitiesForCustomerMerge :execrows
UPDATE identities
SET customer_id = sqlc.arg(primary_customer_id)::bigint
WHERE customer_id = sqlc.arg(merged_customer_id)::bigint;

-- name: InsertAutoCustomerMergeAudit :one
INSERT INTO customer_merges (
  primary_customer_id,
  merged_customer_id,
  mode,
  policy_version,
  review_fingerprint,
  fingerprint_key_version,
  operated_by,
  detail
) VALUES (
  sqlc.arg(primary_customer_id)::bigint,
  sqlc.arg(merged_customer_id)::bigint,
  'auto',
  sqlc.arg(policy_version)::text,
  sqlc.arg(review_fingerprint)::bytea,
  sqlc.arg(fingerprint_key_version)::smallint,
  sqlc.arg(operated_by)::text,
  sqlc.arg(detail)::jsonb
)
RETURNING id;

-- name: ListPendingMergeReviews :many
SELECT
  pending.id,
  pending.state,
  identity_row.id AS identity_id,
  identity_row.kind,
  identity_row.scope,
  identity_row.normalized_value,
  pending.review_fingerprint,
  pending.fingerprint_key_version,
  pending.candidate_customer_ids,
  pending.policy_version,
  pending.version,
  pending.created_at,
  pending.resolved_at
FROM pending_events AS pending
JOIN identities AS identity_row
  ON identity_row.id = pending.identity_ids[1]
WHERE pending.kind = 'merge_review'
  AND pending.state = 'pending'
  AND cardinality(pending.identity_ids) = 1
  AND pending.id > sqlc.arg(after_id)::bigint
ORDER BY pending.id
LIMIT sqlc.arg(page_limit)::int;

-- name: LockMergeReview :one
SELECT
  pending.id,
  pending.state,
  identity_row.id AS identity_id,
  identity_row.kind,
  identity_row.scope,
  identity_row.normalized_value,
  pending.review_fingerprint,
  pending.fingerprint_key_version,
  pending.candidate_customer_ids,
  pending.policy_version,
  pending.version,
  pending.created_at,
  pending.resolved_at
FROM pending_events AS pending
JOIN identities AS identity_row
  ON identity_row.id = pending.identity_ids[1]
WHERE pending.id = sqlc.arg(review_id)::bigint
  AND pending.kind = 'merge_review'
  AND cardinality(pending.identity_ids) = 1
FOR UPDATE OF pending, identity_row;

-- name: LockActiveMergeReviewCustomers :many
SELECT id
FROM customers
WHERE id = ANY(sqlc.arg(customer_ids)::bigint[])
  AND is_deleted = FALSE
ORDER BY id
FOR UPDATE;

-- name: ReserveMergeReviewReceipt :one
INSERT INTO identity_operation_receipts (
  operation,
  idempotency_scope,
  key_digest,
  command_schema_version,
  payload_hmac,
  payload_hmac_key_version,
  result_schema_version
) VALUES (
  sqlc.arg(operation)::text,
  sqlc.arg(operation)::text || '.v1',
  sqlc.arg(key_digest)::bytea,
  1,
  sqlc.arg(payload_hmac)::bytea,
  1,
  1
)
ON CONFLICT (operation, idempotency_scope, key_digest) DO NOTHING
RETURNING id;

-- name: LoadMergeReviewReceipt :one
SELECT payload_hmac, state, result_status, result_pending_event_id
FROM identity_operation_receipts
WHERE operation = sqlc.arg(operation)::text
  AND idempotency_scope = sqlc.arg(operation)::text || '.v1'
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: ResolveMergeReview :execrows
UPDATE pending_events
SET state = sqlc.arg(result_status)::text,
    version = version + 1,
    resolved_at = now()
WHERE id = sqlc.arg(review_id)::bigint
  AND kind = 'merge_review'
  AND state = 'pending'
  AND version = sqlc.arg(expected_version)::bigint;

-- name: CompleteMergeReviewReceipt :execrows
UPDATE identity_operation_receipts
SET state = 'completed',
    result_status = sqlc.arg(result_status)::text,
    result_customer_id = sqlc.narg(result_customer_id)::bigint,
    result_merge_audit_id = sqlc.narg(result_merge_audit_id)::bigint,
    result_pending_event_id = sqlc.arg(review_id)::bigint,
    result_policy_version = sqlc.arg(policy_version)::text,
    completed_at = now()
WHERE id = sqlc.arg(receipt_id)::bigint
  AND state = 'in_progress';

-- name: InsertManualCustomerMergeAudit :one
INSERT INTO customer_merges (
  primary_customer_id,
  merged_customer_id,
  mode,
  policy_version,
  review_fingerprint,
  fingerprint_key_version,
  operated_by,
  detail
) VALUES (
  sqlc.arg(primary_customer_id)::bigint,
  sqlc.arg(merged_customer_id)::bigint,
  'manual',
  sqlc.arg(policy_version)::text,
  sqlc.arg(review_fingerprint)::bytea,
  sqlc.arg(fingerprint_key_version)::smallint,
  sqlc.arg(operated_by)::text,
  sqlc.arg(detail)::jsonb
)
RETURNING id;
