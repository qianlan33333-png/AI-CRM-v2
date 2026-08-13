\set ON_ERROR_STOP on
\pset format unaligned
\pset tuples_only on
\pset pager off

SET plan_cache_mode = force_generic_plan;

PREPARE count_customer_ids_bounded(
  bigint, timestamptz, text, bigint, bigint, bigint, boolean,
  timestamptz, timestamptz, timestamptz, timestamptz, integer
) AS
SELECT count(*)::bigint
FROM (
  (
    SELECT c.id
    FROM customers AS c
    WHERE $1::bigint IS NULL
      AND c.updated_at <= $2::timestamptz
      AND ($3::text IS NULL OR lower(c.name) % lower($3::text))
      AND ($4::bigint IS NULL OR c.owner_staff_id = $4::bigint)
      AND ($5::bigint IS NULL OR c.stage_id = $5::bigint)
      AND ($6::bigint IS NULL OR c.channel_id = $6::bigint)
      AND c.is_deleted = $7
      AND ($8::timestamptz IS NULL OR c.added_at >= $8::timestamptz)
      AND ($9::timestamptz IS NULL OR c.added_at <= $9::timestamptz)
      AND ($10::timestamptz IS NULL OR c.last_interact_at >= $10::timestamptz)
      AND ($11::timestamptz IS NULL OR c.last_interact_at <= $11::timestamptz)
    ORDER BY c.updated_at DESC, c.id DESC
    LIMIT $12::integer
  )
  UNION ALL
  (
    SELECT tagged_customer.id
    FROM customer_tags AS ct
    CROSS JOIN LATERAL (
      SELECT c.id
      FROM customers AS c
      WHERE c.id = ct.customer_id
        AND c.updated_at <= $2::timestamptz
        AND ($3::text IS NULL OR lower(c.name) % lower($3::text))
        AND ($4::bigint IS NULL OR c.owner_staff_id = $4::bigint)
        AND ($5::bigint IS NULL OR c.stage_id = $5::bigint)
        AND ($6::bigint IS NULL OR c.channel_id = $6::bigint)
        AND c.is_deleted = $7
        AND ($8::timestamptz IS NULL OR c.added_at >= $8::timestamptz)
        AND ($9::timestamptz IS NULL OR c.added_at <= $9::timestamptz)
        AND ($10::timestamptz IS NULL OR c.last_interact_at >= $10::timestamptz)
        AND ($11::timestamptz IS NULL OR c.last_interact_at <= $11::timestamptz)
      LIMIT 1
    ) AS tagged_customer
    WHERE $1::bigint IS NOT NULL
      AND ct.tag_id = $1::bigint
    LIMIT $12::integer
  )
) AS bounded_customer_ids;

\o /tmp/p3-c06-count-generic-plan.json
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
EXECUTE count_customer_ids_bounded(
  11, '2026-08-13T00:00:00Z', 'kw017', 7, 3, 5, false,
  '2026-04-01T00:00:00Z', '2026-07-31T23:59:59Z',
  '2026-05-01T00:00:00Z', '2026-08-11T23:59:59Z', 10001
);
\o /tmp/p3-c06-count-prepared.txt
SELECT name, generic_plans, custom_plans
FROM pg_prepared_statements
WHERE name = 'count_customer_ids_bounded';

PREPARE list_customers(
  timestamptz, text, bigint, bigint, bigint, bigint, boolean,
  timestamptz, timestamptz, timestamptz, timestamptz,
  timestamptz, bigint, integer
) AS
SELECT
  c.id, c.name, c.avatar_url, c.gender, c.stage_id, c.owner_staff_id,
  c.channel_id, c.added_at, c.last_interact_at, c.is_deleted, c.extra,
  c.created_at, c.updated_at
FROM customers AS c
WHERE c.updated_at <= $1::timestamptz
  AND ($2::text IS NULL OR lower(c.name) % lower($2::text))
  AND ($3::bigint IS NULL OR c.owner_staff_id = $3::bigint)
  AND ($4::bigint IS NULL OR c.stage_id = $4::bigint)
  AND ($5::bigint IS NULL OR c.channel_id = $5::bigint)
  AND (
    $6::bigint IS NULL
    OR EXISTS (
      SELECT 1
      FROM customer_tags AS ct
      WHERE ct.tag_id = $6::bigint
        AND ct.customer_id = c.id
    )
  )
  AND c.is_deleted = $7
  AND ($8::timestamptz IS NULL OR c.added_at >= $8::timestamptz)
  AND ($9::timestamptz IS NULL OR c.added_at <= $9::timestamptz)
  AND ($10::timestamptz IS NULL OR c.last_interact_at >= $10::timestamptz)
  AND ($11::timestamptz IS NULL OR c.last_interact_at <= $11::timestamptz)
  AND (
    ($12::timestamptz IS NULL AND $13::bigint IS NULL)
    OR (
      $12::timestamptz IS NOT NULL
      AND $13::bigint IS NOT NULL
      AND (c.updated_at, c.id) < ($12::timestamptz, $13::bigint)
    )
  )
ORDER BY c.updated_at DESC, c.id DESC
LIMIT $14::integer;

\o /tmp/p3-c06-list-generic-plan.json
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
EXECUTE list_customers(
  '2026-08-13T00:00:00Z', 'kw017', 7, 3, 5, 11, false,
  '2026-04-01T00:00:00Z', '2026-07-31T23:59:59Z',
  '2026-05-01T00:00:00Z', '2026-08-11T23:59:59Z',
  NULL, NULL, 201
);
\o /tmp/p3-c06-list-prepared.txt
SELECT name, generic_plans, custom_plans
FROM pg_prepared_statements
WHERE name = 'list_customers';
\o
