-- name: ReserveOutboundEnqueueReceipt :one
INSERT INTO outbound_enqueue_receipts (
  idempotency_scope,
  idempotency_key,
  customer_id,
  template_key,
  payload
)
SELECT
  sqlc.arg(idempotency_scope)::text,
  sqlc.arg(idempotency_key)::text,
  customer.id,
  sqlc.arg(template_key)::text,
  sqlc.arg(payload)::jsonb
FROM customers AS customer
WHERE customer.id = sqlc.arg(customer_id)::bigint
ON CONFLICT (idempotency_scope, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, idempotency_scope, idempotency_key, customer_id, template_key, payload, state, task_id, event_id, river_job_id;

-- name: AcceptOutboundEnqueueReceipt :one
UPDATE outbound_enqueue_receipts
SET state = 'accepted',
    task_id = sqlc.arg(task_id)::bigint,
    event_id = sqlc.arg(event_id)::bigint,
    river_job_id = sqlc.arg(river_job_id)::bigint,
    accepted_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'reserved'
RETURNING id, idempotency_scope, idempotency_key, customer_id, template_key, payload, state, task_id, event_id, river_job_id;
