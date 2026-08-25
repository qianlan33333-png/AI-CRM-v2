-- +goose Up
-- This table stores verified local WeCom ingress facts only. It never stores
-- provider credentials or represents provider delivery/success.
CREATE TABLE wecom_contact_inbox (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source            TEXT NOT NULL,
  source_key        TEXT NOT NULL,
  corp_id           TEXT NOT NULL,
  event_type        TEXT NOT NULL,
  external_userid   TEXT NOT NULL DEFAULT '',
  raw_payload       BYTEA NOT NULL,
  payload_digest    TEXT NOT NULL,
  occurred_at       TIMESTAMPTZ NOT NULL,
  state             TEXT NOT NULL DEFAULT 'pending',
  attempt_count     INTEGER NOT NULL DEFAULT 0,
  lease_fence       BIGINT NOT NULL DEFAULT 0,
  lease_owner       TEXT NOT NULL DEFAULT '',
  lease_expires_at  TIMESTAMPTZ,
  river_job_id      BIGINT,
  last_error        TEXT NOT NULL DEFAULT '',
  processed_at      TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT wecom_contact_inbox_source CHECK (source IN ('callback_inbox', 'directory_sync')),
  CONSTRAINT wecom_contact_inbox_source_key CHECK (
    btrim(source_key) = source_key AND char_length(source_key) BETWEEN 16 AND 512
  ),
  CONSTRAINT wecom_contact_inbox_corp CHECK (
    btrim(corp_id) = corp_id AND char_length(corp_id) BETWEEN 1 AND 256
  ),
  CONSTRAINT wecom_contact_inbox_event CHECK (
    btrim(event_type) = event_type AND char_length(event_type) BETWEEN 1 AND 256
  ),
  CONSTRAINT wecom_contact_inbox_external_user CHECK (char_length(external_userid) <= 1024),
  CONSTRAINT wecom_contact_inbox_payload CHECK (octet_length(raw_payload) BETWEEN 1 AND 1048576),
  CONSTRAINT wecom_contact_inbox_digest CHECK (payload_digest ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT wecom_contact_inbox_state CHECK (
    state IN ('pending', 'processing', 'processed', 'pending_identity', 'conflict', 'failed', 'skipped')
  ),
  CONSTRAINT wecom_contact_inbox_attempts CHECK (attempt_count >= 0),
  CONSTRAINT wecom_contact_inbox_fence CHECK (lease_fence >= 0),
  CONSTRAINT wecom_contact_inbox_lease CHECK (
    (state = 'processing' AND lease_fence > 0 AND lease_owner <> '' AND lease_expires_at IS NOT NULL)
    OR state <> 'processing'
  ),
  CONSTRAINT wecom_contact_inbox_processed CHECK (
    (state IN ('processed', 'pending_identity', 'conflict', 'skipped') AND processed_at IS NOT NULL)
    OR state IN ('pending', 'processing', 'failed')
  ),
  CONSTRAINT wecom_contact_inbox_river CHECK (river_job_id IS NULL OR river_job_id > 0),
  UNIQUE (source, source_key)
);

CREATE INDEX wecom_contact_inbox_state_idx
  ON wecom_contact_inbox (state, updated_at ASC, id ASC);
CREATE INDEX wecom_contact_inbox_external_user_idx
  ON wecom_contact_inbox (corp_id, external_userid, occurred_at DESC, id DESC)
  WHERE external_userid <> '';

-- +goose Down
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM wecom_contact_inbox LIMIT 1) THEN
    RAISE EXCEPTION 'cannot roll back populated WeCom contact inbox' USING ERRCODE = '55000';
  END IF;
END $$;
DROP TABLE wecom_contact_inbox;
