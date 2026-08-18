-- name: GetMediaImageDetail :one
SELECT image.id,
       image.name,
       image.file_name,
       image.mime_type,
       image.file_size,
       image.enabled,
       image.description,
       image.tags,
       image.category,
       image.width,
       image.height,
       image.created_at,
       image.updated_at,
       image.checksum AS image_checksum,
       blob.checksum AS blob_checksum,
       blob.content
FROM media_images AS image
JOIN media_image_blobs AS blob ON blob.image_id = image.id
WHERE image.id = sqlc.arg(image_id)::bigint;

-- name: LockMediaImageMetadata :one
SELECT image.id,
       image.name,
       image.file_name,
       image.mime_type,
       image.file_size,
       image.enabled,
       image.description,
       image.tags,
       image.category,
       image.width,
       image.height,
       image.created_at,
       image.updated_at
FROM media_images AS image
WHERE image.id = sqlc.arg(image_id)::bigint
FOR UPDATE;

-- name: UpdateMediaImageMetadata :one
UPDATE media_images
SET name = sqlc.arg(name)::text,
    description = sqlc.arg(description)::text,
    tags = sqlc.arg(tags)::text,
    category = sqlc.arg(category)::text,
    enabled = sqlc.arg(enabled)::boolean,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(image_id)::bigint
RETURNING id,
          name,
          file_name,
          mime_type,
          file_size,
          enabled,
          description,
          tags,
          category,
          width,
          height,
          created_at,
          updated_at;
