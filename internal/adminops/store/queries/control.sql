-- name: CreateCredential :one
INSERT INTO admin_ops_credentials (credential_kind, client_id, display_name, state, secret_ref, secret_mask, metadata, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
RETURNING *;

-- name: GetCredential :one
SELECT * FROM admin_ops_credentials WHERE credential_kind = $1 AND client_id = $2;

-- name: ListCredentials :many
SELECT * FROM admin_ops_credentials ORDER BY credential_kind, created_at DESC, id DESC;

-- name: UpdateCredential :one
UPDATE admin_ops_credentials
SET display_name = $3, state = $4, secret_ref = $5, secret_mask = $6, metadata = $7, version = version + 1, updated_at = $8
WHERE credential_kind = $1 AND client_id = $2
RETURNING *;

-- name: UpsertCategory :one
INSERT INTO admin_ops_config_categories (category_key, enabled, settings, updated_by, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (category_key) DO UPDATE
SET enabled = EXCLUDED.enabled, settings = EXCLUDED.settings, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at, version = admin_ops_config_categories.version + 1
RETURNING *;

-- name: GetCategory :one
SELECT * FROM admin_ops_config_categories WHERE category_key = $1;

-- name: ListCategories :many
SELECT * FROM admin_ops_config_categories ORDER BY category_key;

-- name: CreateRelease :one
INSERT INTO admin_ops_config_releases (state, changes, checksum, based_on_release_id, rollback_of_release_id, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetRelease :one
SELECT * FROM admin_ops_config_releases WHERE id = $1;

-- name: ListReleases :many
SELECT * FROM admin_ops_config_releases ORDER BY created_at DESC, id DESC LIMIT $1;

-- name: ValidateRelease :one
UPDATE admin_ops_config_releases SET state = 'validated', validated_at = $2 WHERE id = $1 AND state = 'draft' RETURNING *;

-- name: PublishRelease :one
UPDATE admin_ops_config_releases SET state = 'published', published_by = $2, published_at = $3 WHERE id = $1 AND state = 'validated' AND checksum = $4 RETURNING *;

-- name: CreateRollbackRelease :one
INSERT INTO admin_ops_config_releases (state, changes, checksum, based_on_release_id, rollback_of_release_id, created_by, created_at, validated_at, published_at, published_by)
SELECT 'rolled_back', changes, checksum, admin_ops_config_releases.id, admin_ops_config_releases.id, $2, $3, $3, $3, $2 FROM admin_ops_config_releases WHERE admin_ops_config_releases.id = $1 AND admin_ops_config_releases.state = 'published' RETURNING *;

-- name: CreateJob :one
INSERT INTO admin_ops_jobs (job_key, kind, state, target_ref, request_summary, requested_by, created_at, updated_at) VALUES ($1, $2, 'queued', $3, $4, $5, $6, $6) RETURNING *;

-- name: GetJob :one
SELECT * FROM admin_ops_jobs WHERE job_key = $1;

-- name: ListJobs :many
SELECT * FROM admin_ops_jobs WHERE ($1::text = '' OR kind = $1) AND ($2::text = '' OR state = $2) ORDER BY created_at DESC, id DESC LIMIT $3;

-- name: TransitionJob :one
UPDATE admin_ops_jobs SET state = $2, failure_code = $3, result_summary = $4, started_at = COALESCE(started_at, $5), completed_at = $6, updated_at = $5, version = version + 1 WHERE job_key = $1 AND version = $7 RETURNING *;

-- name: GetNotificationSetting :one
SELECT * FROM admin_ops_notification_settings WHERE channel = $1;

-- name: UpsertNotificationSetting :one
INSERT INTO admin_ops_notification_settings (channel, enabled, secret_ref, secret_mask, validation_state, updated_by, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (channel) DO UPDATE SET enabled = EXCLUDED.enabled, secret_ref = EXCLUDED.secret_ref, secret_mask = EXCLUDED.secret_mask, validation_state = EXCLUDED.validation_state, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at RETURNING *;

-- name: ReserveActionReceipt :one
INSERT INTO admin_ops_action_receipts (action, actor_scope, key_digest, payload_digest, created_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (action, actor_scope, key_digest) DO NOTHING RETURNING *;

-- name: GetActionReceipt :one
SELECT * FROM admin_ops_action_receipts WHERE action = $1 AND actor_scope = $2 AND key_digest = $3;

-- name: CompleteActionReceipt :one
UPDATE admin_ops_action_receipts SET state = 'completed', result_snapshot = $2, completed_at = $3 WHERE id = $1 AND state = 'in_progress' RETURNING *;
