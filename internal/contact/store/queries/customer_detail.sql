-- name: GetCustomerDetailSnapshot :many
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
  c.updated_at,
  t.id AS tag_id,
  t.group_id,
  g.name AS group_name,
  COALESCE(g.sort_order, 0) AS group_sort_order,
  t.name AS tag_name,
  t.sort_order AS tag_sort_order
FROM customers AS c
LEFT JOIN customer_tags AS ct ON ct.customer_id = c.id
LEFT JOIN tags AS t ON t.id = ct.tag_id
LEFT JOIN tag_groups AS g ON g.id = t.group_id
WHERE c.id = sqlc.arg(customer_id)::bigint
  AND NOT c.is_deleted
  AND (
    sqlc.narg(owner_staff_id)::bigint IS NULL
    OR c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint
  )
ORDER BY COALESCE(g.sort_order, 0), t.sort_order, t.id;
