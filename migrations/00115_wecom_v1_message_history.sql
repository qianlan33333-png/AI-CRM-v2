-- +goose Up
-- This history is never read by the current message sync or dispatch workers.
CREATE TABLE wecom_v1_message_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL UNIQUE CHECK (source_id > 0),
  sequence BIGINT,
  customer_id BIGINT CHECK (customer_id > 0),
  chat_type TEXT NOT NULL,
  message_type TEXT NOT NULL,
  content_masked TEXT,
  original_send_time TEXT NOT NULL,
  send_time_basis TEXT NOT NULL CHECK (send_time_basis IN ('civil_unzoned', 'explicit_offset')),
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  CHECK ((send_time_basis = 'civil_unzoned' AND sent_at IS NULL)
      OR (send_time_basis = 'explicit_offset' AND sent_at IS NOT NULL))
);
CREATE INDEX wecom_v1_message_history_customer_idx ON wecom_v1_message_history (customer_id, id);
CREATE INDEX wecom_v1_message_history_chat_idx ON wecom_v1_message_history (chat_type, id);

-- +goose Down
DROP TABLE wecom_v1_message_history;
