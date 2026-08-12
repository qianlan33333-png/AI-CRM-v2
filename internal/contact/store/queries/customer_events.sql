-- name: ListCustomerEvents :many
WITH RECURSIVE root_customer AS (
  SELECT c.id, c.created_at
  FROM customers AS c
  WHERE c.id = sqlc.arg(customer_id)::bigint
    AND NOT c.is_deleted
    AND (
      sqlc.narg(owner_staff_id)::bigint IS NULL
      OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
    )
), lineage_ids(customer_id) AS (
  SELECT root.id
  FROM root_customer AS root
  UNION
  SELECT lineage.merged_customer_id
  FROM customer_merge_lineage AS lineage
  JOIN lineage_ids AS parent
    ON lineage.primary_customer_id = parent.customer_id
)
SELECT
  COALESCE(e.customer_id, root.id)::bigint AS customer_id,
  COALESCE(e.id, 0)::bigint AS event_id,
  COALESCE(e.event_type, '')::text AS event_type,
  COALESCE(e.payload, '{}'::jsonb) AS payload,
  COALESCE(e.actor, '')::text AS actor,
  COALESCE(e.occurred_at, root.created_at) AS occurred_at
FROM root_customer AS root
LEFT JOIN LATERAL (
  SELECT
    candidate.customer_id,
    candidate.id,
    candidate.event_type,
    candidate.payload,
    candidate.actor,
    candidate.occurred_at
  FROM lineage_ids AS event_customer
  CROSS JOIN LATERAL (
    SELECT
      ce.customer_id,
      ce.id,
      ce.event_type,
      ce.payload,
      ce.actor,
      ce.occurred_at
    FROM customer_events AS ce
    WHERE ce.customer_id = event_customer.customer_id
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
  ) AS candidate
  ORDER BY candidate.occurred_at DESC, candidate.id DESC
  LIMIT sqlc.arg(row_limit)::integer
) AS e ON TRUE
ORDER BY e.occurred_at DESC NULLS LAST, e.id DESC NULLS LAST;
