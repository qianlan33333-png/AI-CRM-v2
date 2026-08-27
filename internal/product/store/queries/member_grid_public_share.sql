-- name: CurrentMemberGridExternalShare :one
SELECT
  s.service_product_id,
  COALESCE(s.share_id, '') AS share_id,
  s.enabled,
  s.version
FROM public.service_period_member_grid_external_shares AS s
WHERE s.service_product_id = $1;

-- name: SetMemberGridExternalShare :one
INSERT INTO public.service_period_member_grid_external_shares AS s (
  service_product_id,
  share_id,
  enabled,
  version,
  updated_by,
  created_at,
  updated_at
) VALUES (
  sqlc.arg(service_product_id),
  NULLIF(sqlc.arg(share_id)::text, ''),
  sqlc.arg(enabled),
  1,
  sqlc.arg(updated_by),
  CURRENT_TIMESTAMP,
  CURRENT_TIMESTAMP
)
ON CONFLICT (service_product_id) DO UPDATE
SET share_id = EXCLUDED.share_id,
    enabled = EXCLUDED.enabled,
    version = s.version + 1,
    updated_by = EXCLUDED.updated_by,
    updated_at = CURRENT_TIMESTAMP
WHERE s.version = sqlc.arg(expected_version)
RETURNING service_product_id, COALESCE(share_id, '') AS share_id, enabled, version;

-- name: LookupEnabledMemberGridExternalShare :one
SELECT
  s.service_product_id,
  COALESCE(s.share_id, '') AS share_id,
  s.enabled,
  s.version
FROM public.service_period_member_grid_external_shares AS s
WHERE s.share_id = sqlc.arg(share_id)::text
  AND s.enabled = TRUE;

-- name: SummarizePublicServicePeriodMembers :many
SELECT m.state, COUNT(*)::bigint AS member_count
FROM public.service_period_members AS m
WHERE m.service_product_id = sqlc.arg(service_product_id)
  AND m.state IN ('active', 'expired', 'removed')
GROUP BY m.state
ORDER BY m.state ASC;

-- name: ListPublicServicePeriodMembersFirstPage :many
SELECT
  m.member_ref,
  m.state,
  m.source,
  m.starts_at,
  m.expires_at,
  m.updated_at,
  c.name AS display_name
FROM public.service_period_members AS m
JOIN public.customers AS c ON c.id = m.customer_id
WHERE m.service_product_id = sqlc.arg(service_product_id)
ORDER BY m.updated_at DESC, m.member_ref DESC
LIMIT sqlc.arg(row_limit);

-- name: ListPublicServicePeriodMembersAfter :many
SELECT
  m.member_ref,
  m.state,
  m.source,
  m.starts_at,
  m.expires_at,
  m.updated_at,
  c.name AS display_name
FROM public.service_period_members AS m
JOIN public.customers AS c ON c.id = m.customer_id
WHERE m.service_product_id = sqlc.arg(service_product_id)
  AND (m.updated_at, m.member_ref) < (
    sqlc.arg(after_updated_at)::timestamptz,
    sqlc.arg(after_member_ref)::text
  )
ORDER BY m.updated_at DESC, m.member_ref DESC
LIMIT sqlc.arg(row_limit);
