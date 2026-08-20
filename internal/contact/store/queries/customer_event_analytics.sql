-- name: LoadCustomerEventAnalyticsSummary :one
WITH RECURSIVE root_customer AS (
  SELECT c.id
  FROM customers AS c
  WHERE c.id = sqlc.arg(customer_id)::bigint
    AND NOT c.is_deleted
    AND (sqlc.narg(owner_staff_id)::bigint IS NULL OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint)
), lineage_ids(customer_id) AS (
  SELECT root.id FROM root_customer AS root
  UNION
  SELECT lineage.merged_customer_id
  FROM customer_merge_lineage AS lineage
  JOIN lineage_ids AS parent ON lineage.primary_customer_id = parent.customer_id
), activity AS (
  SELECT event.event_type, event.occurred_at
  FROM customer_events AS event
  JOIN lineage_ids AS lineage ON lineage.customer_id = event.customer_id
  WHERE event.occurred_at >= sqlc.arg(from_time)::timestamptz
    AND event.occurred_at <= sqlc.arg(through_time)::timestamptz
)
SELECT root.id AS customer_id,
       count(activity.*)::bigint AS total_events,
       count(DISTINCT (activity.occurred_at AT TIME ZONE 'UTC')::date)::integer AS active_days,
       count(DISTINCT activity.event_type)::integer AS unique_event_types,
       COALESCE(max(activity.occurred_at), 'epoch'::timestamptz) AS last_occurred_at
FROM root_customer AS root
LEFT JOIN activity ON TRUE
GROUP BY root.id;

-- name: ListCustomerEventTypeAnalytics :many
WITH RECURSIVE lineage_ids(customer_id) AS (
  SELECT c.id FROM customers AS c
  WHERE c.id = sqlc.arg(customer_id)::bigint AND NOT c.is_deleted
    AND (sqlc.narg(owner_staff_id)::bigint IS NULL OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint)
  UNION
  SELECT lineage.merged_customer_id FROM customer_merge_lineage AS lineage
  JOIN lineage_ids AS parent ON lineage.primary_customer_id = parent.customer_id
)
SELECT event.event_type, count(*)::bigint AS event_count, COALESCE(max(event.occurred_at), 'epoch'::timestamptz) AS last_occurred_at
FROM customer_events AS event
JOIN lineage_ids AS lineage ON lineage.customer_id = event.customer_id
WHERE event.occurred_at >= sqlc.arg(from_time)::timestamptz AND event.occurred_at <= sqlc.arg(through_time)::timestamptz
GROUP BY event.event_type
ORDER BY event_count DESC, event.event_type ASC
LIMIT sqlc.arg(type_limit)::integer;

-- name: ListCustomerEventDailyAnalytics :many
WITH RECURSIVE lineage_ids(customer_id) AS (
  SELECT c.id FROM customers AS c
  WHERE c.id = sqlc.arg(customer_id)::bigint AND NOT c.is_deleted
    AND (sqlc.narg(owner_staff_id)::bigint IS NULL OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint)
  UNION
  SELECT lineage.merged_customer_id FROM customer_merge_lineage AS lineage
  JOIN lineage_ids AS parent ON lineage.primary_customer_id = parent.customer_id
)
SELECT (((event.occurred_at AT TIME ZONE 'UTC')::date)::timestamp AT TIME ZONE 'UTC') AS activity_day, count(*)::bigint AS event_count
FROM customer_events AS event
JOIN lineage_ids AS lineage ON lineage.customer_id = event.customer_id
WHERE event.occurred_at >= sqlc.arg(from_time)::timestamptz AND event.occurred_at <= sqlc.arg(through_time)::timestamptz
GROUP BY activity_day
ORDER BY activity_day ASC;
