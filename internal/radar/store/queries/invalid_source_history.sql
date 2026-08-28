-- name: CreateHistoricalInvalidRadarLink :one
INSERT INTO radar_v1_invalid_link_history (source_key_digest, source_payload_digest, source_field_digest, private_digest, redacted_roots, source_id, code, title, destination_url_digest, created_at, updated_at, quarantine_reason) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: GetHistoricalInvalidRadarLink :one
SELECT * FROM radar_v1_invalid_link_history WHERE id=$1;

-- name: CountHistoricalInvalidRadarLink :one
SELECT count(*) FROM radar_v1_invalid_link_history;

-- name: ListHistoricalInvalidRadarLink :many
SELECT * FROM radar_v1_invalid_link_history ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
