-- name: GetMediaImageVariant :one
SELECT image.id,
       image.file_name,
       image.mime_type,
       image.file_size,
       image.width,
       image.height,
       image.checksum AS image_checksum,
       blob.checksum AS blob_checksum,
       blob.content
FROM media_images AS image
JOIN media_image_blobs AS blob ON blob.image_id = image.id
WHERE image.id = sqlc.arg(image_id)::bigint;
