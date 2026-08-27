-- Import is bound to a non-executing historical channel. It never writes the
-- current customer attribution, staff directory or Provider asset bindings.
-- name: CreateHistoricalChannelContact :one
INSERT INTO channel_historical_contacts (
  channel_id, source_contact_id, customer_id, owner_reference,
  first_entered_at, last_entered_at, enter_count, created_at, updated_at
)
SELECT c.id, sqlc.arg(source_contact_id)::bigint, sqlc.narg(customer_id)::bigint,
       sqlc.arg(owner_reference)::text, sqlc.arg(first_entered_at)::timestamptz,
       sqlc.arg(last_entered_at)::timestamptz, sqlc.arg(enter_count)::integer,
       sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
FROM channels c JOIN channel_acquisition_legacy_archives a ON a.channel_id=c.id
WHERE c.id=sqlc.arg(channel_id)::bigint AND c.status='inactive' AND a.status='legacy_unverified'
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetHistoricalChannelContact :one
SELECT * FROM channel_historical_contacts WHERE id=sqlc.arg(id)::bigint FOR UPDATE;

-- name: ListHistoricalChannelContacts :many
SELECT * FROM channel_historical_contacts WHERE channel_id=sqlc.arg(channel_id)::bigint
ORDER BY id LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: CountHistoricalChannelContacts :one
SELECT count(*) FROM channel_historical_contacts WHERE channel_id=sqlc.arg(channel_id)::bigint;

-- V1 assignee timestamps have no timezone. Timestamp (not timestamptz) keeps
-- those civil values without silently assigning a timezone during migration.
-- name: CreateHistoricalChannelAssignee :one
INSERT INTO channel_historical_assignees (
  channel_id, source_assignee_id, staff_reference, display_name_snapshot,
  priority, ratio_percent, max_scans_24h, status, source_created_at, source_updated_at
)
SELECT c.id, sqlc.arg(source_assignee_id)::bigint, sqlc.arg(staff_reference)::text,
       sqlc.arg(display_name_snapshot)::text, sqlc.arg(priority)::integer,
       sqlc.narg(ratio_percent)::integer, sqlc.narg(max_scans_24h)::integer,
       sqlc.arg(status)::text, sqlc.arg(source_created_at)::timestamp, sqlc.arg(source_updated_at)::timestamp
FROM channels c JOIN channel_acquisition_legacy_archives a ON a.channel_id=c.id
WHERE c.id=sqlc.arg(channel_id)::bigint AND c.status='inactive' AND a.status='legacy_unverified'
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetHistoricalChannelAssignee :one
SELECT * FROM channel_historical_assignees WHERE id=sqlc.arg(id)::bigint FOR UPDATE;

-- name: ListHistoricalChannelAssignees :many
SELECT * FROM channel_historical_assignees WHERE channel_id=sqlc.arg(channel_id)::bigint
ORDER BY priority,id LIMIT 201;
