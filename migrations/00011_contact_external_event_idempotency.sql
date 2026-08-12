-- +goose Up
CREATE TABLE customer_event_idempotency (
  idempotency_key   TEXT PRIMARY KEY,
  event_occurred_at TIMESTAMPTZ NOT NULL,
  event_id          BIGINT NOT NULL,
  event_customer_id BIGINT NOT NULL REFERENCES customers(id),
  event_type        TEXT NOT NULL,
  payload           JSONB NOT NULL,
  actor             TEXT NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT customer_event_idempotency_event_fk
    FOREIGN KEY (event_occurred_at, event_id)
    REFERENCES customer_events (occurred_at, id),
  CONSTRAINT customer_event_idempotency_event_unique
    UNIQUE (event_occurred_at, event_id),
  CONSTRAINT customer_event_idempotency_key CHECK (
    btrim(idempotency_key) = idempotency_key
    AND idempotency_key <> ''
    AND char_length(idempotency_key) <= 200
  ),
  CONSTRAINT customer_event_idempotency_type CHECK (
    btrim(event_type) = event_type
    AND event_type <> ''
    AND char_length(event_type) <= 200
  ),
  CONSTRAINT customer_event_idempotency_payload CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT customer_event_idempotency_actor CHECK (
    btrim(actor) = actor
    AND actor <> ''
    AND char_length(actor) <= 200
  )
);

-- +goose Down
DROP TABLE customer_event_idempotency;
