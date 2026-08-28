-- name: CreateHistoricalExternalContactBinding :one
INSERT INTO contact_v1_external_binding_history (source_key_digest, source_payload_digest, source_field_digest, external_user_id_digest, source_person_id, person_history_id, identity_id, identity_assurance, first_bound_by_user_id_digest, first_owner_user_id_digest, last_owner_user_id_digest, created_at, updated_at)
VALUES (sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea, sqlc.arg(source_field_digest)::bytea, sqlc.arg(external_user_id_digest)::bytea, sqlc.arg(source_person_id)::bigint, sqlc.narg(person_history_id)::bigint, sqlc.narg(identity_id)::bigint, sqlc.arg(identity_assurance)::text, sqlc.arg(first_bound_by_user_id_digest)::bytea, sqlc.arg(first_owner_user_id_digest)::bytea, sqlc.arg(last_owner_user_id_digest)::bytea, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz)
RETURNING *;

-- name: GetHistoricalExternalContactBinding :one
SELECT * FROM contact_v1_external_binding_history WHERE id = sqlc.arg(id)::bigint;

-- name: ListHistoricalExternalContactBinding :many
SELECT * FROM contact_v1_external_binding_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalExternalContactBinding :one
SELECT count(*)::bigint FROM contact_v1_external_binding_history;

-- name: CreateHistoricalWeComDirectoryMember :one
INSERT INTO contact_v1_directory_member_history (source_key_digest, source_payload_digest, source_field_digest, source_id, wecom_corp_id_digest, corp_id_digest, wecom_user_id_digest, corp_attribution, matched_staff_id, display_name, department_ids_digest, department_name, position, wecom_status, is_active, synced_at, raw_payload_digest, mobile_digest, avatar_url_digest, updated_by_digest, first_seen_at, last_synced_at, created_at, updated_at)
VALUES (sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea, sqlc.arg(source_field_digest)::bytea, sqlc.arg(source_id)::bigint, sqlc.arg(wecom_corp_id_digest)::bytea, sqlc.arg(corp_id_digest)::bytea, sqlc.arg(wecom_user_id_digest)::bytea, sqlc.arg(corp_attribution)::text, sqlc.narg(matched_staff_id)::bigint, sqlc.arg(display_name)::text, sqlc.arg(department_ids_digest)::bytea, sqlc.arg(department_name)::text, sqlc.arg(position)::text, sqlc.narg(wecom_status)::integer, sqlc.arg(is_active)::boolean, sqlc.arg(synced_at)::timestamptz, sqlc.arg(raw_payload_digest)::bytea, sqlc.arg(mobile_digest)::bytea, sqlc.arg(avatar_url_digest)::bytea, sqlc.arg(updated_by_digest)::bytea, sqlc.arg(first_seen_at)::timestamptz, sqlc.arg(last_synced_at)::timestamptz, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz)
RETURNING *;

-- name: GetHistoricalWeComDirectoryMember :one
SELECT * FROM contact_v1_directory_member_history WHERE id = sqlc.arg(id)::bigint;

-- name: ListHistoricalWeComDirectoryMember :many
SELECT * FROM contact_v1_directory_member_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalWeComDirectoryMember :one
SELECT count(*)::bigint FROM contact_v1_directory_member_history;
