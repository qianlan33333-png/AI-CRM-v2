-- name: ReserveAutomationTriggerReceipt :one
INSERT INTO automation_trigger_receipts (
  event_id, consumer, customer_id, tag_id, actor
) VALUES (
  sqlc.arg(event_id), sqlc.arg(consumer), sqlc.arg(customer_id),
  sqlc.arg(tag_id), sqlc.arg(actor)
)
ON CONFLICT (event_id, consumer) DO UPDATE
SET event_id = EXCLUDED.event_id
WHERE automation_trigger_receipts.customer_id = EXCLUDED.customer_id
  AND automation_trigger_receipts.tag_id = EXCLUDED.tag_id
  AND automation_trigger_receipts.actor = EXCLUDED.actor
RETURNING id, event_id, consumer, customer_id, tag_id, actor,
          state, triggered_event_id, triggered_at, completed_at;

-- name: CompleteAutomationTriggerReceipt :one
UPDATE automation_trigger_receipts
SET state = 'triggered',
    triggered_event_id = sqlc.arg(triggered_event_id),
    completed_at = COALESCE(completed_at, now())
WHERE id = sqlc.arg(id)
  AND (
    state = 'reserved'
    OR (state = 'triggered' AND triggered_event_id = sqlc.arg(triggered_event_id))
  )
RETURNING id, event_id, consumer, customer_id, tag_id, actor,
          state, triggered_event_id, triggered_at, completed_at;

-- name: ListAutomationTriggerReceipts :many
SELECT id, event_id, consumer, customer_id, tag_id, actor,
       state, triggered_event_id, triggered_at, completed_at
FROM automation_trigger_receipts
WHERE state = 'triggered'
  AND (sqlc.narg(receipt_id)::bigint IS NULL OR id = sqlc.narg(receipt_id))
  AND (sqlc.narg(event_id)::bigint IS NULL OR event_id = sqlc.narg(event_id))
  AND (sqlc.narg(started_after)::timestamptz IS NULL OR triggered_at >= sqlc.narg(started_after))
  AND (sqlc.narg(started_before)::timestamptz IS NULL OR triggered_at < sqlc.narg(started_before))
ORDER BY triggered_at DESC, id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountAutomationTriggerReceipts :one
SELECT count(*)
FROM automation_trigger_receipts
WHERE state = 'triggered'
  AND (sqlc.narg(receipt_id)::bigint IS NULL OR id = sqlc.narg(receipt_id))
  AND (sqlc.narg(event_id)::bigint IS NULL OR event_id = sqlc.narg(event_id))
  AND (sqlc.narg(started_after)::timestamptz IS NULL OR triggered_at >= sqlc.narg(started_after))
  AND (sqlc.narg(started_before)::timestamptz IS NULL OR triggered_at < sqlc.narg(started_before));
