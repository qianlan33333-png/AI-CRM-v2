-- name: CreateHistoricalUnboundTag :one
INSERT INTO contact_v1_unbound_tag_history (source_key_digest, source_payload_digest, source_field_digest, private_digest, redacted_roots, tag_source_id, union_id_digest, created_at, quarantine_reason) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetHistoricalUnboundTag :one
SELECT * FROM contact_v1_unbound_tag_history WHERE id=$1;

-- name: CountHistoricalUnboundTag :one
SELECT count(*) FROM contact_v1_unbound_tag_history;

-- name: ListHistoricalUnboundTag :many
SELECT * FROM contact_v1_unbound_tag_history ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CreateHistoricalInvalidChannel :one
INSERT INTO contact_v1_invalid_channel_history (source_key_digest, source_payload_digest, source_field_digest, private_digest, redacted_roots, source_id, code, name, channel_type, carrier_type, created_at, updated_at, quarantine_reason) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING *;

-- name: GetHistoricalInvalidChannel :one
SELECT * FROM contact_v1_invalid_channel_history WHERE id=$1;

-- name: CountHistoricalInvalidChannel :one
SELECT count(*) FROM contact_v1_invalid_channel_history;

-- name: ListHistoricalInvalidChannel :many
SELECT * FROM contact_v1_invalid_channel_history ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);
