-- +goose Up
CREATE TABLE hxc_sender_config_receipts (
  id BIGSERIAL PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN ('save','reorder','archive')),
  actor TEXT NOT NULL CHECK (actor = btrim(actor) AND actor <> ''),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  result JSONB,
  state TEXT NOT NULL CHECK (state IN ('in_progress','completed')),
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  UNIQUE (operation, actor, key_digest),
  CHECK ((state = 'in_progress' AND result IS NULL AND completed_at IS NULL) OR
         (state = 'completed' AND result IS NOT NULL AND completed_at IS NOT NULL))
);

-- +goose Down
DROP TABLE hxc_sender_config_receipts;
