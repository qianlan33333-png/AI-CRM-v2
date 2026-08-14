-- name: AppendEvent :one
INSERT INTO event_log (
  event_type,
  customer_id,
  payload,
  occurred_at,
  idempotency_key
) VALUES (
  sqlc.arg(event_type),
  sqlc.narg(customer_id)::bigint,
  sqlc.arg(payload)::jsonb,
  sqlc.arg(occurred_at),
  sqlc.arg(idempotency_key)
)
ON CONFLICT (idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
WHERE event_log.event_type = EXCLUDED.event_type
  AND event_log.customer_id IS NOT DISTINCT FROM EXCLUDED.customer_id
  AND event_log.payload = EXCLUDED.payload
  AND event_log.occurred_at = EXCLUDED.occurred_at
RETURNING id;

-- name: ClaimUndispatchedEvents :many
SELECT
  id,
  event_type,
  customer_id,
  payload,
  occurred_at,
  idempotency_key,
  dispatched
FROM event_log
WHERE NOT dispatched
  AND NOT (event_type = ANY(sqlc.arg(excluded_event_types)::text[]))
ORDER BY id
LIMIT sqlc.arg(batch_size)
FOR UPDATE SKIP LOCKED;

-- name: ClaimEventsMissingDelivery :many
SELECT
  event.id,
  event.event_type,
  event.customer_id,
  event.payload,
  event.occurred_at,
  event.idempotency_key,
  event.dispatched
FROM event_log AS event
WHERE event.event_type = sqlc.arg(event_type)
  AND NOT EXISTS (
    SELECT 1
    FROM event_deliveries AS delivery
    WHERE delivery.event_id = event.id
      AND delivery.consumer = sqlc.arg(consumer)
  )
ORDER BY event.id
LIMIT sqlc.arg(batch_size)
FOR UPDATE OF event SKIP LOCKED;

-- name: MarkEventsDispatched :execrows
UPDATE event_log
SET dispatched = TRUE
WHERE id = ANY(sqlc.arg(event_ids)::bigint[])
  AND NOT dispatched;

-- name: GetEvent :one
SELECT
  id,
  event_type,
  customer_id,
  payload,
  occurred_at,
  idempotency_key,
  dispatched
FROM event_log
WHERE id = sqlc.arg(event_id);

-- name: ReserveEventDelivery :one
INSERT INTO event_deliveries (event_id, consumer)
VALUES (sqlc.arg(event_id), sqlc.arg(consumer))
ON CONFLICT (event_id, consumer) DO NOTHING
RETURNING event_id;

-- name: GetEventDelivery :one
SELECT event_id, consumer, status, attempt_count, river_job_id,
       lease_owner, lease_expires_at, last_error_code, completed_at
FROM event_deliveries
WHERE event_id = sqlc.arg(event_id) AND consumer = sqlc.arg(consumer);

-- name: AcceptEventDelivery :one
UPDATE event_deliveries
SET river_job_id = sqlc.arg(river_job_id), updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer = sqlc.arg(consumer)
  AND (river_job_id IS NULL OR river_job_id = sqlc.arg(river_job_id))
RETURNING river_job_id;

-- name: MarkEventDispatched :execrows
UPDATE event_log SET dispatched = TRUE
WHERE id = sqlc.arg(event_id) AND NOT dispatched;

-- name: ClaimEventDelivery :one
UPDATE event_deliveries AS delivery
SET status = 'processing',
    attempt_count = attempt_count + 1,
    lease_owner = sqlc.arg(lease_owner),
    lease_expires_at = sqlc.arg(lease_expires_at),
    last_error_code = NULL,
    updated_at = now()
FROM event_log AS event
WHERE delivery.event_id = sqlc.arg(event_id)
  AND delivery.consumer = sqlc.arg(consumer)
  AND event.id = delivery.event_id
  AND (
    delivery.status = 'pending'
    OR (delivery.status = 'processing' AND (
      delivery.lease_owner = sqlc.arg(lease_owner)
      OR delivery.lease_expires_at <= sqlc.arg(claimed_at)
    ))
  )
RETURNING event.id, event.event_type, event.customer_id, event.payload,
          event.occurred_at, event.idempotency_key,
          delivery.consumer, delivery.status, delivery.attempt_count;

-- name: CompleteEventDelivery :one
UPDATE event_deliveries
SET status = 'completed', lease_owner = NULL, lease_expires_at = NULL,
    last_error_code = NULL, completed_at = COALESCE(completed_at, now()), updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer = sqlc.arg(consumer)
  AND (
    status = 'completed'
    OR (status = 'processing' AND lease_owner = sqlc.arg(lease_owner))
  )
RETURNING status;

-- name: RetryEventDelivery :execrows
UPDATE event_deliveries
SET status = 'pending', lease_owner = NULL, lease_expires_at = NULL,
    last_error_code = sqlc.arg(error_code), updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer = sqlc.arg(consumer)
  AND status = 'processing'
  AND lease_owner = sqlc.arg(lease_owner);

-- name: FinalFailEventDelivery :execrows
UPDATE event_deliveries
SET status = 'final_failed', lease_owner = NULL, lease_expires_at = NULL,
    last_error_code = sqlc.arg(error_code), completed_at = now(), updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer = sqlc.arg(consumer)
  AND status = 'processing'
  AND lease_owner = sqlc.arg(lease_owner);

-- name: OutcomeUnknownEventDelivery :execrows
UPDATE event_deliveries
SET status = 'outcome_unknown', lease_owner = NULL, lease_expires_at = NULL,
    last_error_code = sqlc.arg(error_code), completed_at = now(), updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer = sqlc.arg(consumer)
  AND status = 'processing'
  AND lease_owner = sqlc.arg(lease_owner);
