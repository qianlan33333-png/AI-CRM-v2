-- name: LookupActiveMemberGridCollaboratorPermission :one
SELECT c.permission
FROM public.service_period_member_grid_collaborators AS c
JOIN public.staff AS s ON s.id = c.staff_id AND s.is_active = TRUE
WHERE c.service_product_id = sqlc.arg(service_product_id)
  AND c.staff_id = sqlc.arg(staff_id);

-- name: ListSelectedMemberGridMembers :many
WITH selected AS (
  SELECT
    m.member_ref,
    m.service_product_id,
    m.customer_id,
    m.state,
    m.source,
    m.starts_at,
    m.expires_at,
    m.expired_at,
    m.removed_at,
    m.version,
    m.updated_at,
    c.name AS display_name,
    CASE
      WHEN sqlc.arg(group_by)::text = 'state' THEN CASE m.state
        WHEN 'active' THEN 1
        WHEN 'expired' THEN 2
        WHEN 'removed' THEN 3
        ELSE 4
      END
      ELSE 0
    END AS group_rank,
    CASE
      WHEN sqlc.arg(sort)::text = 'starts_at_desc' THEN m.starts_at
      ELSE m.updated_at
    END AS sort_at
  FROM public.service_period_members AS m
  JOIN public.customers AS c ON c.id = m.customer_id
  WHERE m.service_product_id = sqlc.arg(service_product_id)
    AND (sqlc.arg(state)::text = 'all' OR m.state = sqlc.arg(state)::text)
    AND (sqlc.arg(source)::text = '' OR m.source = sqlc.arg(source)::text)
)
SELECT member_ref, service_product_id, customer_id, state, source,
  starts_at, expires_at, expired_at, removed_at, version, updated_at, display_name
FROM selected
WHERE NOT sqlc.arg(has_after)::boolean
   OR group_rank > sqlc.arg(after_group_rank)::integer
   OR (group_rank = sqlc.arg(after_group_rank)::integer
       AND (sort_at, member_ref) < (sqlc.arg(after_sort_at)::timestamptz, sqlc.arg(after_member_ref)::text))
ORDER BY group_rank ASC, sort_at DESC, member_ref DESC
LIMIT sqlc.arg(row_limit);
