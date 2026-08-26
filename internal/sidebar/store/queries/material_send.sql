-- name: ReserveSidebarImageSend :one
INSERT INTO public.sidebar_image_temporary_media_receipts (
  actor_id, customer_id, image_id, key_digest, state
) VALUES ($1, $2, $3, $4, 'pending')
ON CONFLICT (actor_id, customer_id, key_digest) DO NOTHING
RETURNING id, image_id, state, media_id, media_expires_at, provider_call_dispatched;

-- name: GetSidebarImageSendByKey :one
SELECT id, image_id, state, media_id, media_expires_at, provider_call_dispatched
FROM public.sidebar_image_temporary_media_receipts
WHERE actor_id = $1 AND customer_id = $2 AND key_digest = $3;

-- name: CompleteSidebarImageSend :one
UPDATE public.sidebar_image_temporary_media_receipts
SET state = sqlc.arg(state),
    media_id = NULLIF(sqlc.arg(media_id)::text, ''),
    media_expires_at = sqlc.narg(media_expires_at),
    provider_call_dispatched = sqlc.arg(provider_call_dispatched),
    updated_at = now()
WHERE id = sqlc.arg(id) AND state = 'pending'
RETURNING id, image_id, state, media_id, media_expires_at, provider_call_dispatched;
