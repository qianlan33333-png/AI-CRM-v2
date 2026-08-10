-- +goose Up
CREATE TABLE admin_users (
  id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  auth_provider       TEXT NOT NULL,
  provider_tenant_id  TEXT NOT NULL,
  provider_subject_id TEXT NOT NULL,
  display_name        TEXT NOT NULL,
  role                TEXT NOT NULL,
  staff_id            BIGINT,
  is_active           BOOLEAN NOT NULL DEFAULT TRUE,
  login_enabled       BOOLEAN NOT NULL DEFAULT TRUE,
  session_version     BIGINT NOT NULL DEFAULT 1,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ck_admin_users_provider CHECK (auth_provider = 'wecom'),
  CONSTRAINT ck_admin_users_role CHECK (role IN ('admin', 'ops', 'sales')),
  CONSTRAINT ck_admin_users_provider_tenant CHECK (length(provider_tenant_id) BETWEEN 1 AND 128),
  CONSTRAINT ck_admin_users_provider_subject CHECK (length(provider_subject_id) BETWEEN 1 AND 128),
  CONSTRAINT ck_admin_users_display_name CHECK (length(display_name) BETWEEN 1 AND 200),
  CONSTRAINT ck_admin_users_session_version CHECK (session_version > 0),
  CONSTRAINT uq_admin_users_provider_identity UNIQUE (
    auth_provider, provider_tenant_id, provider_subject_id
  )
);

CREATE TABLE admin_sessions (
  id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  session_token_hash BYTEA NOT NULL,
  csrf_token_hash    BYTEA NOT NULL,
  admin_user_id      BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  session_version    BIGINT NOT NULL,
  auth_time          TIMESTAMPTZ NOT NULL,
  expires_at         TIMESTAMPTZ NOT NULL,
  revoked_at         TIMESTAMPTZ,
  revoked_reason     TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_admin_sessions_token_hash UNIQUE (session_token_hash),
  CONSTRAINT ck_admin_sessions_token_hash CHECK (octet_length(session_token_hash) = 32),
  CONSTRAINT ck_admin_sessions_csrf_hash CHECK (octet_length(csrf_token_hash) = 32),
  CONSTRAINT ck_admin_sessions_version CHECK (session_version > 0),
  CONSTRAINT ck_admin_sessions_expiry CHECK (expires_at > auth_time),
  CONSTRAINT ck_admin_sessions_revocation CHECK (
    (revoked_at IS NULL AND revoked_reason = '') OR
    (revoked_at IS NOT NULL AND length(revoked_reason) BETWEEN 1 AND 64)
  )
);

CREATE INDEX idx_admin_sessions_user_active
  ON admin_sessions (admin_user_id, expires_at DESC)
  WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE admin_sessions;
DROP TABLE admin_users;
