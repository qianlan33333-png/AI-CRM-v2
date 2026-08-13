-- name: ReserveOutboundCancelReceipt :one
INSERT INTO outbound_control_receipts (
  idempotency_scope, idempotency_key, operation, task_id
) VALUES (
  sqlc.arg(idempotency_scope)::text,
  sqlc.arg(idempotency_key)::text,
  'cancel',
  sqlc.arg(task_id)::bigint
)
ON CONFLICT (idempotency_scope, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, idempotency_scope, idempotency_key, operation, task_id, state,
  customer_id, job_generation, river_job_id, job_kind, event_id, task_status, completed_at;

-- name: LockOutboundTaskForCancel :one
SELECT task.id, task.customer_id, task.status
FROM outbound_tasks AS task
WHERE task.id = sqlc.arg(task_id)::bigint
FOR UPDATE;

-- name: LoadLatestOutboundTaskJobLink :one
SELECT link.task_id, link.generation, link.river_job_id, link.job_kind, link.cancelled_at
FROM outbound_task_job_links AS link
WHERE link.task_id = sqlc.arg(task_id)::bigint
ORDER BY link.generation DESC
LIMIT 1;

-- name: RecordOutboundTaskJobLink :one
INSERT INTO outbound_task_job_links (task_id, generation, river_job_id, job_kind)
VALUES (
  sqlc.arg(task_id)::bigint,
  sqlc.arg(generation)::integer,
  sqlc.arg(river_job_id)::bigint,
  sqlc.arg(job_kind)::text
)
ON CONFLICT (task_id, generation) DO UPDATE
SET task_id = EXCLUDED.task_id
RETURNING task_id, generation, river_job_id, job_kind, cancelled_at;

-- name: MarkOutboundTaskJobCancelled :one
UPDATE outbound_task_job_links AS link
SET cancelled_at = now()
WHERE link.task_id = sqlc.arg(task_id)::bigint
  AND link.generation = sqlc.arg(generation)::integer
  AND link.river_job_id = sqlc.arg(river_job_id)::bigint
  AND link.job_kind = sqlc.arg(job_kind)::text
  AND link.cancelled_at IS NULL
RETURNING link.task_id, link.generation, link.river_job_id, link.job_kind, link.cancelled_at;

-- name: MarkOutboundTaskCancelled :one
UPDATE outbound_tasks AS task
SET status = 'cancelled', status_updated_at = now()
WHERE task.id = sqlc.arg(task_id)::bigint
  AND task.status = 'pending'
RETURNING task.id, task.customer_id, task.status, task.status_updated_at;

-- name: CompleteOutboundCancelReceipt :one
UPDATE outbound_control_receipts AS receipt
SET state = 'completed',
    customer_id = sqlc.arg(customer_id)::bigint,
    job_generation = sqlc.arg(job_generation)::integer,
    river_job_id = sqlc.arg(river_job_id)::bigint,
    job_kind = sqlc.arg(job_kind)::text,
    event_id = sqlc.arg(event_id)::bigint,
    task_status = 'cancelled',
    completed_at = now()
WHERE receipt.id = sqlc.arg(id)::bigint
  AND receipt.operation = 'cancel'
  AND receipt.task_id = sqlc.arg(task_id)::bigint
  AND receipt.state = 'reserved'
RETURNING receipt.id, receipt.idempotency_scope, receipt.idempotency_key,
  receipt.operation, receipt.task_id, receipt.state, receipt.customer_id,
  receipt.job_generation, receipt.river_job_id, receipt.job_kind,
  receipt.event_id, receipt.task_status, receipt.completed_at;

-- name: ReserveOutboundManualRetryReceipt :one
INSERT INTO outbound_control_receipts (
  idempotency_scope, idempotency_key, operation, task_id
) VALUES (
  sqlc.arg(idempotency_scope)::text,
  sqlc.arg(idempotency_key)::text,
  'manual_retry',
  sqlc.arg(task_id)::bigint
)
ON CONFLICT (idempotency_scope, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, idempotency_scope, idempotency_key, operation, task_id, state,
  customer_id, job_generation, river_job_id, job_kind, event_id, task_status, completed_at;

-- name: LockOutboundTaskForManualRetry :one
SELECT task.id, task.customer_id, task.status, task.batch_id, task.batch_chunk_index,
  receipt.id AS enqueue_receipt_id
FROM outbound_tasks AS task
LEFT JOIN outbound_enqueue_receipts AS receipt ON receipt.task_id = task.id AND receipt.state = 'accepted'
WHERE task.id = sqlc.arg(task_id)::bigint
FOR UPDATE OF task;

-- name: MarkOutboundTaskManualRetryPending :one
UPDATE outbound_tasks AS task
SET status = 'pending',
    attempt_count = 0,
    current_attempt_id = NULL,
    last_failure_kind = NULL,
    last_error = NULL,
    provider_message_id = NULL,
    sent_at = NULL,
    status_updated_at = now()
WHERE task.id = sqlc.arg(task_id)::bigint
  AND task.status IN ('final_failed', 'cancelled')
RETURNING task.id, task.customer_id, task.status, task.status_updated_at;

-- name: CompleteOutboundManualRetryReceipt :one
UPDATE outbound_control_receipts AS receipt
SET state = 'completed',
    customer_id = sqlc.arg(customer_id)::bigint,
    job_generation = sqlc.arg(job_generation)::integer,
    river_job_id = sqlc.arg(river_job_id)::bigint,
    job_kind = sqlc.arg(job_kind)::text,
    event_id = sqlc.arg(event_id)::bigint,
    task_status = 'pending',
    completed_at = now()
WHERE receipt.id = sqlc.arg(id)::bigint
  AND receipt.operation = 'manual_retry'
  AND receipt.task_id = sqlc.arg(task_id)::bigint
  AND receipt.state = 'reserved'
RETURNING receipt.id, receipt.idempotency_scope, receipt.idempotency_key,
  receipt.operation, receipt.task_id, receipt.state, receipt.customer_id,
  receipt.job_generation, receipt.river_job_id, receipt.job_kind,
  receipt.event_id, receipt.task_status, receipt.completed_at;
