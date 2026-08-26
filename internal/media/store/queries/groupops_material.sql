-- name: LockGroupOpsImageSource :one
SELECT image.id, image.file_name, image.mime_type, image.checksum AS source_checksum,
       blob.checksum AS blob_checksum, blob.content
FROM public.media_images AS image
JOIN public.media_image_blobs AS blob ON blob.image_id = image.id
WHERE image.id = sqlc.arg(image_id)::bigint AND image.enabled = TRUE
FOR UPDATE OF image, blob;

-- name: LockGroupOpsAttachmentSource :one
SELECT attachment.id, attachment.file_name, attachment.mime_type, attachment.checksum AS source_checksum,
       blob.checksum AS blob_checksum, blob.content
FROM public.media_attachments AS attachment
JOIN public.media_attachment_blobs AS blob ON blob.attachment_id = attachment.id
WHERE attachment.id = sqlc.arg(attachment_id)::bigint AND attachment.enabled = TRUE
FOR UPDATE OF attachment, blob;

-- name: LockGroupOpsMiniProgramSource :one
SELECT id, app_id, page_path, title, thumbnail_image_id
FROM public.media_miniprograms
WHERE id = sqlc.arg(miniprogram_id)::bigint AND enabled = TRUE
FOR UPDATE;

-- name: LockGroupOpsGroupInviteSource :one
SELECT id, title, description, join_url
FROM public.media_group_invites
WHERE id = sqlc.arg(group_invite_id)::bigint AND enabled = TRUE AND archived_at IS NULL
FOR UPDATE;

-- name: ReadGroupOpsPreparedUpload :one
SELECT preparation.source_kind, preparation.source_id, preparation.source_digest,
       receipt.receipt_digest, receipt.provider_media_id, receipt.expires_at
FROM public.media_wecom_upload_preparations AS preparation
JOIN public.media_wecom_upload_receipts AS receipt
  ON receipt.preparation_id = preparation.id
WHERE preparation.source_kind = sqlc.arg(source_kind)::text
  AND preparation.source_id = sqlc.arg(source_id)::bigint
  AND preparation.source_digest = sqlc.arg(source_digest)::text
  AND preparation.provider_scope_digest = sqlc.arg(provider_scope_digest)::text
  AND preparation.state = 'ready'
  AND receipt.expires_at > sqlc.arg(required_through)::timestamptz
ORDER BY receipt.expires_at DESC, preparation.id DESC
LIMIT 1
FOR KEY SHARE OF preparation, receipt;

-- name: InsertGroupOpsUploadPreparation :one
INSERT INTO public.media_wecom_upload_preparations (
  source_kind, source_id, source_digest, provider_scope_digest, upload_kind,
  external_effect_id, created_at, updated_at
) VALUES (
  sqlc.arg(source_kind)::text, sqlc.arg(source_id)::bigint,
  sqlc.arg(source_digest)::text, sqlc.arg(provider_scope_digest)::text,
  sqlc.arg(upload_kind)::text, sqlc.arg(external_effect_id)::bigint,
  sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz
)
ON CONFLICT DO NOTHING
RETURNING id, source_kind, source_id, source_digest, provider_scope_digest,
          upload_kind, external_effect_id, state;

-- name: GetGroupOpsUploadPreparation :one
SELECT id, source_kind, source_id, source_digest, provider_scope_digest,
       upload_kind, external_effect_id, state
FROM public.media_wecom_upload_preparations
WHERE external_effect_id = sqlc.arg(external_effect_id)::bigint;
