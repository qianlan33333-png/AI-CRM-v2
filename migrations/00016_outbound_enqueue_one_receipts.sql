-- +goose Up
CREATE TABLE outbound_enqueue_receipts (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  idempotency_scope TEXT NOT NULL,
  idempotency_key   TEXT NOT NULL,
  customer_id       BIGINT NOT NULL REFERENCES customers(id),
  template_key      TEXT NOT NULL,
  payload           JSONB NOT NULL,
  state             TEXT NOT NULL DEFAULT 'reserved',
  task_id           BIGINT REFERENCES outbound_tasks(id),
  event_id          BIGINT REFERENCES event_log(id),
  river_job_id      BIGINT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at       TIMESTAMPTZ,
  CONSTRAINT outbound_enqueue_receipts_scope CHECK (
    btrim(idempotency_scope) = idempotency_scope
    AND idempotency_scope <> ''
    AND char_length(idempotency_scope) <= 200
  ),
  CONSTRAINT outbound_enqueue_receipts_key CHECK (
    btrim(idempotency_key) = idempotency_key
    AND char_length(idempotency_key) BETWEEN 16 AND 128
  ),
  CONSTRAINT outbound_enqueue_receipts_template_key CHECK (template_key = 'text.notice.v1'),
  CONSTRAINT outbound_enqueue_receipts_payload_object CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT outbound_enqueue_receipts_state CHECK (state IN ('reserved', 'accepted')),
  CONSTRAINT outbound_enqueue_receipts_acceptance CHECK (
    (state = 'reserved' AND task_id IS NULL AND event_id IS NULL AND river_job_id IS NULL AND accepted_at IS NULL)
    OR
    (state = 'accepted' AND task_id IS NOT NULL AND event_id IS NOT NULL AND river_job_id IS NOT NULL AND river_job_id > 0 AND accepted_at IS NOT NULL)
  ),
  UNIQUE (idempotency_scope, idempotency_key)
);

-- +goose Down
DROP TABLE outbound_enqueue_receipts;
