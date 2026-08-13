-- +goose Up
ALTER TABLE outbound_tasks
  ADD COLUMN status TEXT NOT NULL DEFAULT 'pending',
  ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN current_attempt_id BIGINT REFERENCES outbound_send_attempts(id),
  ADD COLUMN last_failure_kind TEXT,
  ADD COLUMN last_error TEXT,
  ADD COLUMN provider_message_id TEXT,
  ADD COLUMN sent_at TIMESTAMPTZ,
  ADD COLUMN status_updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD CONSTRAINT outbound_tasks_status CHECK (
    status IN ('pending', 'sending', 'sent', 'retryable_failed', 'final_failed', 'outcome_unknown', 'cancelled')
  ),
  ADD CONSTRAINT outbound_tasks_attempt_count CHECK (attempt_count >= 0),
  ADD CONSTRAINT outbound_tasks_last_failure_kind CHECK (
    last_failure_kind IS NULL OR last_failure_kind IN (
      'timeout', 'connection', 'no_response_5xx', 'rate_limited', 'temporary',
      'invalid_argument', 'recipient_unavailable', 'adapter_error',
      'invalid_provider_result', 'interrupted_dispatch'
    )
  ),
  ADD CONSTRAINT outbound_tasks_last_error CHECK (
    last_error IS NULL OR (
      btrim(last_error) = last_error AND last_error <> '' AND char_length(last_error) <= 200
    )
  ),
  ADD CONSTRAINT outbound_tasks_provider_message CHECK (
    provider_message_id IS NULL OR (
      btrim(provider_message_id) = provider_message_id
      AND provider_message_id <> ''
      AND char_length(provider_message_id) <= 500
    )
  ),
  ADD CONSTRAINT outbound_tasks_status_lifecycle CHECK (
    (status = 'pending' AND attempt_count = 0 AND current_attempt_id IS NULL
      AND last_failure_kind IS NULL AND last_error IS NULL
      AND provider_message_id IS NULL AND sent_at IS NULL)
    OR
    (status = 'sending' AND attempt_count > 0 AND current_attempt_id IS NOT NULL
      AND last_failure_kind IS NULL AND last_error IS NULL
      AND provider_message_id IS NULL AND sent_at IS NULL)
    OR
    (status = 'sent' AND attempt_count > 0 AND current_attempt_id IS NOT NULL
      AND last_failure_kind IS NULL AND last_error IS NULL
      AND provider_message_id IS NOT NULL AND sent_at IS NOT NULL)
    OR
    (status IN ('retryable_failed', 'final_failed', 'outcome_unknown')
      AND attempt_count > 0 AND current_attempt_id IS NOT NULL
      AND last_failure_kind IS NOT NULL AND last_error IS NOT NULL
      AND provider_message_id IS NULL AND sent_at IS NULL)
    OR
    (status = 'cancelled' AND provider_message_id IS NULL AND sent_at IS NULL)
  );

CREATE INDEX outbound_tasks_status_idx ON outbound_tasks (status, id);

-- +goose Down
DROP INDEX outbound_tasks_status_idx;
ALTER TABLE outbound_tasks
  DROP CONSTRAINT outbound_tasks_status_lifecycle,
  DROP CONSTRAINT outbound_tasks_provider_message,
  DROP CONSTRAINT outbound_tasks_last_error,
  DROP CONSTRAINT outbound_tasks_last_failure_kind,
  DROP CONSTRAINT outbound_tasks_attempt_count,
  DROP CONSTRAINT outbound_tasks_status,
  DROP COLUMN status_updated_at,
  DROP COLUMN sent_at,
  DROP COLUMN provider_message_id,
  DROP COLUMN last_error,
  DROP COLUMN last_failure_kind,
  DROP COLUMN current_attempt_id,
  DROP COLUMN attempt_count,
  DROP COLUMN status;
