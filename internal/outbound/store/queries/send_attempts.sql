-- name: ReserveOutboundSendAttempt :one
INSERT INTO outbound_send_attempts (river_job_id, task_id, job_kind)
VALUES ($1, $2, $3)
ON CONFLICT (river_job_id) DO UPDATE
SET river_job_id = EXCLUDED.river_job_id
RETURNING id, river_job_id, task_id, job_kind, state, failure_kind,
  provider_code, provider_message_id, dispatch_started_at, completed_at;

-- name: StartOutboundSendAttempt :one
WITH started AS (
  UPDATE outbound_send_attempts AS attempt
  SET state = 'dispatching', dispatch_started_at = now()
  WHERE attempt.id = $1 AND attempt.state = 'reserved'
  RETURNING attempt.id, attempt.river_job_id, attempt.task_id, attempt.job_kind, attempt.state, attempt.failure_kind,
    attempt.provider_code, attempt.provider_message_id, attempt.dispatch_started_at, attempt.completed_at, TRUE AS started
)
SELECT * FROM started
UNION ALL
SELECT attempt.id, attempt.river_job_id, attempt.task_id, attempt.job_kind, attempt.state, attempt.failure_kind,
  attempt.provider_code, attempt.provider_message_id, attempt.dispatch_started_at, attempt.completed_at, FALSE AS started
FROM outbound_send_attempts AS attempt
WHERE attempt.id = $1 AND NOT EXISTS (SELECT 1 FROM started)
LIMIT 1;

-- name: LoadOutboundSendRequest :one
SELECT id, customer_id, template_key, payload
FROM outbound_tasks
WHERE id = $1;

-- name: CompleteOutboundSendAttempt :one
WITH completed AS (
  UPDATE outbound_send_attempts AS attempt
  SET state = $2,
      failure_kind = NULLIF($3::text, ''),
      provider_code = NULLIF($4::text, ''),
      provider_message_id = NULLIF($5::text, ''),
      completed_at = now()
  WHERE attempt.id = $1 AND attempt.state = 'dispatching'
  RETURNING attempt.id, attempt.river_job_id, attempt.task_id, attempt.job_kind, attempt.state, attempt.failure_kind,
    attempt.provider_code, attempt.provider_message_id, attempt.dispatch_started_at, attempt.completed_at
)
SELECT * FROM completed
UNION ALL
SELECT attempt.id, attempt.river_job_id, attempt.task_id, attempt.job_kind, attempt.state, attempt.failure_kind,
  attempt.provider_code, attempt.provider_message_id, attempt.dispatch_started_at, attempt.completed_at
FROM outbound_send_attempts AS attempt
WHERE attempt.id = $1 AND NOT EXISTS (SELECT 1 FROM completed)
LIMIT 1;

-- name: MarkOutboundTaskSending :execrows
UPDATE outbound_tasks AS task
SET status = 'sending',
    attempt_count = CASE
      WHEN task.current_attempt_id = attempt.id THEN task.attempt_count
      ELSE task.attempt_count + 1
    END,
    current_attempt_id = attempt.id,
    last_failure_kind = NULL,
    last_error = NULL,
    provider_message_id = NULL,
    sent_at = NULL,
    status_updated_at = attempt.dispatch_started_at
FROM outbound_send_attempts AS attempt
WHERE attempt.id = sqlc.arg(attempt_id)
  AND attempt.task_id = task.id
  AND attempt.state = 'dispatching'
  AND (
    (task.status IN ('pending', 'retryable_failed') AND task.current_attempt_id IS DISTINCT FROM attempt.id)
    OR (task.status = 'sending' AND task.current_attempt_id = attempt.id)
  );

-- name: ProjectOutboundTaskResult :one
UPDATE outbound_tasks AS task
SET status = sqlc.arg(task_status),
    attempt_count = CASE
      WHEN task.current_attempt_id = attempt.id THEN task.attempt_count
      ELSE task.attempt_count + 1
    END,
    current_attempt_id = attempt.id,
    last_failure_kind = attempt.failure_kind,
    last_error = attempt.provider_code,
    provider_message_id = attempt.provider_message_id,
    sent_at = CASE WHEN attempt.state = 'succeeded' THEN attempt.completed_at ELSE NULL END,
    status_updated_at = attempt.completed_at
FROM outbound_send_attempts AS attempt
WHERE attempt.id = sqlc.arg(attempt_id)
  AND attempt.task_id = task.id
  AND attempt.state = sqlc.arg(attempt_state)
  AND attempt.completed_at IS NOT NULL
  AND (
    (task.current_attempt_id = attempt.id AND task.status IN ('sending', sqlc.arg(task_status)))
    OR (task.status = 'pending' AND task.current_attempt_id IS NULL)
  )
RETURNING task.id AS task_id, task.customer_id, task.status, task.attempt_count,
  task.last_failure_kind, task.last_error, task.provider_message_id,
  attempt.id AS attempt_id, attempt.river_job_id, attempt.completed_at;
