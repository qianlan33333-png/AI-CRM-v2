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

-- name: GetDM01TargetDatabaseIdentity :one
SELECT system_identifier::text AS server_id, current_database()::text AS database
FROM pg_control_system();

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

-- name: ReserveHistoricalImportRun :one
WITH inserted AS (
INSERT INTO legacy_contact_identity_import_runs (
  source_manifest_sha256, source_repository_sha, snapshot_id, parent_run_id,
  mode, upper_watermark, hmac_key_version, state
) VALUES (
  sqlc.arg(source_manifest_sha256)::bytea, sqlc.arg(source_repository_sha)::text,
  sqlc.arg(snapshot_id)::text, sqlc.narg(parent_run_id)::bigint,
  sqlc.arg(mode)::text, sqlc.arg(upper_watermark)::timestamptz,
  sqlc.arg(hmac_key_version)::smallint, 'reserved'
)
ON CONFLICT (source_manifest_sha256, mode, upper_watermark) DO NOTHING
RETURNING id, lease_generation, state
)
SELECT id, lease_generation, state FROM inserted
UNION ALL
SELECT id, lease_generation, state
FROM legacy_contact_identity_import_runs
WHERE source_manifest_sha256 = sqlc.arg(source_manifest_sha256)::bytea
  AND mode = sqlc.arg(mode)::text
  AND upper_watermark = sqlc.arg(upper_watermark)::timestamptz
  AND source_repository_sha = sqlc.arg(source_repository_sha)::text
  AND snapshot_id = sqlc.arg(snapshot_id)::text
  AND parent_run_id IS NOT DISTINCT FROM sqlc.narg(parent_run_id)::bigint
  AND hmac_key_version = sqlc.arg(hmac_key_version)::smallint
  AND ((mode = 'preflight' AND state IN ('reserved','preflighted'))
       OR (mode IN ('full','incremental') AND state IN ('reserved','preflighted','importing','imported'))
       OR (mode = 'reconcile' AND state IN ('reserved','reconciling','reconciled')))
LIMIT 1;

-- name: ReadHistoricalImportRun :one
SELECT mode, state FROM legacy_contact_identity_import_runs
WHERE id = sqlc.arg(run_id)::bigint
FOR SHARE;

-- name: ReadHistoricalImportRunSnapshot :one
SELECT mode, state FROM legacy_contact_identity_import_runs
WHERE id = sqlc.arg(run_id)::bigint;

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

-- name: AppendHistoricalImportCheckpointFenced :execrows
INSERT INTO legacy_contact_identity_import_checkpoints (
  run_id, source_table, final_source_key_hmac, payload_hmac, field_digest,
  watermark, upper_source_key_hmac, upper_bound_empty
)
SELECT r.id, sqlc.arg(source_table)::text,
       sqlc.arg(final_source_key_hmac)::bytea, sqlc.arg(payload_hmac)::bytea,
       sqlc.arg(field_digest)::bytea, sqlc.narg(watermark)::timestamptz,
       sqlc.narg(upper_source_key_hmac)::bytea, sqlc.arg(upper_bound_empty)::boolean
FROM legacy_contact_identity_import_runs AS r
WHERE r.id = sqlc.arg(run_id)::bigint
  AND r.lease_generation = sqlc.arg(expected_generation)::bigint
  AND r.lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND r.lease_expires_at >= now()
  AND ((r.mode IN ('full','incremental') AND r.state = 'importing')
       OR (r.mode = 'reconcile' AND r.state = 'reconciling'))
ON CONFLICT (run_id, source_table) DO NOTHING;

-- name: FindHistoricalImportCheckpoint :one
SELECT final_source_key_hmac, payload_hmac, field_digest, watermark,
       upper_source_key_hmac, upper_bound_empty
FROM legacy_contact_identity_import_checkpoints
WHERE run_id = sqlc.arg(run_id)::bigint
  AND source_table = sqlc.arg(source_table)::text;

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
SELECT m.staff_id, m.customer_id, m.identity_id, m.payload_hmac,
       m.last_run_id, r.field_digest
FROM legacy_contact_identity_source_mappings AS m
JOIN legacy_contact_identity_import_row_receipts AS r
  ON r.run_id = m.last_run_id AND r.source_table = m.source_table
 AND r.source_key_hmac = m.source_key_hmac AND r.disposition = 'imported'
WHERE m.source_table = sqlc.arg(source_table)::text
  AND m.source_key_hmac = sqlc.arg(source_key_hmac)::bytea
FOR UPDATE;

-- name: LockHistoricalImportStaffTarget :one
SELECT wecom_userid, name, is_active, created_at, updated_at
FROM staff WHERE id = sqlc.arg(staff_id)::bigint FOR UPDATE;

-- name: LockHistoricalImportCustomerTarget :one
SELECT name, avatar_url, gender, owner_staff_id, added_at, last_interact_at,
       created_at, updated_at
FROM customers
WHERE id = sqlc.arg(customer_id)::bigint AND NOT is_deleted
FOR UPDATE;

-- name: LockHistoricalImportCustomerForMatch :one
SELECT name, avatar_url, gender, owner_staff_id, added_at, last_interact_at, created_at, updated_at
FROM customers
WHERE id = sqlc.arg(customer_id)::bigint AND NOT is_deleted
FOR UPDATE;

-- name: IsHistoricalImportActiveStaff :one
SELECT is_active FROM staff WHERE id = sqlc.arg(staff_id)::bigint FOR SHARE;

-- name: LockActiveStaffWeComUserID :one
SELECT wecom_userid
FROM staff
WHERE id = sqlc.arg(staff_id)::bigint
  AND is_active
  AND btrim(wecom_userid) <> ''
FOR SHARE;

-- name: GetActiveStaffWeComUserID :one
SELECT wecom_userid
FROM staff
WHERE id = sqlc.arg(staff_id)::bigint
  AND is_active
  AND btrim(wecom_userid) <> '';

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

-- name: UpdateHistoricalImportStaffCAS :execrows
UPDATE staff
SET wecom_userid = sqlc.arg(next_wecom_userid)::text,
    name = sqlc.arg(next_name)::text,
    is_active = sqlc.arg(next_is_active)::boolean,
    created_at = sqlc.arg(next_created_at)::timestamptz,
    updated_at = sqlc.arg(next_updated_at)::timestamptz
WHERE id = sqlc.arg(staff_id)::bigint
  AND wecom_userid = sqlc.arg(prior_wecom_userid)::text
  AND name = sqlc.arg(prior_name)::text
  AND is_active = sqlc.arg(prior_is_active)::boolean
  AND created_at = sqlc.arg(prior_created_at)::timestamptz
  AND updated_at = sqlc.arg(prior_updated_at)::timestamptz;

-- name: UpdateHistoricalImportCustomerCAS :execrows
UPDATE customers
SET name = sqlc.arg(next_name)::text,
    avatar_url = sqlc.narg(next_avatar_url)::text,
    gender = sqlc.narg(next_gender)::smallint,
    owner_staff_id = sqlc.narg(next_owner_staff_id)::bigint,
    added_at = sqlc.arg(next_first_seen_at)::timestamptz,
    last_interact_at = sqlc.arg(next_last_seen_at)::timestamptz,
    created_at = sqlc.arg(next_created_at)::timestamptz,
    updated_at = sqlc.arg(next_updated_at)::timestamptz
WHERE id = sqlc.arg(customer_id)::bigint AND NOT is_deleted
  AND name = sqlc.arg(prior_name)::text
  AND avatar_url IS NOT DISTINCT FROM sqlc.narg(prior_avatar_url)::text
  AND gender IS NOT DISTINCT FROM sqlc.narg(prior_gender)::smallint
  AND owner_staff_id IS NOT DISTINCT FROM sqlc.narg(prior_owner_staff_id)::bigint
  AND added_at = sqlc.arg(prior_first_seen_at)::timestamptz
  AND last_interact_at = sqlc.arg(prior_last_seen_at)::timestamptz
  AND created_at = sqlc.arg(prior_created_at)::timestamptz
  AND updated_at = sqlc.arg(prior_updated_at)::timestamptz;

-- name: UpdateHistoricalImportLineageCAS :execrows
UPDATE legacy_contact_identity_source_mappings
SET last_run_id = sqlc.arg(next_run_id)::bigint,
    payload_hmac = sqlc.arg(next_payload_hmac)::bytea,
    updated_at = now()
WHERE source_table = sqlc.arg(source_table)::text
  AND source_key_hmac = sqlc.arg(source_key_hmac)::bytea
  AND last_run_id = sqlc.arg(prior_run_id)::bigint
  AND payload_hmac = sqlc.arg(prior_payload_hmac)::bytea;

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
  run_id, source_table, source_ordinal, source_key_hmac, payload_hmac, field_digest, disposition
)
SELECT r.id, sqlc.arg(source_table)::text,
       (SELECT COALESCE(max(prior.source_ordinal), 0) + 1
        FROM legacy_contact_identity_import_row_receipts AS prior
        WHERE prior.run_id = r.id AND prior.source_table = sqlc.arg(source_table)::text),
       sqlc.arg(source_key_hmac)::bytea,
       sqlc.arg(payload_hmac)::bytea, sqlc.arg(field_digest)::bytea,
       sqlc.arg(disposition)::text
FROM legacy_contact_identity_import_runs AS r
WHERE r.id = sqlc.arg(run_id)::bigint
  AND r.lease_generation = sqlc.arg(expected_generation)::bigint
  AND r.lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND r.lease_expires_at >= now()
  AND r.state = 'importing'
  AND r.mode IN ('full', 'incremental');

-- name: LockHistoricalReconcileRun :one
SELECT child.parent_run_id
FROM legacy_contact_identity_import_runs AS child
JOIN legacy_contact_identity_import_runs AS parent ON parent.id = child.parent_run_id
WHERE child.id = sqlc.arg(run_id)::bigint
  AND child.mode = 'reconcile' AND child.state = 'reconciling'
  AND child.lease_generation = sqlc.arg(expected_generation)::bigint
  AND child.lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND child.lease_expires_at >= now()
  AND parent.mode IN ('full', 'incremental') AND parent.state = 'imported'
  AND parent.source_manifest_sha256 = child.source_manifest_sha256
  AND parent.snapshot_id = child.snapshot_id
FOR SHARE OF child, parent;

-- name: ListHistoricalReconcileReceiptsPage :many
SELECT r.source_ordinal, r.source_key_hmac, r.payload_hmac, r.field_digest, r.disposition,
       (SELECT count(*) FROM legacy_contact_identity_historical_archives AS a
        WHERE a.run_id = r.run_id AND a.source_table = r.source_table
          AND a.source_key_hmac = r.source_key_hmac)::bigint AS archive_count,
       (SELECT count(*) FROM legacy_contact_identity_import_quarantines AS q
        WHERE q.run_id = r.run_id AND q.source_table = r.source_table
          AND q.source_key_hmac = r.source_key_hmac)::bigint AS quarantine_count
FROM legacy_contact_identity_import_row_receipts AS r
WHERE r.run_id = sqlc.arg(parent_run_id)::bigint
  AND r.source_table = sqlc.arg(source_table)::text
  AND r.source_ordinal > sqlc.arg(after_source_ordinal)::bigint
ORDER BY r.source_ordinal
LIMIT sqlc.arg(page_size)::integer;

-- name: CountHistoricalReconcileCompanions :one
SELECT
  (SELECT count(*) FROM legacy_contact_identity_historical_archives
   WHERE run_id = sqlc.arg(parent_run_id)::bigint
     AND source_table = sqlc.arg(source_table)::text)::bigint AS archive_count,
  (SELECT count(*) FROM legacy_contact_identity_import_quarantines
   WHERE run_id = sqlc.arg(parent_run_id)::bigint
     AND source_table = sqlc.arg(source_table)::text)::bigint AS quarantine_count;

-- name: AppendHistoricalReconcileResult :execrows
INSERT INTO legacy_contact_identity_import_receipts (run_id, result_digest)
SELECT child.id, sqlc.arg(result_digest)::bytea
FROM legacy_contact_identity_import_runs AS child
JOIN legacy_contact_identity_import_runs AS parent ON parent.id = child.parent_run_id
WHERE child.id = sqlc.arg(run_id)::bigint
  AND child.mode = 'reconcile' AND child.state = 'reconciling'
  AND child.lease_generation = sqlc.arg(expected_generation)::bigint
  AND child.lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND child.lease_expires_at >= now()
  AND parent.mode IN ('full', 'incremental') AND parent.state = 'imported'
  AND parent.source_manifest_sha256 = child.source_manifest_sha256
  AND parent.snapshot_id = child.snapshot_id;

-- name: CompleteHistoricalReconcileRun :one
UPDATE legacy_contact_identity_import_runs
SET state = 'reconciled', completed_at = now()
WHERE id = sqlc.arg(run_id)::bigint
  AND mode = 'reconcile' AND state = 'reconciling'
  AND lease_generation = sqlc.arg(expected_generation)::bigint
  AND lease_token_hmac = sqlc.arg(token_hmac)::bytea
  AND lease_expires_at >= now()
  AND EXISTS (SELECT 1 FROM legacy_contact_identity_import_receipts AS receipt
              WHERE receipt.run_id = legacy_contact_identity_import_runs.id)
RETURNING id;
