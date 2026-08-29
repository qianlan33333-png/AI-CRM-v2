-- name: ListCloudAuditFacts :many
SELECT event.id, event.event_type, event.occurred_at, event.dispatched,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'pending')::bigint AS pending,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'processing')::bigint AS processing,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'completed')::bigint AS completed,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'final_failed')::bigint AS final_failed,
       count(delivery.event_id) FILTER (WHERE delivery.status = 'outcome_unknown')::bigint AS outcome_unknown
FROM public.event_log AS event
LEFT JOIN public.event_deliveries AS delivery ON delivery.event_id = event.id
WHERE (sqlc.arg(trace_id)::text = '' OR event.payload ->> 'trace_id' = sqlc.arg(trace_id)::text)
  AND (sqlc.arg(session_id)::text = '' OR event.payload ->> 'session_id' = sqlc.arg(session_id)::text)
GROUP BY event.id, event.event_type, event.occurred_at, event.dispatched
ORDER BY event.occurred_at DESC, event.id DESC
LIMIT sqlc.arg(row_limit);
