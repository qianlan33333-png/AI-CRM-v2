-- name: ListCustomerEvents :many
SELECT
  c.id AS customer_id,
  COALESCE(e.id, 0)::bigint AS event_id,
  COALESCE(e.event_type, '')::text AS event_type,
  COALESCE(e.payload, '{}'::jsonb) AS payload,
  COALESCE(e.actor, '')::text AS actor,
  COALESCE(e.occurred_at, c.created_at) AS occurred_at
FROM customers AS c
LEFT JOIN LATERAL (
  SELECT
    ce.id,
    ce.event_type,
    ce.payload,
    ce.actor,
    ce.occurred_at
  FROM customer_events AS ce
  WHERE ce.customer_id = c.id
    AND (
      (
        sqlc.narg(after_occurred_at)::timestamptz IS NULL
        AND sqlc.narg(after_id)::bigint IS NULL
      )
      OR (
        sqlc.narg(after_occurred_at)::timestamptz IS NOT NULL
        AND sqlc.narg(after_id)::bigint IS NOT NULL
        AND (ce.occurred_at, ce.id) < (
          sqlc.narg(after_occurred_at)::timestamptz,
          sqlc.narg(after_id)::bigint
        )
      )
    )
  ORDER BY ce.occurred_at DESC, ce.id DESC
  LIMIT sqlc.arg(row_limit)::integer
) AS e ON TRUE
WHERE c.id = sqlc.arg(customer_id)::bigint
  AND NOT c.is_deleted
  AND (
    sqlc.narg(owner_staff_id)::bigint IS NULL
    OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
  )
ORDER BY e.occurred_at DESC NULLS LAST, e.id DESC NULLS LAST;
