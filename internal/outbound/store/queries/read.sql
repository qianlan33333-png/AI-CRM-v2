-- name: GetOutboundTaskReadModel :one
SELECT task.id, task.customer_id, customer.owner_staff_id, task.batch_id,
  task.batch_chunk_index, task.status, task.attempt_count, task.current_attempt_id,
  task.last_failure_kind, task.last_error, task.provider_message_id, task.created_at,
  task.status_updated_at, task.sent_at, link.generation, link.river_job_id,
  link.job_kind
FROM outbound_tasks AS task
JOIN customers AS customer ON customer.id = task.customer_id
JOIN LATERAL (
  SELECT candidate.generation, candidate.river_job_id, candidate.job_kind
  FROM outbound_task_job_links AS candidate
  WHERE candidate.task_id = task.id
  ORDER BY candidate.generation DESC
  LIMIT 1
) AS link ON TRUE
WHERE task.id = sqlc.arg(task_id)::bigint
  AND (
    sqlc.narg(owner_staff_id)::bigint IS NULL
    OR customer.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
  );

-- name: ListOutboundTaskReadModels :many
SELECT task.id, task.customer_id, customer.owner_staff_id, task.batch_id,
  task.batch_chunk_index, task.status, task.attempt_count, task.current_attempt_id,
  task.last_failure_kind, task.last_error, task.provider_message_id, task.created_at,
  task.status_updated_at, task.sent_at, link.generation, link.river_job_id,
  link.job_kind
FROM outbound_tasks AS task
JOIN customers AS customer ON customer.id = task.customer_id
JOIN LATERAL (
  SELECT candidate.generation, candidate.river_job_id, candidate.job_kind
  FROM outbound_task_job_links AS candidate
  WHERE candidate.task_id = task.id
  ORDER BY candidate.generation DESC
  LIMIT 1
) AS link ON TRUE
WHERE (
    sqlc.narg(batch_id)::bigint IS NULL
    OR task.batch_id = sqlc.narg(batch_id)::bigint
  )
  AND (
    sqlc.narg(task_status)::text IS NULL
    OR task.status = sqlc.narg(task_status)::text
  )
  AND (
    sqlc.narg(owner_staff_id)::bigint IS NULL
    OR customer.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
  )
ORDER BY task.id DESC
LIMIT sqlc.arg(result_limit)::integer
OFFSET sqlc.arg(result_offset)::integer;

-- name: ListOutboundAttemptReadModels :many
SELECT marker.id, history.id AS history_id, marker.task_id, link.generation,
  marker.river_job_id, history.river_attempt, history.river_max_attempts,
  history.state, history.failure_kind, history.provider_code,
  history.provider_message_id, history.dispatch_started_at, history.completed_at
FROM outbound_send_attempts AS marker
JOIN outbound_send_attempt_history AS history ON history.send_attempt_id = marker.id
JOIN outbound_task_job_links AS link
  ON link.task_id = marker.task_id AND link.river_job_id = marker.river_job_id
WHERE marker.task_id = sqlc.arg(task_id)::bigint
ORDER BY link.generation, history.river_attempt, history.id;

-- name: ListOutboundControlReceiptReadModels :many
SELECT receipt.id, receipt.task_id, receipt.operation, receipt.state,
  receipt.job_generation, receipt.river_job_id, receipt.job_kind,
  receipt.event_id, receipt.task_status, receipt.created_at, receipt.completed_at
FROM outbound_control_receipts AS receipt
WHERE receipt.task_id = sqlc.arg(task_id)::bigint
  AND receipt.state = 'completed'
ORDER BY receipt.id;
