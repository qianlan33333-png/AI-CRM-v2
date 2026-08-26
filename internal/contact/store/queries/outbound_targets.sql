-- name: ResolveVerifiedWeComOutboundTarget :one
SELECT
  s.wecom_userid AS sender_wecom_userid,
  min(i.normalized_value)::text AS external_userid
FROM public.customers AS c
JOIN public.staff AS s
  ON s.id = c.owner_staff_id
JOIN public.identities AS i
  ON i.customer_id = c.id
WHERE c.id = sqlc.arg(customer_id)::bigint
  AND c.is_deleted = FALSE
  AND s.is_active = TRUE
  AND btrim(s.wecom_userid) <> ''
  AND i.kind = 'wecom_external_userid'
  AND i.scope = 'wecom-corp:' || sqlc.arg(corp_id)::text
  AND i.assurance = 'verified'
  AND i.bound_at IS NOT NULL
GROUP BY s.id, s.wecom_userid
HAVING count(DISTINCT i.normalized_value) = 1;
