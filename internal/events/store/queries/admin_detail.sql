-- name: GetAdminDetailEvent :one
SELECT id, event_type, occurred_at, dispatched
FROM event_log
WHERE id = sqlc.arg(event_id)::bigint;

-- name: ListAdminDetailDeliveries :many
SELECT event_id, consumer, status, attempt_count, completed_at
FROM event_deliveries
WHERE event_id = sqlc.arg(event_id)::bigint
ORDER BY consumer;
