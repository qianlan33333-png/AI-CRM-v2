-- name: InsertHistoricalRadarDraft :one
INSERT INTO public.radar_links (
    public_code, name, title, destination_url, status, version,
    created_by, updated_by, created_at, updated_at
) VALUES (
    sqlc.arg(public_code)::text,
    sqlc.arg(name)::text,
    sqlc.arg(title)::text,
    sqlc.arg(destination_url)::text,
    'draft', 1,
    sqlc.arg(actor_id)::bigint,
    sqlc.arg(actor_id)::bigint,
    sqlc.arg(created_at)::timestamptz,
    sqlc.arg(updated_at)::timestamptz
)
ON CONFLICT (public_code) DO NOTHING
RETURNING id, public_code, name, title, destination_url, cover_image_id,
          attachment_id, status, version, created_by, updated_by, created_at, updated_at;

-- name: GetHistoricalRadarDraftByCode :one
SELECT id, public_code, name, title, destination_url, cover_image_id,
       attachment_id, status, version, created_by, updated_by, created_at, updated_at
FROM public.radar_links
WHERE public_code = sqlc.arg(public_code)::text
FOR KEY SHARE;
