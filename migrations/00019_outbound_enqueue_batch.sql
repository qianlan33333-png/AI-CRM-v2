-- +goose Up
CREATE TABLE outbound_batches (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  idempotency_scope TEXT NOT NULL,
  idempotency_key   TEXT NOT NULL,
  tier              TEXT NOT NULL,
  recipient_digest  BYTEA NOT NULL,
  recipient_count   INTEGER NOT NULL,
  template_key      TEXT NOT NULL,
  payload           JSONB NOT NULL,
  accepted_event_id BIGINT REFERENCES event_log(id),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at       TIMESTAMPTZ,
  CONSTRAINT outbound_batches_scope CHECK (
    btrim(idempotency_scope) = idempotency_scope
    AND idempotency_scope <> ''
    AND char_length(idempotency_scope) <= 200
  ),
  CONSTRAINT outbound_batches_key CHECK (
    btrim(idempotency_key) = idempotency_key
    AND char_length(idempotency_key) BETWEEN 16 AND 128
  ),
  CONSTRAINT outbound_batches_tier CHECK (tier IN ('S', 'M', 'L')),
  CONSTRAINT outbound_batches_digest CHECK (octet_length(recipient_digest) = 32),
  CONSTRAINT outbound_batches_recipient_count CHECK (recipient_count BETWEEN 1 AND 200000),
  CONSTRAINT outbound_batches_template_key CHECK (template_key = 'text.notice.v1'),
  CONSTRAINT outbound_batches_payload_object CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT outbound_batches_acceptance CHECK (
    (accepted_event_id IS NULL AND accepted_at IS NULL)
    OR (accepted_event_id IS NOT NULL AND accepted_at IS NOT NULL)
  ),
  UNIQUE (idempotency_scope, idempotency_key)
);

CREATE TABLE outbound_batch_chunks (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  batch_id        BIGINT NOT NULL REFERENCES outbound_batches(id),
  chunk_index     INTEGER NOT NULL,
  recipient_start INTEGER NOT NULL,
  recipient_count INTEGER NOT NULL,
  state           TEXT NOT NULL DEFAULT 'reserved',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  expanded_at     TIMESTAMPTZ,
  CONSTRAINT outbound_batch_chunks_index CHECK (chunk_index >= 0),
  CONSTRAINT outbound_batch_chunks_start CHECK (recipient_start >= 0),
  CONSTRAINT outbound_batch_chunks_count CHECK (recipient_count BETWEEN 1 AND 10000),
  CONSTRAINT outbound_batch_chunks_state CHECK (state IN ('reserved', 'expanded')),
  CONSTRAINT outbound_batch_chunks_expansion CHECK (
    (state = 'reserved' AND expanded_at IS NULL)
    OR (state = 'expanded' AND expanded_at IS NOT NULL)
  ),
  UNIQUE (batch_id, chunk_index)
);

ALTER TABLE outbound_tasks
  ADD COLUMN batch_id BIGINT REFERENCES outbound_batches(id),
  ADD COLUMN batch_chunk_index INTEGER,
  ADD CONSTRAINT outbound_tasks_batch_pair CHECK (
    (batch_id IS NULL AND batch_chunk_index IS NULL)
    OR (batch_id IS NOT NULL AND batch_chunk_index >= 0)
  );

CREATE UNIQUE INDEX outbound_tasks_batch_customer_unique
  ON outbound_tasks (batch_id, customer_id)
  WHERE batch_id IS NOT NULL;

-- +goose Down
DROP INDEX outbound_tasks_batch_customer_unique;
ALTER TABLE outbound_tasks
  DROP CONSTRAINT outbound_tasks_batch_pair,
  DROP COLUMN batch_chunk_index,
  DROP COLUMN batch_id;
DROP TABLE outbound_batch_chunks;
DROP TABLE outbound_batches;
