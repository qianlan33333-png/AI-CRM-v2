-- name: LockSettingKey :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(key), 0));

-- name: GetSetting :one
SELECT key, value, updated_by, updated_at
FROM settings
WHERE key = sqlc.arg(key);

-- name: InsertSettingsAudit :one
INSERT INTO settings_audit (
  key, old_value, new_value, updated_by, request_id, updated_at
) VALUES (
  sqlc.arg(key), sqlc.narg(old_value)::jsonb, sqlc.arg(new_value)::jsonb,
  sqlc.arg(updated_by), sqlc.arg(request_id), sqlc.arg(updated_at)
)
ON CONFLICT (request_id) DO NOTHING
RETURNING id, key, old_value, new_value, updated_by, request_id, updated_at;

-- name: GetSettingsAuditByRequestID :one
SELECT id, key, old_value, new_value, updated_by, request_id, updated_at
FROM settings_audit
WHERE request_id = sqlc.arg(request_id);

-- name: UpsertSetting :one
INSERT INTO settings (key, value, updated_by, updated_at)
VALUES (sqlc.arg(key), sqlc.arg(value)::jsonb, sqlc.arg(updated_by), sqlc.arg(updated_at))
ON CONFLICT (key) DO UPDATE SET
  value = EXCLUDED.value,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at
RETURNING key, value, updated_by, updated_at;
