-- name: ReserveSegmentRefreshReceipt :one
INSERT INTO segment_refresh_receipts (idempotency_scope, idempotency_key, segment_id)
SELECT
  sqlc.arg(idempotency_scope)::text,
  sqlc.arg(idempotency_key)::text,
  id
FROM segments
WHERE id = sqlc.arg(segment_id)::bigint
ON CONFLICT (idempotency_scope, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, segment_id, state, river_job_id;

-- name: AcceptSegmentRefreshReceipt :one
UPDATE segment_refresh_receipts
SET state = 'accepted',
    river_job_id = sqlc.arg(river_job_id)::bigint,
    accepted_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'reserved'
RETURNING id, segment_id, state, river_job_id;
