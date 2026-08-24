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
