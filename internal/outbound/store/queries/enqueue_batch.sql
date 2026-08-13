-- name: ReserveOutboundBatch :one
INSERT INTO outbound_batches (
  idempotency_scope, idempotency_key, tier, recipient_digest,
  recipient_count, template_key, payload
) VALUES (
  sqlc.arg(idempotency_scope)::text,
  sqlc.arg(idempotency_key)::text,
  sqlc.arg(tier)::text,
  sqlc.arg(recipient_digest)::bytea,
  sqlc.arg(recipient_count)::integer,
  sqlc.arg(template_key)::text,
  sqlc.arg(payload)::jsonb
)
ON CONFLICT (idempotency_scope, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, idempotency_scope, idempotency_key, tier, recipient_digest,
  recipient_count, template_key, payload, accepted_event_id;

-- name: AcceptOutboundBatch :one
UPDATE outbound_batches
SET accepted_event_id = sqlc.arg(accepted_event_id)::bigint,
    accepted_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND accepted_event_id IS NULL
RETURNING id, idempotency_scope, idempotency_key, tier, recipient_digest,
  recipient_count, template_key, payload, accepted_event_id;

-- name: ReserveOutboundBatchChunk :one
INSERT INTO outbound_batch_chunks (
  batch_id, chunk_index, recipient_start, recipient_count
) VALUES (
  sqlc.arg(batch_id)::bigint,
  sqlc.arg(chunk_index)::integer,
  sqlc.arg(recipient_start)::integer,
  sqlc.arg(recipient_count)::integer
)
ON CONFLICT (batch_id, chunk_index) DO UPDATE
SET batch_id = EXCLUDED.batch_id
RETURNING id, batch_id, chunk_index, recipient_start, recipient_count, state;

-- name: CreateOutboundBatchTask :one
INSERT INTO outbound_tasks (
  customer_id, template_key, payload, batch_id, batch_chunk_index
)
SELECT
  customer.id,
  sqlc.arg(template_key)::text,
  sqlc.arg(payload)::jsonb,
  sqlc.arg(batch_id)::bigint,
  sqlc.arg(batch_chunk_index)::integer
FROM customers AS customer
WHERE customer.id = sqlc.arg(customer_id)::bigint
RETURNING id;

-- name: AcceptOutboundBatchChunk :one
UPDATE outbound_batch_chunks
SET state = 'expanded', expanded_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'reserved'
RETURNING id, batch_id, chunk_index, recipient_start, recipient_count, state;
