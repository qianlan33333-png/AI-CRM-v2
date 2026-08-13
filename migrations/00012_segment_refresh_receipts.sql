-- +goose Up
CREATE TABLE segment_refresh_receipts (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  idempotency_scope TEXT NOT NULL,
  idempotency_key   TEXT NOT NULL,
  segment_id        BIGINT NOT NULL REFERENCES segments(id),
  state             TEXT NOT NULL DEFAULT 'reserved',
  river_job_id      BIGINT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at       TIMESTAMPTZ,
  CONSTRAINT segment_refresh_receipts_scope CHECK (
    btrim(idempotency_scope) = idempotency_scope
    AND idempotency_scope <> ''
    AND char_length(idempotency_scope) <= 200
  ),
  CONSTRAINT segment_refresh_receipts_key CHECK (
    btrim(idempotency_key) = idempotency_key
    AND char_length(idempotency_key) BETWEEN 16 AND 128
  ),
  CONSTRAINT segment_refresh_receipts_state CHECK (state IN ('reserved', 'accepted')),
  CONSTRAINT segment_refresh_receipts_acceptance CHECK (
    (state = 'reserved' AND river_job_id IS NULL AND accepted_at IS NULL)
    OR
    (state = 'accepted' AND river_job_id IS NOT NULL AND river_job_id > 0 AND accepted_at IS NOT NULL)
  ),
  UNIQUE (idempotency_scope, idempotency_key)
);

-- +goose Down
DROP TABLE segment_refresh_receipts;
