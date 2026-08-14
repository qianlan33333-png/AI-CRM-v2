-- +goose Up
CREATE TABLE IF NOT EXISTS event_deliveries (
  event_id         BIGINT NOT NULL REFERENCES event_log(id),
  consumer         TEXT NOT NULL,
  status           TEXT NOT NULL DEFAULT 'pending',
  attempt_count    INTEGER NOT NULL DEFAULT 0,
  river_job_id     BIGINT,
  lease_owner      TEXT,
  lease_expires_at TIMESTAMPTZ,
  last_error_code  TEXT,
  completed_at     TIMESTAMPTZ,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (event_id, consumer),
  CONSTRAINT event_deliveries_consumer CHECK (
    btrim(consumer) = consumer AND char_length(consumer) BETWEEN 1 AND 200
  ),
  CONSTRAINT event_deliveries_status CHECK (
    status IN ('pending', 'processing', 'completed', 'final_failed', 'outcome_unknown')
  ),
  CONSTRAINT event_deliveries_attempt_count CHECK (attempt_count >= 0),
  CONSTRAINT event_deliveries_river_job CHECK (river_job_id IS NULL OR river_job_id > 0),
  CONSTRAINT event_deliveries_error_code CHECK (
    last_error_code IS NULL OR (
      btrim(last_error_code) = last_error_code AND char_length(last_error_code) BETWEEN 1 AND 100
    )
  ),
  CONSTRAINT event_deliveries_state_shape CHECK (
    (status = 'pending' AND lease_owner IS NULL AND lease_expires_at IS NULL AND completed_at IS NULL)
    OR
    (status = 'processing' AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL AND completed_at IS NULL)
    OR
    (status IN ('completed', 'final_failed', 'outcome_unknown')
      AND lease_owner IS NULL AND lease_expires_at IS NULL AND completed_at IS NOT NULL)
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS event_deliveries_river_job_uq
  ON event_deliveries (river_job_id)
  WHERE river_job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS event_deliveries_consumer_state_idx
  ON event_deliveries (consumer, status, lease_expires_at, event_id);

CREATE TABLE IF NOT EXISTS automation_trigger_receipts (
  id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id           BIGINT NOT NULL,
  consumer           TEXT NOT NULL,
  customer_id        BIGINT NOT NULL,
  tag_id             BIGINT NOT NULL,
  actor              TEXT NOT NULL,
  state              TEXT NOT NULL DEFAULT 'reserved',
  triggered_event_id BIGINT,
  triggered_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at       TIMESTAMPTZ,
  CONSTRAINT automation_trigger_receipts_consumer CHECK (consumer = 'automation.tag-trigger.v1'),
  CONSTRAINT automation_trigger_receipts_ids CHECK (event_id > 0 AND customer_id > 0 AND tag_id > 0),
  CONSTRAINT automation_trigger_receipts_actor CHECK (
    btrim(actor) = actor AND char_length(actor) BETWEEN 1 AND 200
  ),
  CONSTRAINT automation_trigger_receipts_state CHECK (state IN ('reserved', 'triggered')),
  CONSTRAINT automation_trigger_receipts_state_shape CHECK (
    (state = 'reserved' AND triggered_event_id IS NULL AND completed_at IS NULL)
    OR
    (state = 'triggered' AND triggered_event_id > 0 AND completed_at IS NOT NULL)
  ),
  UNIQUE (event_id, consumer),
  UNIQUE (triggered_event_id)
);

CREATE INDEX IF NOT EXISTS automation_trigger_receipts_list_idx
  ON automation_trigger_receipts (triggered_at DESC, id DESC)
  INCLUDE (event_id, customer_id, tag_id, state);

-- +goose Down
-- D01 receipts and per-consumer delivery outcomes are immutable business facts.
-- Application rollback must preserve them for the next forward replay.
SELECT 1;
