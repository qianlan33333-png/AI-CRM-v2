-- name: FindAdminUserForVerifiedLogin :one
SELECT u.id, u.role, COALESCE(u.staff_id, 0)::bigint AS staff_id, u.session_version
FROM admin_users AS u
WHERE u.auth_provider = sqlc.arg(auth_provider)
  AND u.wecom_corp_id = sqlc.arg(wecom_corp_id)
  AND u.provider_subject_id = sqlc.arg(provider_subject_id)
  AND u.is_active
  AND u.login_enabled;

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
SELECT u.id, u.role, COALESCE(u.staff_id, 0)::bigint AS staff_id
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

-- name: InsertAdminOAuthState :exec
WITH expired AS (
  DELETE FROM admin_oauth_states
  WHERE expires_at <= sqlc.arg(created_at)
  RETURNING state_hash
)
INSERT INTO admin_oauth_states (
  state_hash, auth_provider, next_path, created_at, expires_at
) VALUES (
  sqlc.arg(state_hash), sqlc.arg(auth_provider), sqlc.arg(next_path),
  sqlc.arg(created_at), sqlc.arg(expires_at)
);

-- name: ClaimAdminOAuthState :one
DELETE FROM admin_oauth_states
WHERE state_hash = sqlc.arg(state_hash)
  AND auth_provider = sqlc.arg(auth_provider)
  AND expires_at > sqlc.arg(claimed_at)
RETURNING next_path;

-- name: ListAdminAccessMembers :many
SELECT u.id, u.display_name, u.role, u.staff_id, u.is_active, u.login_enabled,
       COALESCE(s.wecom_userid, '') AS staff_wecom_userid,
       COALESCE(s.name, '') AS staff_name
FROM admin_users AS u
LEFT JOIN staff AS s ON s.id = u.staff_id
ORDER BY lower(u.display_name), u.id;

-- name: SaveAdminAccessMember :one
WITH updated AS (
  UPDATE admin_users
  SET login_enabled = sqlc.arg(login_enabled),
      session_version = CASE
        WHEN login_enabled IS DISTINCT FROM sqlc.arg(login_enabled) THEN session_version + 1
        ELSE session_version
      END,
      updated_at = CASE
        WHEN login_enabled IS DISTINCT FROM sqlc.arg(login_enabled) THEN sqlc.arg(updated_at)
        ELSE updated_at
      END
  WHERE id = sqlc.arg(id)
    AND is_active
  RETURNING id, login_enabled
)
SELECT id, login_enabled FROM updated;
