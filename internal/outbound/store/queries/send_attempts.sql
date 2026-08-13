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
