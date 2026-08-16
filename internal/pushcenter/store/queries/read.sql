-- name: GetPushCenterReadModelState :one
SELECT production_data_ready, fixture_mode, allow_fixture_repo_in_prod
FROM public.push_center_read_model_state
WHERE singleton = TRUE;

-- name: CountPushCenterProjection :one
WITH filters AS (
  SELECT NULLIF(sqlc.arg(section)::text, '') AS section,
    NULLIF(sqlc.arg(effect_type)::text, '') AS effect_type,
    NULLIF(sqlc.arg(status)::text, '') AS status,
    NULLIF(sqlc.arg(business_type)::text, '') AS business_type,
    NULLIF(sqlc.arg(business_id)::text, '') AS business_id,
    NULLIF(sqlc.arg(target_type)::text, '') AS target_type,
    NULLIF(sqlc.arg(target_id)::text, '') AS target_id,
    NULLIF(sqlc.arg(external_userid)::text, '') AS external_userid,
    NULLIF(sqlc.arg(owner_userid)::text, '') AS owner_userid,
    NULLIF(sqlc.arg(trace_id)::text, '') AS trace_id,
    NULLIF(sqlc.arg(idempotency_key)::text, '') AS idempotency_key,
    NULLIF(sqlc.arg(source_module)::text, '') AS source_module,
    NULLIF(sqlc.arg(source_route)::text, '') AS source_route,
    NULLIF(sqlc.arg(created_from)::text, '')::timestamptz AS created_from,
    NULLIF(sqlc.arg(created_to)::text, '')::timestamptz AS created_to
)
SELECT count(*)::bigint
FROM public.push_center_read_model_entries entry CROSS JOIN filters
WHERE (filters.section IS NULL OR entry.section = filters.section)
  AND (filters.effect_type IS NULL OR entry.effect_type ILIKE '%' || filters.effect_type || '%')
  AND (filters.status IS NULL OR entry.status = filters.status OR (filters.status = 'sent' AND entry.status = 'sent_with_shadow_warning'))
  AND (filters.business_type IS NULL OR entry.business_type ILIKE '%' || filters.business_type || '%')
  AND (filters.business_id IS NULL OR entry.business_id ILIKE '%' || filters.business_id || '%')
  AND (filters.target_type IS NULL OR entry.target_type ILIKE '%' || filters.target_type || '%')
  AND (filters.target_id IS NULL OR entry.target_id ILIKE '%' || filters.target_id || '%')
  AND (filters.external_userid IS NULL OR entry.external_userid ILIKE '%' || filters.external_userid || '%')
  AND (filters.owner_userid IS NULL OR entry.owner_userid ILIKE '%' || filters.owner_userid || '%')
  AND (filters.trace_id IS NULL OR entry.trace_id ILIKE '%' || filters.trace_id || '%')
  AND (filters.idempotency_key IS NULL OR entry.idempotency_key ILIKE '%' || filters.idempotency_key || '%')
  AND (filters.source_module IS NULL OR entry.source_module ILIKE '%' || filters.source_module || '%')
  AND (filters.source_route IS NULL OR entry.source_route ILIKE '%' || filters.source_route || '%')
  AND (filters.created_from IS NULL OR entry.created_at >= filters.created_from)
  AND (filters.created_to IS NULL OR entry.created_at <= filters.created_to);

-- name: CountPushCenterProjectionByStatus :many
WITH filters AS (
  SELECT NULLIF(sqlc.arg(section)::text, '') AS section,
    NULLIF(sqlc.arg(effect_type)::text, '') AS effect_type,
    NULLIF(sqlc.arg(status)::text, '') AS status,
    NULLIF(sqlc.arg(business_type)::text, '') AS business_type,
    NULLIF(sqlc.arg(business_id)::text, '') AS business_id,
    NULLIF(sqlc.arg(target_type)::text, '') AS target_type,
    NULLIF(sqlc.arg(target_id)::text, '') AS target_id,
    NULLIF(sqlc.arg(external_userid)::text, '') AS external_userid,
    NULLIF(sqlc.arg(owner_userid)::text, '') AS owner_userid,
    NULLIF(sqlc.arg(trace_id)::text, '') AS trace_id,
    NULLIF(sqlc.arg(idempotency_key)::text, '') AS idempotency_key,
    NULLIF(sqlc.arg(source_module)::text, '') AS source_module,
    NULLIF(sqlc.arg(source_route)::text, '') AS source_route,
    NULLIF(sqlc.arg(created_from)::text, '')::timestamptz AS created_from,
    NULLIF(sqlc.arg(created_to)::text, '')::timestamptz AS created_to
)
SELECT entry.status, count(*)::bigint AS count
FROM public.push_center_read_model_entries entry CROSS JOIN filters
WHERE (filters.section IS NULL OR entry.section = filters.section)
  AND (filters.effect_type IS NULL OR entry.effect_type ILIKE '%' || filters.effect_type || '%')
  AND (filters.status IS NULL OR entry.status = filters.status OR (filters.status = 'sent' AND entry.status = 'sent_with_shadow_warning'))
  AND (filters.business_type IS NULL OR entry.business_type ILIKE '%' || filters.business_type || '%')
  AND (filters.business_id IS NULL OR entry.business_id ILIKE '%' || filters.business_id || '%')
  AND (filters.target_type IS NULL OR entry.target_type ILIKE '%' || filters.target_type || '%')
  AND (filters.target_id IS NULL OR entry.target_id ILIKE '%' || filters.target_id || '%')
  AND (filters.external_userid IS NULL OR entry.external_userid ILIKE '%' || filters.external_userid || '%')
  AND (filters.owner_userid IS NULL OR entry.owner_userid ILIKE '%' || filters.owner_userid || '%')
  AND (filters.trace_id IS NULL OR entry.trace_id ILIKE '%' || filters.trace_id || '%')
  AND (filters.idempotency_key IS NULL OR entry.idempotency_key ILIKE '%' || filters.idempotency_key || '%')
  AND (filters.source_module IS NULL OR entry.source_module ILIKE '%' || filters.source_module || '%')
  AND (filters.source_route IS NULL OR entry.source_route ILIKE '%' || filters.source_route || '%')
  AND (filters.created_from IS NULL OR entry.created_at >= filters.created_from)
  AND (filters.created_to IS NULL OR entry.created_at <= filters.created_to)
GROUP BY entry.status ORDER BY entry.status;

-- name: CountPushCenterProjectionByEffectiveStatus :many
WITH filters AS (
  SELECT NULLIF(sqlc.arg(section)::text, '') AS section,
    NULLIF(sqlc.arg(effect_type)::text, '') AS effect_type,
    NULLIF(sqlc.arg(status)::text, '') AS status,
    NULLIF(sqlc.arg(business_type)::text, '') AS business_type,
    NULLIF(sqlc.arg(business_id)::text, '') AS business_id,
    NULLIF(sqlc.arg(target_type)::text, '') AS target_type,
    NULLIF(sqlc.arg(target_id)::text, '') AS target_id,
    NULLIF(sqlc.arg(external_userid)::text, '') AS external_userid,
    NULLIF(sqlc.arg(owner_userid)::text, '') AS owner_userid,
    NULLIF(sqlc.arg(trace_id)::text, '') AS trace_id,
    NULLIF(sqlc.arg(idempotency_key)::text, '') AS idempotency_key,
    NULLIF(sqlc.arg(source_module)::text, '') AS source_module,
    NULLIF(sqlc.arg(source_route)::text, '') AS source_route,
    NULLIF(sqlc.arg(created_from)::text, '')::timestamptz AS created_from,
    NULLIF(sqlc.arg(created_to)::text, '')::timestamptz AS created_to
)
SELECT entry.effective_status, count(*)::bigint AS count
FROM public.push_center_read_model_entries entry CROSS JOIN filters
WHERE (filters.section IS NULL OR entry.section = filters.section)
  AND (filters.effect_type IS NULL OR entry.effect_type ILIKE '%' || filters.effect_type || '%')
  AND (filters.status IS NULL OR entry.status = filters.status OR (filters.status = 'sent' AND entry.status = 'sent_with_shadow_warning'))
  AND (filters.business_type IS NULL OR entry.business_type ILIKE '%' || filters.business_type || '%')
  AND (filters.business_id IS NULL OR entry.business_id ILIKE '%' || filters.business_id || '%')
  AND (filters.target_type IS NULL OR entry.target_type ILIKE '%' || filters.target_type || '%')
  AND (filters.target_id IS NULL OR entry.target_id ILIKE '%' || filters.target_id || '%')
  AND (filters.external_userid IS NULL OR entry.external_userid ILIKE '%' || filters.external_userid || '%')
  AND (filters.owner_userid IS NULL OR entry.owner_userid ILIKE '%' || filters.owner_userid || '%')
  AND (filters.trace_id IS NULL OR entry.trace_id ILIKE '%' || filters.trace_id || '%')
  AND (filters.idempotency_key IS NULL OR entry.idempotency_key ILIKE '%' || filters.idempotency_key || '%')
  AND (filters.source_module IS NULL OR entry.source_module ILIKE '%' || filters.source_module || '%')
  AND (filters.source_route IS NULL OR entry.source_route ILIKE '%' || filters.source_route || '%')
  AND (filters.created_from IS NULL OR entry.created_at >= filters.created_from)
  AND (filters.created_to IS NULL OR entry.created_at <= filters.created_to)
GROUP BY entry.effective_status ORDER BY entry.effective_status;

-- name: CountPushCenterProjectionBySection :many
WITH filters AS (
  SELECT NULLIF(sqlc.arg(section)::text, '') AS section,
    NULLIF(sqlc.arg(effect_type)::text, '') AS effect_type,
    NULLIF(sqlc.arg(status)::text, '') AS status,
    NULLIF(sqlc.arg(business_type)::text, '') AS business_type,
    NULLIF(sqlc.arg(business_id)::text, '') AS business_id,
    NULLIF(sqlc.arg(target_type)::text, '') AS target_type,
    NULLIF(sqlc.arg(target_id)::text, '') AS target_id,
    NULLIF(sqlc.arg(external_userid)::text, '') AS external_userid,
    NULLIF(sqlc.arg(owner_userid)::text, '') AS owner_userid,
    NULLIF(sqlc.arg(trace_id)::text, '') AS trace_id,
    NULLIF(sqlc.arg(idempotency_key)::text, '') AS idempotency_key,
    NULLIF(sqlc.arg(source_module)::text, '') AS source_module,
    NULLIF(sqlc.arg(source_route)::text, '') AS source_route,
    NULLIF(sqlc.arg(created_from)::text, '')::timestamptz AS created_from,
    NULLIF(sqlc.arg(created_to)::text, '')::timestamptz AS created_to
)
SELECT entry.section, count(*)::bigint AS count
FROM public.push_center_read_model_entries entry CROSS JOIN filters
WHERE (filters.section IS NULL OR entry.section = filters.section)
  AND (filters.effect_type IS NULL OR entry.effect_type ILIKE '%' || filters.effect_type || '%')
  AND (filters.status IS NULL OR entry.status = filters.status OR (filters.status = 'sent' AND entry.status = 'sent_with_shadow_warning'))
  AND (filters.business_type IS NULL OR entry.business_type ILIKE '%' || filters.business_type || '%')
  AND (filters.business_id IS NULL OR entry.business_id ILIKE '%' || filters.business_id || '%')
  AND (filters.target_type IS NULL OR entry.target_type ILIKE '%' || filters.target_type || '%')
  AND (filters.target_id IS NULL OR entry.target_id ILIKE '%' || filters.target_id || '%')
  AND (filters.external_userid IS NULL OR entry.external_userid ILIKE '%' || filters.external_userid || '%')
  AND (filters.owner_userid IS NULL OR entry.owner_userid ILIKE '%' || filters.owner_userid || '%')
  AND (filters.trace_id IS NULL OR entry.trace_id ILIKE '%' || filters.trace_id || '%')
  AND (filters.idempotency_key IS NULL OR entry.idempotency_key ILIKE '%' || filters.idempotency_key || '%')
  AND (filters.source_module IS NULL OR entry.source_module ILIKE '%' || filters.source_module || '%')
  AND (filters.source_route IS NULL OR entry.source_route ILIKE '%' || filters.source_route || '%')
  AND (filters.created_from IS NULL OR entry.created_at >= filters.created_from)
  AND (filters.created_to IS NULL OR entry.created_at <= filters.created_to)
GROUP BY entry.section ORDER BY entry.section;
