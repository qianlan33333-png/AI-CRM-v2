-- +goose Up
CREATE TABLE IF NOT EXISTS outbound_send_attempt_history (
  id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  send_attempt_id       BIGINT NOT NULL REFERENCES outbound_send_attempts(id),
  river_attempt         INTEGER NOT NULL,
  river_max_attempts    INTEGER NOT NULL,
  state                 TEXT NOT NULL DEFAULT 'reserved',
  failure_kind          TEXT,
  provider_code         TEXT,
  provider_message_id   TEXT,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  dispatch_started_at   TIMESTAMPTZ,
  completed_at          TIMESTAMPTZ,
  CONSTRAINT outbound_send_attempt_history_attempt CHECK (
    river_attempt > 0 AND river_max_attempts >= river_attempt
  ),
  CONSTRAINT outbound_send_attempt_history_state CHECK (
    state IN ('reserved', 'dispatching', 'succeeded', 'retryable_failed', 'final_failed', 'outcome_unknown')
  ),
  CONSTRAINT outbound_send_attempt_history_failure_kind CHECK (
    failure_kind IS NULL OR failure_kind IN (
      'timeout', 'connection', 'no_response_5xx', 'rate_limited', 'temporary',
      'invalid_argument', 'recipient_unavailable', 'adapter_error',
      'invalid_provider_result', 'interrupted_dispatch'
    )
  ),
  CONSTRAINT outbound_send_attempt_history_provider_code CHECK (
    provider_code IS NULL OR (
      btrim(provider_code) = provider_code AND provider_code <> '' AND char_length(provider_code) <= 200
    )
  ),
  CONSTRAINT outbound_send_attempt_history_provider_message CHECK (
    provider_message_id IS NULL OR (
      btrim(provider_message_id) = provider_message_id
      AND provider_message_id <> ''
      AND char_length(provider_message_id) <= 500
    )
  ),
  CONSTRAINT outbound_send_attempt_history_lifecycle CHECK (
    (state = 'reserved' AND dispatch_started_at IS NULL AND completed_at IS NULL
      AND failure_kind IS NULL AND provider_code IS NULL AND provider_message_id IS NULL)
    OR
    (state = 'dispatching' AND dispatch_started_at IS NOT NULL AND completed_at IS NULL
      AND failure_kind IS NULL AND provider_code IS NULL AND provider_message_id IS NULL)
    OR
    (state = 'succeeded' AND dispatch_started_at IS NOT NULL AND completed_at IS NOT NULL
      AND failure_kind IS NULL AND provider_message_id IS NOT NULL)
    OR
    (state IN ('retryable_failed', 'final_failed', 'outcome_unknown')
      AND dispatch_started_at IS NOT NULL AND completed_at IS NOT NULL
      AND failure_kind IS NOT NULL AND provider_code IS NOT NULL AND provider_message_id IS NULL)
  ),
  CONSTRAINT outbound_send_attempt_history_attempt_unique UNIQUE (send_attempt_id, river_attempt)
);

CREATE INDEX IF NOT EXISTS outbound_send_attempt_history_marker_idx
  ON outbound_send_attempt_history (send_attempt_id, river_attempt DESC);

-- +goose Down
-- O6A is an expand/contract compatibility migration. The O5 marker table and
-- its UNIQUE(river_job_id) contract remain unchanged, while attempt 1+2 history
-- must survive an application rollback and a later re-upgrade.
SELECT 1;
