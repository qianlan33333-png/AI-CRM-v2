-- name: CreateHistoricalProductPageSlice :one
INSERT INTO product_v1_page_slice_history (source_id, source_key_digest, source_payload_digest, product_source_id, image_source_id, sort_order, original_enabled, created_at, updated_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(product_source_id), sqlc.arg(image_source_id), sqlc.arg(sort_order), sqlc.arg(original_enabled), sqlc.arg(created_at), sqlc.arg(updated_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, product_source_id, image_source_id, sort_order, original_enabled, created_at, updated_at;

-- name: GetHistoricalProductPageSlice :one
SELECT id, source_id, source_key_digest, source_payload_digest, product_source_id, image_source_id, sort_order, original_enabled, created_at, updated_at FROM product_v1_page_slice_history WHERE id=sqlc.arg(id);

-- name: ListHistoricalProductPageSlice :many
SELECT id, source_id, source_key_digest, source_payload_digest, product_source_id, image_source_id, sort_order, original_enabled, created_at, updated_at FROM product_v1_page_slice_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalProductPageSlice :one
SELECT count(*)::bigint FROM product_v1_page_slice_history;
