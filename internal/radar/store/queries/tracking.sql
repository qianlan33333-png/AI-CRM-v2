-- name: GetEnabledRadarLinkByCode :one
SELECT id, public_code, name, title, destination_url, cover_image_id,
       attachment_id, status, version, created_by, updated_by, created_at, updated_at
FROM public.radar_links
WHERE public_code = sqlc.arg(public_code)::text
  AND status = 'enabled';

-- name: InsertRadarLinkEvent :one
INSERT INTO public.radar_link_events (
    receipt_id, link_id, stage, page_no, source, key_digest, payload_digest, created_at
) VALUES (
    sqlc.arg(receipt_id)::text,
    sqlc.arg(link_id)::bigint,
    sqlc.arg(stage)::text,
    sqlc.narg(page_no)::integer,
    sqlc.arg(source)::text,
    sqlc.narg(key_digest)::bytea,
    sqlc.arg(payload_digest)::bytea,
    sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (link_id, key_digest) DO NOTHING
RETURNING id, receipt_id, link_id, stage, page_no, source, key_digest,
          payload_digest, created_at;

-- name: GetRadarLinkEventByKey :one
SELECT id, receipt_id, link_id, stage, page_no, source, key_digest,
       payload_digest, created_at
FROM public.radar_link_events
WHERE link_id = sqlc.arg(link_id)::bigint
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CountRadarLinkEvents :one
SELECT count(*)::bigint
FROM public.radar_link_events
WHERE link_id = sqlc.arg(link_id)::bigint
  AND (sqlc.narg(stage)::text IS NULL OR stage = sqlc.narg(stage)::text)
  AND (sqlc.narg(start_at)::timestamptz IS NULL OR created_at >= sqlc.narg(start_at)::timestamptz)
  AND (sqlc.narg(end_at)::timestamptz IS NULL OR created_at <= sqlc.narg(end_at)::timestamptz);

-- name: ListRadarLinkEvents :many
SELECT id, receipt_id, link_id, stage, page_no, source, created_at
FROM public.radar_link_events
WHERE link_id = sqlc.arg(link_id)::bigint
  AND (sqlc.narg(stage)::text IS NULL OR stage = sqlc.narg(stage)::text)
  AND (sqlc.narg(start_at)::timestamptz IS NULL OR created_at >= sqlc.narg(start_at)::timestamptz)
  AND (sqlc.narg(end_at)::timestamptz IS NULL OR created_at <= sqlc.narg(end_at)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: GetRadarLinkEventStats :one
SELECT count(*)::bigint AS total_events,
       count(*) FILTER (WHERE stage = 'landing')::bigint AS total_landings,
       count(*) FILTER (WHERE stage = 'redirect')::bigint AS redirects,
       count(*) FILTER (WHERE stage = 'viewer_open')::bigint AS viewer_opens,
       count(*) FILTER (WHERE stage = 'image_loaded')::bigint AS image_loaded,
       count(*) FILTER (WHERE stage = 'pdf_opened')::bigint AS pdf_opened,
       count(*) FILTER (WHERE stage = 'landing' AND created_at >= date_trunc('day', now()))::bigint AS today_landings,
       COALESCE(EXTRACT(EPOCH FROM max(created_at) FILTER (WHERE stage = 'landing'))::bigint, 0)::bigint AS last_clicked_epoch,
       COALESCE(EXTRACT(EPOCH FROM max(created_at))::bigint, 0)::bigint AS last_event_epoch,
       COALESCE(EXTRACT(EPOCH FROM max(created_at) FILTER (WHERE stage IN ('viewer_open', 'image_loaded', 'pdf_opened')))::bigint, 0)::bigint AS last_viewed_epoch
FROM public.radar_link_events
WHERE link_id = sqlc.arg(link_id)::bigint;

-- name: ListEnabledRadarLinksForSidebar :many
SELECT id, public_code, title, updated_at
FROM public.radar_links
WHERE status = 'enabled'
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountEnabledRadarLinksForSidebar :one
SELECT count(*)::bigint FROM public.radar_links WHERE status = 'enabled';
