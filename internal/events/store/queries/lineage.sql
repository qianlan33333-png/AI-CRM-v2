-- name: ListEventDeliveryLineage :many
SELECT
  delivery.event_id,
  delivery.consumer,
  delivery.status,
  delivery.attempt_count,
  delivery.updated_at,
  count(*) OVER (PARTITION BY delivery.updated_at)::bigint AS same_timestamp_count
FROM event_deliveries AS delivery
ORDER BY delivery.updated_at DESC, delivery.event_id ASC, delivery.consumer ASC
LIMIT sqlc.arg(result_limit)::integer;
