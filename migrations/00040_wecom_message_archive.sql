-- +goose Up
CREATE TABLE wecom_message_archive_records (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_message_id TEXT NOT NULL,
  customer_id       BIGINT NOT NULL,
  external_userid   TEXT NOT NULL,
  chat_type         TEXT NOT NULL,
  owner_userid      TEXT NOT NULL DEFAULT '',
  sender            TEXT NOT NULL DEFAULT '',
  receiver          TEXT NOT NULL DEFAULT '',
  chat_id           TEXT NOT NULL DEFAULT '',
  roomid            TEXT NOT NULL DEFAULT '',
  group_name        TEXT NOT NULL DEFAULT '',
  message_type      TEXT NOT NULL,
  content_masked    TEXT NOT NULL,
  sent_at           TIMESTAMPTZ NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT wecom_message_archive_records_source_message CHECK (
    btrim(source_message_id) = source_message_id
    AND char_length(source_message_id) BETWEEN 1 AND 512
  ),
  CONSTRAINT wecom_message_archive_records_customer CHECK (customer_id > 0),
  CONSTRAINT wecom_message_archive_records_external_user CHECK (
    btrim(external_userid) = external_userid
    AND char_length(external_userid) BETWEEN 1 AND 1024
  ),
  CONSTRAINT wecom_message_archive_records_chat_type CHECK (chat_type IN ('private', 'group')),
  CONSTRAINT wecom_message_archive_records_message_type CHECK (
    btrim(message_type) = message_type
    AND char_length(message_type) BETWEEN 1 AND 128
  ),
  CONSTRAINT wecom_message_archive_records_text_limits CHECK (
    char_length(owner_userid) <= 256
    AND char_length(sender) <= 1024
    AND char_length(receiver) <= 1024
    AND char_length(chat_id) <= 1024
    AND char_length(roomid) <= 1024
    AND char_length(group_name) <= 512
  ),
  UNIQUE (source_message_id)
);

CREATE INDEX wecom_message_archive_records_customer_sent_idx
  ON wecom_message_archive_records (customer_id, sent_at DESC, id DESC);

CREATE TABLE wecom_message_archive_sync_receipts (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  idempotency_scope TEXT NOT NULL,
  idempotency_key   TEXT NOT NULL,
  request_digest    BYTEA NOT NULL,
  state             TEXT NOT NULL DEFAULT 'reserved',
  accepted_event_id BIGINT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at       TIMESTAMPTZ,
  CONSTRAINT wecom_message_archive_sync_receipts_scope CHECK (
    btrim(idempotency_scope) = idempotency_scope
    AND char_length(idempotency_scope) BETWEEN 1 AND 200
  ),
  CONSTRAINT wecom_message_archive_sync_receipts_key CHECK (
    btrim(idempotency_key) = idempotency_key
    AND char_length(idempotency_key) BETWEEN 16 AND 128
  ),
  CONSTRAINT wecom_message_archive_sync_receipts_digest CHECK (octet_length(request_digest) = 32),
  CONSTRAINT wecom_message_archive_sync_receipts_state CHECK (state IN ('reserved', 'accepted')),
  CONSTRAINT wecom_message_archive_sync_receipts_acceptance CHECK (
    (state = 'reserved' AND accepted_event_id IS NULL AND accepted_at IS NULL)
    OR
    (state = 'accepted' AND accepted_event_id IS NOT NULL AND accepted_event_id > 0 AND accepted_at IS NOT NULL)
  ),
  UNIQUE (idempotency_scope, idempotency_key)
);

-- +goose Down
DROP TABLE wecom_message_archive_sync_receipts;
DROP TABLE wecom_message_archive_records;
