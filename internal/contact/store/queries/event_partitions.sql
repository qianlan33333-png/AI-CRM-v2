-- name: EnsureCustomerEventPartitions :exec
SELECT public.aicrm_ensure_customer_event_partitions(
  sqlc.arg(anchor)::timestamptz,
  sqlc.arg(future_months)::integer
);
