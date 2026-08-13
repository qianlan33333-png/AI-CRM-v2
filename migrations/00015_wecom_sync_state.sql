-- +goose Up
CREATE TABLE wecom_sync_state (
  sync_key     TEXT PRIMARY KEY,
  cursor       TEXT NOT NULL DEFAULT '',
  completed_at TIMESTAMPTZ,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT wecom_sync_state_key CHECK (
    btrim(sync_key) = sync_key
    AND char_length(sync_key) BETWEEN 1 AND 200
  ),
  CONSTRAINT wecom_sync_state_cursor CHECK (
    btrim(cursor) = cursor
    AND char_length(cursor) <= 512
  ),
  CONSTRAINT wecom_sync_state_completion CHECK (
    completed_at IS NULL OR cursor = ''
  )
);

-- +goose Down
DROP TABLE wecom_sync_state;
