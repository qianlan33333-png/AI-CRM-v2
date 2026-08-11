-- name: FindAdminUserForVerifiedLogin :one
SELECT id, role, staff_id, session_version
FROM admin_users
WHERE auth_provider = sqlc.arg(auth_provider)
  AND provider_tenant_id = sqlc.arg(provider_tenant_id)
  AND provider_subject_id = sqlc.arg(provider_subject_id)
  AND is_active
  AND login_enabled;

-- name: InsertAdminSession :exec
INSERT INTO admin_sessions (
  session_token_hash, csrf_token_hash, admin_user_id, session_version,
  auth_time, expires_at
) VALUES (
  sqlc.arg(session_token_hash), sqlc.arg(csrf_token_hash),
  sqlc.arg(admin_user_id), sqlc.arg(session_version),
  sqlc.arg(auth_time), sqlc.arg(expires_at)
);

-- name: GetActiveSession :one
SELECT u.id, u.role, u.staff_id
FROM admin_sessions AS s
JOIN admin_users AS u ON u.id = s.admin_user_id
WHERE s.session_token_hash = sqlc.arg(session_token_hash)
  AND s.revoked_at IS NULL
  AND s.expires_at > sqlc.arg(now)
  AND s.session_version = u.session_version
  AND u.is_active
  AND u.login_enabled;

-- name: RevokeSession :one
UPDATE admin_sessions
SET revoked_at = sqlc.arg(revoked_at), revoked_reason = 'logout'
WHERE session_token_hash = sqlc.arg(session_token_hash)
  AND csrf_token_hash = sqlc.arg(csrf_token_hash)
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg(revoked_at)
RETURNING id;

-- name: ValidateSessionCSRF :one
SELECT EXISTS (
  SELECT 1
  FROM admin_sessions AS s
  JOIN admin_users AS u ON u.id = s.admin_user_id
  WHERE s.session_token_hash = sqlc.arg(session_token_hash)
    AND s.csrf_token_hash = sqlc.arg(csrf_token_hash)
    AND s.revoked_at IS NULL
    AND s.expires_at > sqlc.arg(now)
    AND s.session_version = u.session_version
    AND u.is_active
    AND u.login_enabled
);
