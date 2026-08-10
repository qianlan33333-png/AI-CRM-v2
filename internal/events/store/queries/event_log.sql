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
