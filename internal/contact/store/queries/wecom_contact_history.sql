-- name: CreateHistoricalWeComExternalContactEventLog :one
INSERT INTO contact_v1_wecom_event_log_history (source_key_digest, source_payload_digest, source_field_digest, source_id, corp_id_digest, event_type, change_type, external_user_id_digest, user_id_digest, event_time, event_key_digest, payload_xml_digest, payload_json_digest, process_status, retry_count, error_message_digest, created_at, updated_at, identity_sync_status, identity_sync_error_code_digest, identity_sync_error_message_digest, identity_sync_response_digest)
VALUES (sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea, sqlc.arg(source_field_digest)::bytea, sqlc.arg(source_id)::bigint, sqlc.arg(corp_id_digest)::bytea, sqlc.arg(event_type)::text, sqlc.arg(change_type)::text, sqlc.arg(external_user_id_digest)::bytea, sqlc.arg(user_id_digest)::bytea, sqlc.narg(event_time)::bigint, sqlc.arg(event_key_digest)::bytea, sqlc.arg(payload_xml_digest)::bytea, sqlc.arg(payload_json_digest)::bytea, sqlc.arg(process_status)::text, sqlc.arg(retry_count)::integer, sqlc.arg(error_message_digest)::bytea, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz, sqlc.arg(identity_sync_status)::text, sqlc.arg(identity_sync_error_code_digest)::bytea, sqlc.arg(identity_sync_error_message_digest)::bytea, sqlc.arg(identity_sync_response_digest)::bytea) RETURNING *;

-- name: GetHistoricalWeComExternalContactEventLog :one
SELECT * FROM contact_v1_wecom_event_log_history WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalWeComExternalContactEventLog :many
SELECT * FROM contact_v1_wecom_event_log_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalWeComExternalContactEventLog :one
SELECT count(*)::bigint FROM contact_v1_wecom_event_log_history;

-- name: CreateHistoricalWeComExternalContactFollowUser :one
INSERT INTO contact_v1_wecom_follow_user_history (source_key_digest, source_payload_digest, source_field_digest, source_id, corp_id_digest, external_user_id_digest, user_id_digest, relation_status, is_primary, remark_digest, description_digest, add_way, state, oper_user_id_digest, create_time, raw_follow_user_digest, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea, sqlc.arg(source_field_digest)::bytea, sqlc.arg(source_id)::bigint, sqlc.arg(corp_id_digest)::bytea, sqlc.arg(external_user_id_digest)::bytea, sqlc.arg(user_id_digest)::bytea, sqlc.arg(relation_status)::text, sqlc.arg(is_primary)::boolean, sqlc.arg(remark_digest)::bytea, sqlc.arg(description_digest)::bytea, sqlc.narg(add_way)::integer, sqlc.arg(state)::text, sqlc.arg(oper_user_id_digest)::bytea, sqlc.narg(create_time)::bigint, sqlc.arg(raw_follow_user_digest)::bytea, sqlc.arg(first_seen_at)::timestamptz, sqlc.arg(last_seen_at)::timestamptz, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz) RETURNING *;

-- name: GetHistoricalWeComExternalContactFollowUser :one
SELECT * FROM contact_v1_wecom_follow_user_history WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalWeComExternalContactFollowUser :many
SELECT * FROM contact_v1_wecom_follow_user_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalWeComExternalContactFollowUser :one
SELECT count(*)::bigint FROM contact_v1_wecom_follow_user_history;
