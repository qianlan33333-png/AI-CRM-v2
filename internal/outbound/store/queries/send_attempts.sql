-- name: ReserveOutboundSendAttempt :one
INSERT INTO outbound_send_attempts (river_job_id, task_id, job_kind)
VALUES ($1, $2, $3)
ON CONFLICT (river_job_id) DO UPDATE
SET river_job_id = EXCLUDED.river_job_id
RETURNING id, river_job_id, task_id, job_kind, state, failure_kind,
  provider_code, provider_message_id, dispatch_started_at, completed_at;

-- name: BackfillOutboundFirstAttemptHistory :execrows
INSERT INTO outbound_send_attempt_history AS history (
  send_attempt_id, river_attempt, river_max_attempts, state, failure_kind,
  provider_code, provider_message_id, created_at, dispatch_started_at, completed_at
)
SELECT marker.id, 1, sqlc.arg(river_max_attempts)::integer, marker.state, marker.failure_kind,
  marker.provider_code, marker.provider_message_id, marker.created_at,
  marker.dispatch_started_at, marker.completed_at
FROM outbound_send_attempts AS marker
WHERE marker.id = sqlc.arg(send_attempt_id)::bigint
  AND marker.river_job_id = sqlc.arg(river_job_id)::bigint
  AND marker.task_id = sqlc.arg(task_id)::bigint
  AND marker.job_kind = sqlc.arg(job_kind)::text
ON CONFLICT (send_attempt_id, river_attempt) DO NOTHING;

-- name: ReserveOutboundAttemptHistory :one
INSERT INTO outbound_send_attempt_history AS history (
  send_attempt_id, river_attempt, river_max_attempts
)
SELECT marker.id, sqlc.arg(river_attempt)::integer, sqlc.arg(river_max_attempts)::integer
FROM outbound_send_attempts AS marker
WHERE marker.id = sqlc.arg(send_attempt_id)::bigint
  AND marker.river_job_id = sqlc.arg(river_job_id)::bigint
  AND marker.task_id = sqlc.arg(task_id)::bigint
  AND marker.job_kind = sqlc.arg(job_kind)::text
  AND (
    sqlc.arg(river_attempt)::integer = 1
    OR EXISTS (
      SELECT 1
      FROM outbound_send_attempt_history AS previous
      WHERE previous.send_attempt_id = marker.id
        AND previous.river_attempt = sqlc.arg(river_attempt)::integer - 1
        AND previous.state = 'retryable_failed'
    )
  )
ON CONFLICT (send_attempt_id, river_attempt) DO UPDATE
SET send_attempt_id = EXCLUDED.send_attempt_id
RETURNING history.id, history.send_attempt_id, history.river_attempt, history.river_max_attempts,
  history.state, history.failure_kind, history.provider_code, history.provider_message_id,
  history.dispatch_started_at, history.completed_at;

-- name: StartOutboundAttemptHistory :one
UPDATE outbound_send_attempt_history AS history
SET state = 'dispatching', dispatch_started_at = now()
WHERE history.id = sqlc.arg(history_id)::bigint
  AND history.send_attempt_id = sqlc.arg(send_attempt_id)::bigint
  AND history.river_attempt = sqlc.arg(river_attempt)::integer
  AND history.state = 'reserved'
RETURNING history.id, history.send_attempt_id, history.river_attempt, history.river_max_attempts,
  history.state, history.failure_kind, history.provider_code, history.provider_message_id,
  history.dispatch_started_at, history.completed_at;

-- name: LoadOutboundAttemptHistory :one
SELECT history.id, history.send_attempt_id, marker.river_job_id, marker.task_id, marker.job_kind,
  history.river_attempt, history.river_max_attempts, history.state, history.failure_kind,
  history.provider_code, history.provider_message_id, history.dispatch_started_at, history.completed_at
FROM outbound_send_attempt_history AS history
JOIN outbound_send_attempts AS marker ON marker.id = history.send_attempt_id
WHERE history.id = sqlc.arg(history_id)::bigint;

-- name: PrepareOutboundSendAttemptRetry :one
UPDATE outbound_send_attempts AS marker
SET state = 'reserved',
    failure_kind = NULL,
    provider_code = NULL,
    provider_message_id = NULL,
    dispatch_started_at = NULL,
    completed_at = NULL
FROM outbound_send_attempt_history AS history, outbound_tasks AS task
WHERE marker.id = sqlc.arg(send_attempt_id)::bigint
  AND history.id = sqlc.arg(history_id)::bigint
  AND history.send_attempt_id = marker.id
  AND history.river_attempt > 1
  AND history.state = 'dispatching'
  AND marker.state = 'retryable_failed'
  AND task.id = marker.task_id
  AND task.status = 'retryable_failed'
  AND task.current_attempt_id = marker.id
  AND task.attempt_count = history.river_attempt - 1
RETURNING marker.id, marker.river_job_id, marker.task_id, marker.job_kind, marker.state,
  marker.failure_kind, marker.provider_code, marker.provider_message_id,
  marker.dispatch_started_at, marker.completed_at;

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

-- name: CompleteOutboundAttemptHistory :one
UPDATE outbound_send_attempt_history AS history
SET state = sqlc.arg(attempt_state)::text,
    failure_kind = NULLIF(sqlc.arg(failure_kind)::text, ''),
    provider_code = NULLIF(sqlc.arg(provider_code)::text, ''),
    provider_message_id = NULLIF(sqlc.arg(provider_message_id)::text, ''),
    completed_at = now()
WHERE history.id = sqlc.arg(history_id)::bigint
  AND history.send_attempt_id = sqlc.arg(send_attempt_id)::bigint
  AND history.state = 'dispatching'
RETURNING history.id, history.send_attempt_id, history.river_attempt, history.river_max_attempts,
  history.state, history.failure_kind, history.provider_code, history.provider_message_id,
  history.dispatch_started_at, history.completed_at;

-- name: MarkOutboundTaskSending :execrows
UPDATE outbound_tasks AS task
SET status = 'sending',
    attempt_count = history.river_attempt,
    current_attempt_id = attempt.id,
    last_failure_kind = NULL,
    last_error = NULL,
    provider_message_id = NULL,
    sent_at = NULL,
    status_updated_at = attempt.dispatch_started_at
FROM outbound_send_attempts AS attempt
JOIN outbound_send_attempt_history AS history ON history.send_attempt_id = attempt.id
WHERE attempt.id = sqlc.arg(attempt_id)
  AND history.id = sqlc.arg(history_id)
  AND attempt.task_id = task.id
  AND attempt.state = 'dispatching'
  AND history.state = 'dispatching'
  AND (
    (history.river_attempt = 1 AND task.status = 'pending' AND task.attempt_count = 0 AND task.current_attempt_id IS NULL)
    OR (history.river_attempt = task.attempt_count + 1 AND task.status = 'retryable_failed' AND task.current_attempt_id = attempt.id)
    OR (history.river_attempt = task.attempt_count AND task.status = 'sending' AND task.current_attempt_id = attempt.id)
  );

-- name: ProjectOutboundTaskResult :execrows
UPDATE outbound_tasks AS task
SET status = sqlc.arg(task_status),
    attempt_count = history.river_attempt,
    current_attempt_id = attempt.id,
    last_failure_kind = history.failure_kind,
    last_error = history.provider_code,
    provider_message_id = history.provider_message_id,
    sent_at = CASE WHEN history.state = 'succeeded' THEN history.completed_at ELSE NULL END,
    status_updated_at = history.completed_at
FROM outbound_send_attempts AS attempt
JOIN outbound_send_attempt_history AS history ON history.send_attempt_id = attempt.id
WHERE attempt.id = sqlc.arg(attempt_id)
  AND history.id = sqlc.arg(history_id)
  AND attempt.task_id = task.id
  AND history.state = sqlc.arg(attempt_state)
  AND history.completed_at IS NOT NULL
  AND task.current_attempt_id = attempt.id
  AND task.attempt_count = history.river_attempt
  AND task.status IN ('sending', sqlc.arg(task_status));

-- name: LoadOutboundTaskResultFact :one
SELECT task.id AS task_id, task.customer_id, task.status AS current_task_status,
  task.attempt_count AS current_attempt_count, marker.id AS attempt_id, marker.river_job_id,
  history.id AS history_id, history.river_attempt, history.river_max_attempts,
  history.state AS attempt_state, history.failure_kind, history.provider_code,
  history.provider_message_id, history.completed_at
FROM outbound_send_attempt_history AS history
JOIN outbound_send_attempts AS marker ON marker.id = history.send_attempt_id
JOIN outbound_tasks AS task ON task.id = marker.task_id
WHERE history.id = sqlc.arg(history_id)::bigint;
