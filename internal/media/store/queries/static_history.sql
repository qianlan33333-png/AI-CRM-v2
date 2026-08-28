-- name: CreateHistoricalGroupInvite :one
INSERT INTO media_v1_group_invite_history (source_id, source_key_digest, source_payload_digest, name, title, description, original_state, original_auto_create, room_base_name, room_base_source_id, original_enabled, original_binding_state, created_at, updated_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(name), sqlc.arg(title), sqlc.arg(description), sqlc.arg(original_state), sqlc.arg(original_auto_create), sqlc.arg(room_base_name), sqlc.narg(room_base_source_id), sqlc.arg(original_enabled), sqlc.arg(original_binding_state), sqlc.arg(created_at), sqlc.arg(updated_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, name, title, description, original_state, original_auto_create, room_base_name, room_base_source_id, original_enabled, original_binding_state, created_at, updated_at;

-- name: GetHistoricalGroupInvite :one
SELECT id, source_id, source_key_digest, source_payload_digest, name, title, description, original_state, original_auto_create, room_base_name, room_base_source_id, original_enabled, original_binding_state, created_at, updated_at FROM media_v1_group_invite_history WHERE id=sqlc.arg(id);

-- name: ListHistoricalGroupInvite :many
SELECT id, source_id, source_key_digest, source_payload_digest, name, title, description, original_state, original_auto_create, room_base_name, room_base_source_id, original_enabled, original_binding_state, created_at, updated_at FROM media_v1_group_invite_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalGroupInvite :one
SELECT count(*)::bigint FROM media_v1_group_invite_history;
