-- name: ListCustomers :many
SELECT
  c.id,
  c.name,
  c.avatar_url,
  c.gender,
  c.stage_id,
  c.owner_staff_id,
  c.channel_id,
  c.added_at,
  c.last_interact_at,
  c.is_deleted,
  c.extra,
  c.created_at,
  c.updated_at
FROM customers AS c
WHERE c.updated_at <= sqlc.arg(watermark)::timestamptz
  AND (
    sqlc.narg(customer_id)::bigint IS NULL
    OR c.id = sqlc.narg(customer_id)::bigint
  )
  AND (
    sqlc.narg(keyword)::text IS NULL
    OR lower(c.name) % lower(sqlc.narg(keyword)::text)
  )
  AND (
    sqlc.narg(owner_staff_id)::bigint IS NULL
    OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
  )
  AND (
    sqlc.narg(stage_id)::bigint IS NULL
    OR c.stage_id = sqlc.narg(stage_id)::bigint
  )
  AND (
    sqlc.narg(channel_id)::bigint IS NULL
    OR c.channel_id = sqlc.narg(channel_id)::bigint
  )
  AND (
    sqlc.narg(tag_id)::bigint IS NULL
    OR EXISTS (
      SELECT 1
      FROM customer_tags AS ct
      WHERE ct.tag_id = sqlc.narg(tag_id)::bigint
        AND ct.customer_id = c.id
    )
  )
  AND c.is_deleted = sqlc.arg(is_deleted)
  AND (
    sqlc.narg(added_after)::timestamptz IS NULL
    OR c.added_at >= sqlc.narg(added_after)::timestamptz
  )
  AND (
    sqlc.narg(added_before)::timestamptz IS NULL
    OR c.added_at <= sqlc.narg(added_before)::timestamptz
  )
  AND (
    sqlc.narg(last_interact_after)::timestamptz IS NULL
    OR c.last_interact_at >= sqlc.narg(last_interact_after)::timestamptz
  )
  AND (
    sqlc.narg(last_interact_before)::timestamptz IS NULL
    OR c.last_interact_at <= sqlc.narg(last_interact_before)::timestamptz
  )
  AND (
    (
      sqlc.narg(after_updated_at)::timestamptz IS NULL
      AND sqlc.narg(after_id)::bigint IS NULL
    )
    OR (
      sqlc.narg(after_updated_at)::timestamptz IS NOT NULL
      AND sqlc.narg(after_id)::bigint IS NOT NULL
      AND (c.updated_at, c.id) < (
        sqlc.narg(after_updated_at)::timestamptz,
        sqlc.narg(after_id)::bigint
      )
    )
  )
ORDER BY c.updated_at DESC, c.id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: CountCustomerIDsBounded :one
SELECT count(*)::bigint
FROM (
  (
    SELECT c.id
    FROM customers AS c
    WHERE sqlc.narg(tag_id)::bigint IS NULL
      AND c.updated_at <= sqlc.arg(watermark)::timestamptz
      AND (
        sqlc.narg(customer_id)::bigint IS NULL
        OR c.id = sqlc.narg(customer_id)::bigint
      )
      AND (
        sqlc.narg(keyword)::text IS NULL
        OR lower(c.name) % lower(sqlc.narg(keyword)::text)
      )
      AND (
        sqlc.narg(owner_staff_id)::bigint IS NULL
        OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
      )
      AND (
        sqlc.narg(stage_id)::bigint IS NULL
        OR c.stage_id = sqlc.narg(stage_id)::bigint
      )
      AND (
        sqlc.narg(channel_id)::bigint IS NULL
        OR c.channel_id = sqlc.narg(channel_id)::bigint
      )
      AND c.is_deleted = sqlc.arg(is_deleted)
      AND (
        sqlc.narg(added_after)::timestamptz IS NULL
        OR c.added_at >= sqlc.narg(added_after)::timestamptz
      )
      AND (
        sqlc.narg(added_before)::timestamptz IS NULL
        OR c.added_at <= sqlc.narg(added_before)::timestamptz
      )
      AND (
        sqlc.narg(last_interact_after)::timestamptz IS NULL
        OR c.last_interact_at >= sqlc.narg(last_interact_after)::timestamptz
      )
      AND (
        sqlc.narg(last_interact_before)::timestamptz IS NULL
        OR c.last_interact_at <= sqlc.narg(last_interact_before)::timestamptz
      )
    ORDER BY c.updated_at DESC, c.id DESC
    LIMIT sqlc.arg(total_limit)::integer
  )
  UNION ALL
  (
    SELECT tagged_customer.id
    FROM customer_tags AS ct
    CROSS JOIN LATERAL (
      SELECT c.id
      FROM customers AS c
      WHERE c.id = ct.customer_id
        AND (
          sqlc.narg(customer_id)::bigint IS NULL
          OR c.id = sqlc.narg(customer_id)::bigint
        )
        AND c.updated_at <= sqlc.arg(watermark)::timestamptz
        AND (
          sqlc.narg(keyword)::text IS NULL
          OR lower(c.name) % lower(sqlc.narg(keyword)::text)
        )
        AND (
          sqlc.narg(owner_staff_id)::bigint IS NULL
          OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
        )
        AND (
          sqlc.narg(stage_id)::bigint IS NULL
          OR c.stage_id = sqlc.narg(stage_id)::bigint
        )
        AND (
          sqlc.narg(channel_id)::bigint IS NULL
          OR c.channel_id = sqlc.narg(channel_id)::bigint
        )
        AND c.is_deleted = sqlc.arg(is_deleted)
        AND (
          sqlc.narg(added_after)::timestamptz IS NULL
          OR c.added_at >= sqlc.narg(added_after)::timestamptz
        )
        AND (
          sqlc.narg(added_before)::timestamptz IS NULL
          OR c.added_at <= sqlc.narg(added_before)::timestamptz
        )
        AND (
          sqlc.narg(last_interact_after)::timestamptz IS NULL
          OR c.last_interact_at >= sqlc.narg(last_interact_after)::timestamptz
        )
        AND (
          sqlc.narg(last_interact_before)::timestamptz IS NULL
          OR c.last_interact_at <= sqlc.narg(last_interact_before)::timestamptz
        )
      LIMIT 1
    ) AS tagged_customer
    WHERE sqlc.narg(tag_id)::bigint IS NOT NULL
      AND ct.tag_id = sqlc.narg(tag_id)::bigint
    LIMIT sqlc.arg(total_limit)::integer
  )
) AS bounded_customer_ids;
