-- name: CreateHistoricalRadarClick :one
INSERT INTO radar_v1_click_history (source_key_digest, source_payload_digest, source_field_digest, source_id, link_source_id, radar_link_id, customer_id, code, raw_stage, source_channel, target_type_snapshot, source_channel_snapshot, error_code, created_at, open_id_digest, union_id_digest, external_user_id_digest, campaign_id_digest, staff_id_digest, user_agent_digest, ip_digest, person_id_digest, ip_hash_digest, campaign_snapshot_digest, staff_snapshot_digest, referer_digest, query_params_digest)
VALUES (sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_field_digest), sqlc.arg(source_id), sqlc.arg(link_source_id), sqlc.narg(radar_link_id), sqlc.narg(customer_id), sqlc.arg(code), sqlc.arg(raw_stage), sqlc.arg(source_channel), sqlc.arg(target_type_snapshot), sqlc.arg(source_channel_snapshot), sqlc.arg(error_code), sqlc.arg(created_at), sqlc.arg(open_id_digest), sqlc.arg(union_id_digest), sqlc.arg(external_user_id_digest), sqlc.arg(campaign_id_digest), sqlc.arg(staff_id_digest), sqlc.arg(user_agent_digest), sqlc.arg(ip_digest), sqlc.arg(person_id_digest), sqlc.arg(ip_hash_digest), sqlc.arg(campaign_snapshot_digest), sqlc.arg(staff_snapshot_digest), sqlc.arg(referer_digest), sqlc.arg(query_params_digest)) RETURNING *;

-- name: GetHistoricalRadarClick :one
SELECT * FROM radar_v1_click_history WHERE id=$1;

-- name: ListHistoricalRadarClick :many
SELECT * FROM radar_v1_click_history ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalRadarClick :one
SELECT count(*) FROM radar_v1_click_history;
