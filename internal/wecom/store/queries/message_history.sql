-- name: CreateHistoricalMessage :one
INSERT INTO wecom_v1_message_history (
  source_id, sequence, customer_id, chat_type, message_type, content_masked,
  original_send_time, send_time_basis, sent_at, created_at, source_payload_digest
) VALUES (
  sqlc.arg(source_id)::bigint, sqlc.narg(sequence)::bigint,
  sqlc.narg(customer_id)::bigint, sqlc.arg(chat_type)::text,
  sqlc.arg(message_type)::text, sqlc.narg(content_masked)::text,
  sqlc.arg(original_send_time)::text, sqlc.arg(send_time_basis)::text,
  sqlc.narg(sent_at)::timestamptz, sqlc.arg(created_at)::timestamptz,
  sqlc.arg(source_payload_digest)::bytea
)
RETURNING *;

-- name: GetHistoricalMessage :one
SELECT * FROM wecom_v1_message_history WHERE id = sqlc.arg(id)::bigint;

-- name: ListHistoricalMessages :many
SELECT * FROM wecom_v1_message_history
WHERE (sqlc.narg(customer_id)::bigint IS NULL OR customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.arg(chat_type)::text = '' OR chat_type = sqlc.arg(chat_type)::text)
ORDER BY id
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalMessages :one
SELECT count(*)::bigint FROM wecom_v1_message_history
WHERE (sqlc.narg(customer_id)::bigint IS NULL OR customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.arg(chat_type)::text = '' OR chat_type = sqlc.arg(chat_type)::text);
