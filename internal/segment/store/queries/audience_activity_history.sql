-- name: CreateHistoricalAudienceActivityRun :one
INSERT INTO segment_v1_audience_activity_runs (
    source_key_digest,source_payload_digest,source_field_digest,source_id,package_history_id,version_history_id,
    run_type,original_status,refresh_started_at,refresh_finished_at,last_watermark_at,next_watermark_at,
    returned_count,entered_count,updated_count,exited_count,member_event_count,duration_ms,created_at,private_digest
) VALUES (
    sqlc.arg(source_key_digest),sqlc.arg(source_payload_digest),sqlc.arg(source_field_digest),sqlc.arg(source_id),sqlc.arg(package_history_id),sqlc.narg(version_history_id),
    sqlc.arg(run_type),sqlc.arg(original_status),sqlc.arg(refresh_started_at),sqlc.narg(refresh_finished_at),sqlc.narg(last_watermark_at),sqlc.narg(next_watermark_at),
    sqlc.arg(returned_count),sqlc.arg(entered_count),sqlc.arg(updated_count),sqlc.arg(exited_count),sqlc.arg(member_event_count),sqlc.arg(duration_ms),sqlc.arg(created_at),sqlc.arg(private_digest)
) RETURNING *;

-- name: GetHistoricalAudienceActivityRun :one
SELECT * FROM segment_v1_audience_activity_runs WHERE id = $1;

-- name: GetHistoricalAudienceActivityRunBySourceID :one
SELECT * FROM segment_v1_audience_activity_runs WHERE source_id = $1;

-- name: GetHistoricalAudienceActivityPackageBySourceID :one
SELECT id FROM segment_v1_audience_packages WHERE source_id = $1;

-- name: GetHistoricalAudienceActivityVersionBySourceID :one
SELECT id,package_history_id FROM segment_v1_audience_versions WHERE source_id = $1;

-- name: GetHistoricalAudienceActivityMemberBySourceID :one
SELECT id,package_history_id FROM segment_v1_audience_members WHERE source_id = $1;

-- name: CountHistoricalAudienceActivityRuns :one
SELECT count(*) FROM segment_v1_audience_activity_runs;

-- name: ListHistoricalAudienceActivityRuns :many
SELECT * FROM segment_v1_audience_activity_runs ORDER BY id LIMIT $1 OFFSET $2;

-- name: CreateHistoricalAudienceActivityMemberEvent :one
INSERT INTO segment_v1_audience_activity_member_events (
    source_key_digest,source_payload_digest,source_field_digest,source_id,package_history_id,run_history_id,member_history_id,
    event_type,identity_kind,occurred_at,created_at,private_digest
) VALUES (
    sqlc.arg(source_key_digest),sqlc.arg(source_payload_digest),sqlc.arg(source_field_digest),sqlc.arg(source_id),sqlc.arg(package_history_id),sqlc.narg(run_history_id),sqlc.narg(member_history_id),
    sqlc.arg(event_type),sqlc.arg(identity_kind),sqlc.arg(occurred_at),sqlc.arg(created_at),sqlc.arg(private_digest)
) RETURNING *;

-- name: GetHistoricalAudienceActivityMemberEvent :one
SELECT * FROM segment_v1_audience_activity_member_events WHERE id = $1;

-- name: CountHistoricalAudienceActivityMemberEvents :one
SELECT count(*) FROM segment_v1_audience_activity_member_events;

-- name: ListHistoricalAudienceActivityMemberEvents :many
SELECT * FROM segment_v1_audience_activity_member_events ORDER BY id LIMIT $1 OFFSET $2;
