-- name: ListAdminReadEvents :many
SELECT id, event_type, occurred_at, dispatched
FROM event_log
WHERE (sqlc.arg(event_type)::text = '' OR event_type = sqlc.arg(event_type)::text)
ORDER BY occurred_at DESC, id DESC;

-- name: ListAdminReadDeliveries :many
SELECT event_id, consumer, status, attempt_count, completed_at
FROM event_deliveries
WHERE event_id = ANY(sqlc.arg(event_ids)::bigint[])
ORDER BY event_id, consumer;
