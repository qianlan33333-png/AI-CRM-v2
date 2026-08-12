-- name: ListTags :many
SELECT
  t.id,
  t.group_id,
  g.name AS group_name,
  g.sort_order AS group_sort_order,
  t.name,
  t.sort_order
FROM tags AS t
LEFT JOIN tag_groups AS g ON g.id = t.group_id
ORDER BY
  (t.group_id IS NULL),
  g.sort_order,
  g.id,
  t.sort_order,
  t.id;
