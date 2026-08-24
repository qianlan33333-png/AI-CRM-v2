-- name: UpsertHistoricalImportStaff :one
INSERT INTO staff (wecom_userid, name, is_active, created_at, updated_at)
VALUES (
  sqlc.arg(wecom_userid)::text,
  sqlc.arg(name)::text,
  sqlc.arg(is_active)::boolean,
  sqlc.arg(created_at)::timestamptz,
  sqlc.arg(updated_at)::timestamptz
)
ON CONFLICT (wecom_userid) DO UPDATE
SET name = EXCLUDED.name,
    is_active = EXCLUDED.is_active,
    updated_at = GREATEST(staff.updated_at, EXCLUDED.updated_at)
RETURNING id;
