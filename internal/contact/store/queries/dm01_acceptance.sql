-- name: ResetDM01AcceptanceFixture :exec
TRUNCATE legacy_contact_identity_import_receipts,
  legacy_contact_identity_historical_archives,
  legacy_contact_identity_import_quarantines,
  legacy_contact_identity_import_row_receipts,
  legacy_contact_identity_import_checkpoints,
  legacy_contact_identity_source_mappings,
  legacy_contact_identity_import_runs,
  customers, staff RESTART IDENTITY CASCADE;

-- name: DeleteDM01AcceptanceArchives :exec
TRUNCATE legacy_contact_identity_historical_archives;

-- name: EditDM01AcceptanceCustomerName :exec
UPDATE customers SET name = sqlc.arg(name)::text;

-- name: CreateDM01AcceptanceStaleRun :one
INSERT INTO legacy_contact_identity_import_runs(
  source_manifest_sha256, source_repository_sha, snapshot_id, mode,
  upper_watermark, hmac_key_version, state, lease_token_hmac,
  lease_generation, lease_expires_at
) VALUES (
  sqlc.arg(manifest_digest)::bytea, sqlc.arg(repository_sha)::text,
  sqlc.arg(snapshot_id)::text, 'full', sqlc.arg(upper_watermark)::timestamptz,
  1, 'preflighted', sqlc.arg(token_hmac)::bytea,
  sqlc.arg(generation)::bigint, sqlc.arg(expires_at)::timestamptz
) RETURNING id;

-- name: SetDM01AcceptanceRunState :exec
UPDATE legacy_contact_identity_import_runs
SET state = sqlc.arg(state)::text
WHERE id = sqlc.arg(run_id)::bigint;

-- name: CreateDM01AcceptanceExpiredImportingRun :one
INSERT INTO legacy_contact_identity_import_runs(
  source_manifest_sha256, source_repository_sha, snapshot_id, mode,
  upper_watermark, hmac_key_version, state, lease_token_hmac,
  lease_generation, lease_expires_at
) VALUES (
  sqlc.arg(manifest_digest)::bytea, sqlc.arg(repository_sha)::text,
  sqlc.arg(snapshot_id)::text, 'full', sqlc.arg(upper_watermark)::timestamptz,
  1, 'importing', sqlc.arg(token_hmac)::bytea, 1, now() - interval '1 hour'
) RETURNING id;
