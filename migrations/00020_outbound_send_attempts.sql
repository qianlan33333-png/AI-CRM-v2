-- +goose Up
CREATE TABLE outbound_send_attempts (
  id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  river_job_id        BIGINT NOT NULL UNIQUE,
  task_id             BIGINT NOT NULL REFERENCES outbound_tasks(id),
  job_kind            TEXT NOT NULL,
  state               TEXT NOT NULL DEFAULT 'reserved',
  failure_kind        TEXT,
  provider_code       TEXT,
  provider_message_id TEXT,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  dispatch_started_at TIMESTAMPTZ,
  completed_at        TIMESTAMPTZ,
  CONSTRAINT outbound_send_attempts_river_job CHECK (river_job_id > 0),
  CONSTRAINT outbound_send_attempts_job_kind CHECK (
    job_kind IN ('outbound_enqueue_one', 'outbound_enqueue_batch_task')
  ),
  CONSTRAINT outbound_send_attempts_state CHECK (
    state IN ('reserved', 'dispatching', 'succeeded', 'retryable_failed', 'final_failed', 'outcome_unknown')
  ),
  CONSTRAINT outbound_send_attempts_failure_kind CHECK (
    failure_kind IS NULL OR failure_kind IN (
      'timeout', 'connection', 'no_response_5xx', 'rate_limited', 'temporary',
      'invalid_argument', 'recipient_unavailable', 'adapter_error',
      'invalid_provider_result', 'interrupted_dispatch'
    )
  ),
  CONSTRAINT outbound_send_attempts_provider_code CHECK (
    provider_code IS NULL OR (
      btrim(provider_code) = provider_code AND provider_code <> '' AND char_length(provider_code) <= 200
    )
  ),
  CONSTRAINT outbound_send_attempts_provider_message CHECK (
    provider_message_id IS NULL OR (
      btrim(provider_message_id) = provider_message_id AND provider_message_id <> '' AND char_length(provider_message_id) <= 500
    )
  ),
  CONSTRAINT outbound_send_attempts_lifecycle CHECK (
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
  )
);

CREATE INDEX outbound_send_attempts_task_id_idx ON outbound_send_attempts (task_id);

-- +goose Down
DROP TABLE outbound_send_attempts;
