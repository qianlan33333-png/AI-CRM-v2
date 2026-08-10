-- +goose Up
CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      JSONB NOT NULL,
  updated_by TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE settings_audit (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  key        TEXT NOT NULL,
  old_value  JSONB,
  new_value  JSONB NOT NULL,
  updated_by TEXT NOT NULL,
  request_id TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT uq_settings_audit_request_id UNIQUE (request_id)
);

CREATE INDEX idx_settings_audit_key_time ON settings_audit (key, updated_at DESC, id DESC);

-- +goose Down
DROP TABLE settings_audit;
DROP TABLE settings;
