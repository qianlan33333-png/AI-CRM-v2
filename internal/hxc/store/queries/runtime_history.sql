-- name: CreateHistoricalHXCSenderConfig :one
INSERT INTO hxc_v1_sender_config_history (source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, priority, original_is_active, created_at, updated_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_field_digest), sqlc.arg(private_digest), sqlc.arg(priority), sqlc.arg(original_is_active), sqlc.arg(created_at), sqlc.arg(updated_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, priority, original_is_active, created_at, updated_at;

-- name: GetHistoricalHXCSenderConfig :one
SELECT id, source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, priority, original_is_active, created_at, updated_at FROM hxc_v1_sender_config_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalHXCSenderConfig :many
SELECT id, source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, priority, original_is_active, created_at, updated_at FROM hxc_v1_sender_config_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalHXCSenderConfig :one
SELECT count(*)::bigint FROM hxc_v1_sender_config_history;

-- name: CreateHistoricalHXCSendRecord :one
INSERT INTO hxc_v1_send_record_history (source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, task_type, original_status, selected_count, eligible_count, sent_count, skipped_count, planned_count, queued_count, dispatching_count, succeeded_count, failed_count, blocked_count, cancelled_count, image_count, include_do_not_disturb, target_source, target_source_id, created_at, last_status_sync_at, last_refreshed_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_field_digest), sqlc.arg(private_digest), sqlc.arg(task_type), sqlc.arg(original_status), sqlc.arg(selected_count), sqlc.arg(eligible_count), sqlc.arg(sent_count), sqlc.arg(skipped_count), sqlc.arg(planned_count), sqlc.arg(queued_count), sqlc.arg(dispatching_count), sqlc.arg(succeeded_count), sqlc.arg(failed_count), sqlc.arg(blocked_count), sqlc.arg(cancelled_count), sqlc.arg(image_count), sqlc.arg(include_do_not_disturb), sqlc.arg(target_source), sqlc.narg(target_source_id), sqlc.arg(created_at), sqlc.narg(last_status_sync_at), sqlc.narg(last_refreshed_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, task_type, original_status, selected_count, eligible_count, sent_count, skipped_count, planned_count, queued_count, dispatching_count, succeeded_count, failed_count, blocked_count, cancelled_count, image_count, include_do_not_disturb, target_source, target_source_id, created_at, last_status_sync_at, last_refreshed_at;

-- name: GetHistoricalHXCSendRecord :one
SELECT id, source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, task_type, original_status, selected_count, eligible_count, sent_count, skipped_count, planned_count, queued_count, dispatching_count, succeeded_count, failed_count, blocked_count, cancelled_count, image_count, include_do_not_disturb, target_source, target_source_id, created_at, last_status_sync_at, last_refreshed_at FROM hxc_v1_send_record_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalHXCSendRecord :many
SELECT id, source_id, source_key_digest, source_payload_digest, source_field_digest, private_digest, task_type, original_status, selected_count, eligible_count, sent_count, skipped_count, planned_count, queued_count, dispatching_count, succeeded_count, failed_count, blocked_count, cancelled_count, image_count, include_do_not_disturb, target_source, target_source_id, created_at, last_status_sync_at, last_refreshed_at FROM hxc_v1_send_record_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalHXCSendRecord :one
SELECT count(*)::bigint FROM hxc_v1_send_record_history;
