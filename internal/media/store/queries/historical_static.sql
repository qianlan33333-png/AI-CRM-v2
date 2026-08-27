-- name: InsertHistoricalStaticImage :one
WITH inserted AS (
  INSERT INTO public.media_images
    (name,file_name,mime_type,file_size,width,height,checksum,description,tags,category,enabled,created_by,created_at,updated_at)
  VALUES
    (sqlc.arg(name)::text,sqlc.arg(file_name)::text,sqlc.arg(mime_type)::text,sqlc.arg(file_size)::integer,sqlc.arg(width)::integer,sqlc.arg(height)::integer,sqlc.arg(checksum)::bytea,sqlc.arg(description)::text,sqlc.arg(tags)::text,sqlc.arg(category)::text,FALSE,sqlc.arg(created_by)::bigint,sqlc.arg(created_at)::timestamptz,sqlc.arg(updated_at)::timestamptz)
  RETURNING id
)
INSERT INTO public.media_image_blobs (image_id,content,checksum,created_at)
SELECT id,sqlc.arg(content)::bytea,sqlc.arg(checksum)::bytea,sqlc.arg(created_at)::timestamptz
FROM inserted
RETURNING image_id;

-- name: InsertHistoricalStaticAttachment :one
WITH inserted AS (
  INSERT INTO public.media_attachments
    (name,file_name,mime_type,file_size,checksum,description,tags,enabled,version,created_by,updated_by,created_at,updated_at)
  VALUES
    (sqlc.arg(name)::text,sqlc.arg(file_name)::text,sqlc.arg(mime_type)::text,sqlc.arg(file_size)::integer,sqlc.arg(checksum)::bytea,sqlc.arg(description)::text,sqlc.arg(tags)::jsonb,FALSE,1,sqlc.arg(actor)::bigint,sqlc.arg(actor)::bigint,sqlc.arg(created_at)::timestamptz,sqlc.arg(updated_at)::timestamptz)
  RETURNING id
)
INSERT INTO public.media_attachment_blobs (attachment_id,content,checksum,created_at)
SELECT id,sqlc.arg(content)::bytea,sqlc.arg(checksum)::bytea,sqlc.arg(created_at)::timestamptz
FROM inserted
RETURNING attachment_id;
