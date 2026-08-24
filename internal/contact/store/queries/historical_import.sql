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
