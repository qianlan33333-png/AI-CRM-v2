-- name: ListOutboundDeliveryLineage :many
SELECT
  task.id,
  task.status,
  task.attempt_count,
  task.status_updated_at,
  count(*) OVER (PARTITION BY task.status_updated_at)::bigint AS same_timestamp_count
FROM outbound_tasks AS task
ORDER BY task.status_updated_at DESC, ('outbound-task:' || task.id::text) ASC
LIMIT sqlc.arg(result_limit)::integer;
