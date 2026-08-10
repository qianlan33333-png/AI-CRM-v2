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
ORDER BY id
LIMIT sqlc.arg(batch_size)
FOR UPDATE SKIP LOCKED;

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
