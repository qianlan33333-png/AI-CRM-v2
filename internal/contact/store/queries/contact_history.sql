-- name: CreateHistoricalSidebarProfile :one
INSERT INTO contact_v1_sidebar_profile_history (
  source_key_digest, customer_id, source, industry, industry_description,
  needs_blockers_followup, updated_at, source_payload_digest
) VALUES (
  sqlc.arg(source_key_digest)::bytea, sqlc.narg(customer_id)::bigint,
  sqlc.arg(source)::text, sqlc.arg(industry)::text, sqlc.arg(industry_description)::text,
  sqlc.arg(needs_blockers_followup)::text, sqlc.arg(updated_at)::timestamptz,
  sqlc.arg(source_payload_digest)::bytea
)
RETURNING *;

-- name: GetHistoricalSidebarProfile :one
SELECT * FROM contact_v1_sidebar_profile_history WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalSidebarProfiles :many
SELECT * FROM contact_v1_sidebar_profile_history
WHERE sqlc.narg(customer_id)::bigint IS NULL OR customer_id=sqlc.narg(customer_id)::bigint
ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalSidebarProfiles :one
SELECT count(*)::bigint FROM contact_v1_sidebar_profile_history
WHERE sqlc.narg(customer_id)::bigint IS NULL OR customer_id=sqlc.narg(customer_id)::bigint;

-- name: CreateHistoricalOwnerMigrationResult :one
INSERT INTO contact_v1_owner_migration_result_history (
  source_key_digest, scope_type, file_hash, preview_hash, total_rows, eligible_count,
  wecom_success, wecom_failed, crm_updated, include_wecom_transfer,
  transfer_welcome_message, session_relation, preview_relation, created_at,
  executed_at, source_payload_digest
) VALUES (
  sqlc.arg(source_key_digest)::bytea, sqlc.arg(scope_type)::text,
  sqlc.arg(file_hash)::text, sqlc.arg(preview_hash)::text,
  sqlc.arg(total_rows)::bigint, sqlc.arg(eligible_count)::bigint,
  sqlc.arg(wecom_success)::bigint, sqlc.arg(wecom_failed)::bigint,
  sqlc.arg(crm_updated)::bigint, sqlc.arg(include_wecom_transfer)::boolean,
  sqlc.arg(transfer_welcome_message)::text, sqlc.arg(session_relation)::text,
  sqlc.arg(preview_relation)::text, sqlc.arg(created_at)::timestamptz,
  sqlc.arg(executed_at)::timestamptz, sqlc.arg(source_payload_digest)::bytea
)
RETURNING *;

-- name: GetHistoricalOwnerMigrationResult :one
SELECT * FROM contact_v1_owner_migration_result_history WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalOwnerMigrationResults :many
SELECT * FROM contact_v1_owner_migration_result_history
ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalOwnerMigrationResults :one
SELECT count(*)::bigint FROM contact_v1_owner_migration_result_history;
