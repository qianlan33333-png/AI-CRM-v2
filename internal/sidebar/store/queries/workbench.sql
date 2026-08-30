-- name: ReadSidebarWorkbenchCounts :one
WITH recent_submissions AS MATERIALIZED (
  SELECT s.id, s.unionid, s.external_userid, s.mobile
  FROM public.questionnaire_submissions AS s
  ORDER BY s.submitted_at DESC, s.id DESC
  LIMIT 500
), resolved_submissions AS (
  SELECT recent.id,
         array_remove(ARRAY[
           CASE WHEN recent.external_userid = '' THEN NULL ELSE (
             SELECT CASE
               WHEN identity.customer_id IS NULL THEN NULL
               WHEN customer.id IS NULL OR customer.is_deleted THEN -1::bigint
               ELSE identity.customer_id
             END
             FROM public.identities AS identity
             LEFT JOIN public.customers AS customer ON customer.id = identity.customer_id
             WHERE identity.kind = 'wecom_external_userid'
               AND identity.scope = sqlc.arg(wecom_scope)::text
               AND identity.normalized_value = recent.external_userid
           ) END,
           CASE WHEN recent.mobile = '' THEN NULL ELSE (
             SELECT CASE
               WHEN identity.customer_id IS NULL THEN NULL
               WHEN customer.id IS NULL OR customer.is_deleted THEN -1::bigint
               ELSE identity.customer_id
             END
             FROM public.identities AS identity
             LEFT JOIN public.customers AS customer ON customer.id = identity.customer_id
             WHERE identity.kind = 'phone'
               AND identity.scope = 'phone:e164'
               AND identity.normalized_value = recent.mobile
           ) END,
           CASE WHEN recent.unionid = '' THEN NULL ELSE (
             SELECT CASE count(DISTINCT identity.customer_id)
               WHEN 0 THEN NULL
               WHEN 1 THEN min(identity.customer_id)
               ELSE -1::bigint
             END
             FROM public.identities AS identity
             JOIN public.customers AS customer
               ON customer.id = identity.customer_id AND NOT customer.is_deleted
             WHERE identity.kind = 'unionid'
               AND identity.assurance = 'verified'
               AND identity.normalized_value = recent.unionid
           ) END
         ], NULL)::bigint[] AS customer_ids
  FROM recent_submissions AS recent
)
SELECT
  COALESCE((
    SELECT count(*)
    FROM resolved_submissions AS resolved
    WHERE sqlc.arg(customer_id)::bigint = ANY(resolved.customer_ids)
      AND NOT EXISTS (
        SELECT 1 FROM unnest(resolved.customer_ids) AS candidate(customer_id)
        WHERE candidate.customer_id <> sqlc.arg(customer_id)::bigint
      )
  ), 0)::bigint AS questionnaire_count,
  COALESCE((
    SELECT count(*) FROM public.order_list_projections AS orders
    WHERE orders.customer_id = sqlc.arg(customer_id)::bigint
  ), 0)::bigint AS order_count,
  COALESCE((
    SELECT count(*) FROM public.service_period_members AS members
    WHERE members.customer_id = sqlc.arg(customer_id)::bigint
  ), 0)::bigint AS periodic_order_count,
  COALESCE((
    SELECT count(*) FROM public.media_images AS images
    WHERE images.enabled
  ), 0)::bigint AS material_count;
