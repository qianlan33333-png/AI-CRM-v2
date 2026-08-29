-- name: CreateHistoricalCustomerTimelineEvent :one
INSERT INTO contact_v1_customer_timeline_history (
  source_key_digest, source_payload_digest, source_field_digest, source_id,
  event_id, event_type, event_time, title, summary, source_table, source_value,
  metadata_json, created_at, unionid, customer_id
) VALUES (
  sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_payload_digest)::bytea,
  sqlc.arg(source_field_digest)::bytea, sqlc.arg(source_id)::bigint,
  sqlc.arg(event_id)::text, sqlc.arg(event_type)::text,
  sqlc.arg(event_time)::timestamptz, sqlc.arg(title)::text,
  sqlc.arg(summary)::text, sqlc.arg(source_table)::text,
  sqlc.arg(source_value)::text, sqlc.arg(metadata_json)::text,
  sqlc.arg(created_at)::timestamptz, sqlc.arg(unionid)::text,
  sqlc.narg(customer_id)::bigint
)
RETURNING *;

-- name: GetHistoricalCustomerTimelineEvent :one
SELECT * FROM contact_v1_customer_timeline_history WHERE id = sqlc.arg(id)::bigint;

-- name: CountHistoricalCustomerTimelineEvents :one
SELECT count(*)::bigint FROM contact_v1_customer_timeline_history;

-- name: ListHistoricalCustomerTimelineEvents :many
SELECT id, source_id, event_id, event_type, event_time, source_table,
       source_value, created_at, customer_id
FROM contact_v1_customer_timeline_history
ORDER BY id
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;
