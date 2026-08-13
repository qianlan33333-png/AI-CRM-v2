-- name: LockExternalEventIdempotencyKey :exec
SELECT pg_advisory_xact_lock(
  hashtextextended(sqlc.arg(idempotency_key)::text, 0)
);

-- name: GetExternalEventIdempotency :one
SELECT
  event_occurred_at,
  event_id,
  event_customer_id,
  event_type,
  payload,
  actor
FROM customer_event_idempotency
WHERE idempotency_key = sqlc.arg(idempotency_key)::text;

-- name: InsertExternalEventIdempotency :execrows
INSERT INTO customer_event_idempotency (
  idempotency_key,
  event_occurred_at,
  event_id,
  event_customer_id,
  event_type,
  payload,
  actor
) VALUES (
  sqlc.arg(idempotency_key)::text,
  sqlc.arg(event_occurred_at)::timestamptz,
  sqlc.arg(event_id)::bigint,
  sqlc.arg(event_customer_id)::bigint,
  sqlc.arg(event_type)::text,
  sqlc.arg(payload)::jsonb,
  sqlc.arg(actor)::text
)
ON CONFLICT (idempotency_key) DO NOTHING;
