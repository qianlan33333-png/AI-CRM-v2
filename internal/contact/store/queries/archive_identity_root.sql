-- name: LockVerifiedDM01CustomerRoot :one
SELECT c.id
FROM legacy_contact_identity_source_mappings m
JOIN legacy_contact_identity_import_row_receipts r
  ON r.run_id = m.last_run_id AND r.source_table = m.source_table
 AND r.source_key_hmac = m.source_key_hmac AND r.payload_hmac = m.payload_hmac
JOIN legacy_contact_identity_import_runs run ON run.id = r.run_id
JOIN customers c ON c.id = m.customer_id AND NOT c.is_deleted
WHERE m.source_table = 'crm_user_identity'
  AND m.source_key_hmac = sqlc.arg(source_key_hmac)::bytea
  AND m.last_run_id = sqlc.arg(run_id)::bigint
  AND m.customer_id > 0 AND r.disposition = 'imported'
  AND octet_length(r.field_digest) = 32
  AND run.mode IN ('full', 'incremental') AND run.state = 'imported'
FOR SHARE OF m, r, run, c;

-- name: CountArchiveIdentityRootFixtureRows :one
SELECT (SELECT count(*) FROM legacy_contact_identity_source_mappings)::bigint AS mappings,
  (SELECT count(*) FROM legacy_contact_identity_import_row_receipts)::bigint AS receipts;

-- name: CreateArchiveIdentityRootFixture :one
WITH fixture_run AS (
  INSERT INTO legacy_contact_identity_import_runs
    (source_manifest_sha256, source_repository_sha, snapshot_id, mode, upper_watermark, hmac_key_version, state)
  VALUES (sqlc.arg(manifest)::bytea, sqlc.arg(repository_sha)::text, 'root-lock-test', 'full',
    sqlc.arg(watermark)::timestamptz, 1, 'imported') RETURNING id
), fixture_customer AS (
  INSERT INTO customers (name, is_deleted)
  VALUES ('dm01-root-lock-test', sqlc.arg(deleted)::boolean) RETURNING id
), fixture_mapping AS (
  INSERT INTO legacy_contact_identity_source_mappings
    (source_table, source_key_hmac, customer_id, first_run_id, last_run_id, payload_hmac)
  SELECT 'crm_user_identity', sqlc.arg(source_key)::bytea, c.id, r.id, r.id, sqlc.arg(payload)::bytea
  FROM fixture_run r, fixture_customer c
), fixture_receipt AS (
  INSERT INTO legacy_contact_identity_import_row_receipts
    (run_id, source_table, source_ordinal, source_key_hmac, payload_hmac, field_digest, disposition)
  SELECT r.id, 'crm_user_identity', 1, sqlc.arg(source_key)::bytea,
    sqlc.arg(receipt_payload)::bytea, sqlc.arg(field_digest)::bytea, 'imported' FROM fixture_run r
)
SELECT r.id AS run_id, c.id AS customer_id FROM fixture_run r, fixture_customer c;
