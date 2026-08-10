-- +goose Up
CREATE TABLE event_log (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_type      TEXT NOT NULL,
  customer_id     BIGINT,
  payload         JSONB NOT NULL DEFAULT '{}',
  occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  idempotency_key TEXT NOT NULL,
  dispatched      BOOLEAN NOT NULL DEFAULT FALSE,
  CONSTRAINT uq_event_log_idempotency_key UNIQUE (idempotency_key)
);

CREATE INDEX idx_el_undispatched ON event_log (id) WHERE NOT dispatched;

-- +goose Down
DROP TABLE event_log;
