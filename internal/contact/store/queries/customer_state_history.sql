-- name: CreateHistoricalCustomerStatusSnapshot :one
INSERT INTO contact_v1_customer_status_snapshots (source_key_digest, source_payload_digest, source_field_digest, signup_status, signup_label_name, customer_name_snapshot, owner_userid_snapshot, set_by_userid_digest, set_at, wecom_tag_sync_status, wecom_tag_sync_error_hash, status_flags_digest, created_at, updated_at, unionid)
VALUES (sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea, sqlc.arg(source_field_digest)::bytea, sqlc.arg(signup_status)::text, sqlc.arg(signup_label_name)::text, sqlc.arg(customer_name_snapshot)::text, sqlc.arg(owner_userid_snapshot)::text, sqlc.arg(set_by_userid_digest)::bytea, sqlc.arg(set_at)::timestamptz, sqlc.arg(wecom_tag_sync_status)::text, sqlc.arg(wecom_tag_sync_error_hash)::bytea, sqlc.arg(status_flags_digest)::bytea, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz, sqlc.arg(unionid)::text) RETURNING *;

-- name: GetHistoricalCustomerStatusSnapshot :one
SELECT * FROM contact_v1_customer_status_snapshots WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalCustomerStatusSnapshot :many
SELECT * FROM contact_v1_customer_status_snapshots ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalCustomerStatusSnapshot :one
SELECT count(*)::bigint FROM contact_v1_customer_status_snapshots;

-- name: CreateHistoricalCustomerStatusChange :one
INSERT INTO contact_v1_customer_status_changes (source_key_digest, source_payload_digest, source_field_digest, source_id, old_signup_status, new_signup_status, old_label_name, new_label_name, customer_name_snapshot, owner_userid_snapshot, set_by_userid_digest, set_at, wecom_tag_sync_status, wecom_tag_sync_error_hash, status_flags_digest, created_at, unionid)
VALUES (sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea, sqlc.arg(source_field_digest)::bytea, sqlc.arg(source_id)::bigint, sqlc.arg(old_signup_status)::text, sqlc.arg(new_signup_status)::text, sqlc.arg(old_label_name)::text, sqlc.arg(new_label_name)::text, sqlc.arg(customer_name_snapshot)::text, sqlc.arg(owner_userid_snapshot)::text, sqlc.arg(set_by_userid_digest)::bytea, sqlc.arg(set_at)::timestamptz, sqlc.arg(wecom_tag_sync_status)::text, sqlc.arg(wecom_tag_sync_error_hash)::bytea, sqlc.arg(status_flags_digest)::bytea, sqlc.arg(created_at)::timestamptz, sqlc.arg(unionid)::text) RETURNING *;

-- name: GetHistoricalCustomerStatusChange :one
SELECT * FROM contact_v1_customer_status_changes WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalCustomerStatusChange :many
SELECT * FROM contact_v1_customer_status_changes ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalCustomerStatusChange :one
SELECT count(*)::bigint FROM contact_v1_customer_status_changes;

-- name: CreateHistoricalClassTermTagMapping :one
INSERT INTO contact_v1_class_term_tag_history (source_key_digest, source_payload_digest, source_field_digest, source_id, tag_group_name, tag_name, class_term_no, class_term_label, original_active, created_at, updated_at, strategy_source_id, group_source_id, tag_source_id)
VALUES (sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea, sqlc.arg(source_field_digest)::bytea, sqlc.arg(source_id)::bigint, sqlc.arg(tag_group_name)::text, sqlc.arg(tag_name)::text, sqlc.arg(class_term_no)::integer, sqlc.arg(class_term_label)::text, sqlc.arg(original_active)::boolean, sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz, sqlc.arg(strategy_source_id)::text, sqlc.arg(group_source_id)::text, sqlc.arg(tag_source_id)::text) RETURNING *;

-- name: GetHistoricalClassTermTagMapping :one
SELECT * FROM contact_v1_class_term_tag_history WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalClassTermTagMapping :many
SELECT * FROM contact_v1_class_term_tag_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalClassTermTagMapping :one
SELECT count(*)::bigint FROM contact_v1_class_term_tag_history;
