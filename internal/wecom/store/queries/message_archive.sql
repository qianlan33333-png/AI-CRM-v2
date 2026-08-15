-- name: ReserveMessageArchiveSyncReceipt :one
INSERT INTO wecom_message_archive_sync_receipts (
  idempotency_scope, idempotency_key, request_digest
) VALUES (
  sqlc.arg(idempotency_scope)::text,
  sqlc.arg(idempotency_key)::text,
  sqlc.arg(request_digest)::bytea
)
ON CONFLICT (idempotency_scope, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING id, request_digest, state, accepted_event_id;

-- name: AcceptMessageArchiveSyncReceipt :one
UPDATE wecom_message_archive_sync_receipts
SET state = 'accepted',
    accepted_event_id = sqlc.arg(accepted_event_id)::bigint,
    accepted_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'reserved'
RETURNING id, request_digest, state, accepted_event_id;

-- name: MessageArchiveHealth :one
SELECT
  (SELECT count(*)::bigint FROM wecom_message_archive_records) AS record_count,
  (SELECT count(*)::bigint FROM wecom_message_archive_sync_receipts WHERE state = 'accepted') AS accepted_sync_count;

-- name: ListMessageArchiveLastAcceptedAt :many
SELECT accepted_at
FROM wecom_message_archive_sync_receipts
WHERE state = 'accepted'
ORDER BY accepted_at DESC
LIMIT 1;

-- name: ListMessageArchiveRecords :many
SELECT
  id, source_message_id, external_userid, chat_type, owner_userid, sender,
  receiver, chat_id, roomid, group_name, message_type, content_masked, sent_at
FROM wecom_message_archive_records
WHERE customer_id = sqlc.arg(customer_id)::bigint
  AND (sqlc.arg(chat_type)::text = '' OR chat_type = sqlc.arg(chat_type)::text)
  AND (
    sqlc.arg(keyword)::text = ''
    OR position(lower(sqlc.arg(keyword)::text) IN lower(content_masked)) > 0
  )
ORDER BY sent_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer
OFFSET sqlc.arg(row_offset)::integer;

-- name: CountMessageArchiveExternalRecords :one
SELECT count(*)::bigint
FROM wecom_message_archive_records
WHERE customer_id = sqlc.arg(customer_id)::bigint
  AND chat_type = sqlc.arg(chat_type)::text
  AND sent_at >= sqlc.arg(started_at)::timestamptz
  AND (
    chat_type <> 'private'
    OR sqlc.arg(with_userid)::text = ''
    OR owner_userid = sqlc.arg(with_userid)::text
    OR sender = sqlc.arg(with_userid)::text
    OR receiver = sqlc.arg(with_userid)::text
  );

-- name: ListMessageArchiveExternalRecords :many
SELECT
  id, source_message_id, external_userid, chat_type, owner_userid, sender,
  receiver, chat_id, roomid, group_name, message_type, content_masked, sent_at
FROM wecom_message_archive_records
WHERE customer_id = sqlc.arg(customer_id)::bigint
  AND chat_type = sqlc.arg(chat_type)::text
  AND sent_at >= sqlc.arg(started_at)::timestamptz
  AND (
    chat_type <> 'private'
    OR sqlc.arg(with_userid)::text = ''
    OR owner_userid = sqlc.arg(with_userid)::text
    OR sender = sqlc.arg(with_userid)::text
    OR receiver = sqlc.arg(with_userid)::text
  )
ORDER BY sent_at ASC, id ASC
LIMIT sqlc.arg(row_limit)::integer
OFFSET sqlc.arg(row_offset)::integer;
