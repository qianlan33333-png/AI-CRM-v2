-- name: CreateHistoricalInvalidAsset :one
INSERT INTO media_v1_invalid_asset_history (source_key_digest, source_payload_digest, source_field_digest, private_digest, redacted_roots, kind, source_id, name, file_name, mime_type, file_size, original_enabled, content_digest, created_at, updated_at, quarantine_reason) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *;

-- name: GetHistoricalInvalidAsset :one
SELECT * FROM media_v1_invalid_asset_history WHERE id=$1;

-- name: CountHistoricalInvalidAsset :one
SELECT count(*) FROM media_v1_invalid_asset_history;

-- name: ListHistoricalInvalidAsset :many
SELECT * FROM media_v1_invalid_asset_history ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
