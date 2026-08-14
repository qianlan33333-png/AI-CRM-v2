-- +goose Up
CREATE TABLE admin_oauth_states (
  state_hash          BYTEA PRIMARY KEY,
  auth_provider       TEXT NOT NULL,
  next_path           TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL,
  expires_at          TIMESTAMPTZ NOT NULL,
  CONSTRAINT ck_admin_oauth_states_hash CHECK (octet_length(state_hash) = 32),
  CONSTRAINT ck_admin_oauth_states_provider CHECK (auth_provider = 'wecom'),
  CONSTRAINT ck_admin_oauth_states_next CHECK (
    length(next_path) BETWEEN 1 AND 2048
    AND left(next_path, 1) = '/'
    AND left(next_path, 2) <> '//'
    AND position(E'\\' IN next_path) = 0
    AND next_path !~ '[[:cntrl:]]'
  ),
  CONSTRAINT ck_admin_oauth_states_expiry CHECK (expires_at > created_at)
);

CREATE INDEX idx_admin_oauth_states_expiry
  ON admin_oauth_states (expires_at);

-- +goose Down
DROP TABLE admin_oauth_states;
