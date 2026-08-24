-- name: InsertHistoricalImportStaff :one
INSERT INTO staff (wecom_userid, name, is_active, created_at, updated_at)
VALUES (
  sqlc.arg(wecom_userid)::text,
  sqlc.arg(name)::text,
  sqlc.arg(is_active)::boolean,
  sqlc.arg(created_at)::timestamptz,
  sqlc.arg(updated_at)::timestamptz
)
ON CONFLICT (wecom_userid) DO NOTHING
RETURNING id;

-- name: InsertHistoricalImportStaffMapping :one
INSERT INTO legacy_contact_identity_source_mappings (
  source_table, source_key_hmac, staff_id, first_run_id, last_run_id, payload_hmac
) VALUES (
  'owner_role_map', sqlc.arg(source_key_hmac)::bytea, sqlc.arg(staff_id)::bigint,
  sqlc.arg(run_id)::bigint, sqlc.arg(run_id)::bigint, sqlc.arg(payload_hmac)::bytea
)
ON CONFLICT DO NOTHING
RETURNING staff_id;

-- name: LockHistoricalImportStaffForMatch :one
SELECT id, name, is_active, created_at, updated_at
FROM staff
WHERE wecom_userid = sqlc.arg(wecom_userid)::text
FOR UPDATE;

-- name: ClaimHistoricalImportLease :one
UPDATE legacy_contact_identity_import_runs
SET lease_token_hmac = sqlc.arg(new_token_hmac)::bytea,
    lease_generation = lease_generation + 1,
    lease_expires_at = sqlc.arg(lease_expires_at)::timestamptz
WHERE id = sqlc.arg(run_id)::bigint
  AND lease_generation = sqlc.arg(expected_generation)::bigint
  AND (lease_token_hmac IS NULL OR lease_expires_at < now())
RETURNING lease_generation;

-- name: RenewHistoricalImportLease :one
UPDATE legacy_contact_identity_import_runs
SET lease_expires_at = sqlc.arg(lease_expires_at)::timestamptz
WHERE id = sqlc.arg(run_id)::bigint
  AND lease_generation = sqlc.arg(expected_generation)::bigint
  AND lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND lease_expires_at >= now()
RETURNING lease_generation;

-- name: TransitionHistoricalImportRun :one
UPDATE legacy_contact_identity_import_runs
SET state = sqlc.arg(next_state)::text,
    completed_at = CASE WHEN sqlc.arg(next_state)::text IN ('preflighted', 'imported', 'reconciled', 'failed') THEN now() ELSE completed_at END
WHERE id = sqlc.arg(run_id)::bigint
  AND lease_generation = sqlc.arg(expected_generation)::bigint
  AND lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND lease_expires_at >= now()
RETURNING lease_generation;

-- name: AssertHistoricalImportLease :one
SELECT id
FROM legacy_contact_identity_import_runs
WHERE id = sqlc.arg(run_id)::bigint
  AND lease_generation = sqlc.arg(expected_generation)::bigint
  AND lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND lease_expires_at >= now()
FOR SHARE;

-- name: LockUniqueActiveStaffForHistoricalImport :one
SELECT id
FROM staff
WHERE wecom_userid = sqlc.arg(wecom_userid)::text AND is_active
FOR SHARE;

-- name: CreateHistoricalImportCustomer :one
INSERT INTO customers (name, avatar_url, gender, owner_staff_id, added_at, last_interact_at, created_at, updated_at)
VALUES (
  sqlc.arg(name)::text, sqlc.narg(avatar_url)::text, sqlc.narg(gender)::smallint,
  sqlc.narg(owner_staff_id)::bigint, sqlc.arg(first_seen_at)::timestamptz,
  sqlc.arg(last_seen_at)::timestamptz, sqlc.arg(created_at)::timestamptz,
  sqlc.arg(updated_at)::timestamptz
)
RETURNING id;

-- name: InsertHistoricalImportCustomerMapping :exec
INSERT INTO legacy_contact_identity_source_mappings (
  source_table, source_key_hmac, customer_id, first_run_id, last_run_id, payload_hmac
) VALUES (
  'crm_user_identity', sqlc.arg(source_key_hmac)::bytea, sqlc.arg(customer_id)::bigint,
  sqlc.arg(run_id)::bigint, sqlc.arg(run_id)::bigint, sqlc.arg(payload_hmac)::bytea
);

-- name: InsertHistoricalImportIdentityMapping :exec
INSERT INTO legacy_contact_identity_source_mappings (
  source_table, source_key_hmac, identity_id, first_run_id, last_run_id, payload_hmac
) VALUES (
  'wecom_external_contact_identity_map', sqlc.arg(source_key_hmac)::bytea,
  sqlc.arg(identity_id)::bigint, sqlc.arg(run_id)::bigint, sqlc.arg(run_id)::bigint,
  sqlc.arg(payload_hmac)::bytea
);

-- name: LockHistoricalImportSource :exec
SELECT pg_advisory_xact_lock(hashtextextended(
  sqlc.arg(source_table)::text || ':' || encode(sqlc.arg(source_key_hmac)::bytea, 'hex'), 0
));

-- name: FindHistoricalImportRowReceipt :one
SELECT payload_hmac, field_digest, disposition
FROM legacy_contact_identity_import_row_receipts
WHERE run_id = sqlc.arg(run_id)::bigint
  AND source_table = sqlc.arg(source_table)::text
  AND source_key_hmac = sqlc.arg(source_key_hmac)::bytea
FOR UPDATE;

-- name: LockHistoricalImportLineage :one
SELECT staff_id, customer_id, identity_id, payload_hmac
FROM legacy_contact_identity_source_mappings
WHERE source_table = sqlc.arg(source_table)::text
  AND source_key_hmac = sqlc.arg(source_key_hmac)::bytea
FOR UPDATE;

-- name: LockHistoricalImportCustomerForMatch :one
SELECT name, avatar_url, gender, owner_staff_id, added_at, last_interact_at, created_at, updated_at
FROM customers
WHERE id = sqlc.arg(customer_id)::bigint AND NOT is_deleted
FOR UPDATE;

-- name: IsHistoricalImportActiveStaff :one
SELECT is_active FROM staff WHERE id = sqlc.arg(staff_id)::bigint FOR SHARE;

-- name: LockHistoricalImportCustomerRoot :one
SELECT TRUE AS found
FROM customers
WHERE id = sqlc.arg(customer_id)::bigint AND NOT is_deleted
FOR SHARE;

-- name: AppendHistoricalImportLineage :execrows
INSERT INTO legacy_contact_identity_source_mappings (
  source_table, source_key_hmac, staff_id, customer_id, identity_id,
  first_run_id, last_run_id, payload_hmac
) VALUES (
  sqlc.arg(source_table)::text, sqlc.arg(source_key_hmac)::bytea,
  sqlc.narg(staff_id)::bigint, sqlc.narg(customer_id)::bigint,
  sqlc.narg(identity_id)::bigint, sqlc.arg(run_id)::bigint,
  sqlc.arg(run_id)::bigint, sqlc.arg(payload_hmac)::bytea
);

-- name: AppendHistoricalImportQuarantine :execrows
INSERT INTO legacy_contact_identity_import_quarantines (
  run_id, source_table, source_key_hmac, reason_code, payload_hmac, field_digest
) VALUES (
  sqlc.arg(run_id)::bigint, sqlc.arg(source_table)::text,
  sqlc.arg(source_key_hmac)::bytea, sqlc.arg(reason_code)::text,
  sqlc.arg(payload_hmac)::bytea, sqlc.arg(field_digest)::bytea
);

-- name: AppendHistoricalImportRowReceipt :execrows
INSERT INTO legacy_contact_identity_import_row_receipts (
  run_id, source_table, source_key_hmac, payload_hmac, field_digest, disposition
) VALUES (
  sqlc.arg(run_id)::bigint, sqlc.arg(source_table)::text,
  sqlc.arg(source_key_hmac)::bytea, sqlc.arg(payload_hmac)::bytea,
  sqlc.arg(field_digest)::bytea, sqlc.arg(disposition)::text
);

-- name: FindHistoricalImportArchive :one
SELECT payload_hmac, field_digest, archive_nonce, archive_ciphertext, archive_key_version
FROM legacy_contact_identity_historical_archives
WHERE run_id = sqlc.arg(run_id)::bigint
  AND source_table = sqlc.arg(source_table)::text
  AND source_key_hmac = sqlc.arg(source_key_hmac)::bytea
FOR UPDATE;

-- name: FindHistoricalImportQuarantine :one
SELECT payload_hmac, field_digest, reason_code
FROM legacy_contact_identity_import_quarantines
WHERE run_id = sqlc.arg(run_id)::bigint
  AND source_table = sqlc.arg(source_table)::text
  AND source_key_hmac = sqlc.arg(source_key_hmac)::bytea
  AND reason_code = sqlc.arg(reason_code)::text
FOR UPDATE;

-- name: AppendHistoricalImportArchive :execrows
INSERT INTO legacy_contact_identity_historical_archives (
  run_id, source_table, source_key_hmac, payload_hmac, field_digest,
  archive_nonce, archive_ciphertext, archive_key_version
) VALUES (
  sqlc.arg(run_id)::bigint, sqlc.arg(source_table)::text,
  sqlc.arg(source_key_hmac)::bytea, sqlc.arg(payload_hmac)::bytea,
  sqlc.arg(field_digest)::bytea, sqlc.arg(archive_nonce)::bytea,
  sqlc.arg(archive_ciphertext)::bytea, sqlc.arg(archive_key_version)::smallint
);

-- name: AppendHistoricalImportRowReceiptFenced :execrows
INSERT INTO legacy_contact_identity_import_row_receipts (
  run_id, source_table, source_key_hmac, payload_hmac, field_digest, disposition
)
SELECT r.id, sqlc.arg(source_table)::text, sqlc.arg(source_key_hmac)::bytea,
       sqlc.arg(payload_hmac)::bytea, sqlc.arg(field_digest)::bytea,
       sqlc.arg(disposition)::text
FROM legacy_contact_identity_import_runs AS r
WHERE r.id = sqlc.arg(run_id)::bigint
  AND r.lease_generation = sqlc.arg(expected_generation)::bigint
  AND r.lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND r.lease_expires_at >= now()
  AND r.state = 'importing'
  AND r.mode IN ('full', 'incremental');
