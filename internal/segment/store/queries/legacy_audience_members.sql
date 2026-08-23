-- name: LegacyAudiencePackageExists :one
WITH locked_package AS MATERIALIZED (
  SELECT segment.id
  FROM public.segments AS segment
  JOIN public.ai_audience_package_metadata AS metadata
    ON metadata.segment_id = segment.id
  WHERE segment.id = sqlc.arg(package_id)::bigint
  FOR SHARE OF segment
)
SELECT EXISTS (SELECT 1 FROM locked_package);

-- name: ListLegacyAudienceMembers :many
WITH requested_page AS (
  SELECT
    member.customer_id,
    customer.name,
    member.computed_at
  FROM public.segment_members AS member
  JOIN public.customers AS customer
    ON customer.id = member.customer_id
  WHERE member.segment_id = sqlc.arg(package_id)::bigint
  ORDER BY member.computed_at DESC, member.customer_id DESC
  LIMIT sqlc.arg(row_limit)::integer
  OFFSET sqlc.arg(row_offset)::bigint
), snapshot_total AS (
  SELECT count(*)::bigint AS total
  FROM public.segment_members
  WHERE segment_id = sqlc.arg(package_id)::bigint
)
SELECT
  snapshot_total.total,
  requested_page.customer_id,
  requested_page.name,
  requested_page.computed_at
FROM snapshot_total
LEFT JOIN requested_page ON TRUE
ORDER BY requested_page.computed_at DESC NULLS LAST,
         requested_page.customer_id DESC NULLS LAST;
