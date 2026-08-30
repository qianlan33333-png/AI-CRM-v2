-- name: GetMessageArchiveSyncState :one
SELECT last_seq, last_success_at, updated_at
FROM public.wecom_message_archive_sync_state
WHERE singleton = TRUE;

-- name: StartMessageArchiveSyncRun :one
INSERT INTO public.wecom_message_archive_sync_runs (cursor_from, cursor_to)
VALUES (sqlc.arg(cursor_from)::bigint, sqlc.arg(cursor_from)::bigint)
RETURNING id;

-- name: UpsertMessageArchiveRecord :one
INSERT INTO public.wecom_message_archive_records (
  source_message_id, customer_id, external_userid, chat_type, owner_userid,
  sender, receiver, chat_id, roomid, group_name, message_type,
  content_masked, sent_at, provider_seq, identity_state, source_payload_digest
) VALUES (
  sqlc.arg(source_message_id)::text, sqlc.narg(customer_id)::bigint,
  sqlc.arg(external_userid)::text, sqlc.arg(chat_type)::text,
  sqlc.arg(owner_userid)::text, sqlc.arg(sender)::text,
  sqlc.arg(receiver)::text, sqlc.arg(chat_id)::text,
  sqlc.arg(roomid)::text, sqlc.arg(group_name)::text,
  sqlc.arg(message_type)::text, sqlc.arg(content_masked)::text,
  sqlc.arg(sent_at)::timestamptz, sqlc.arg(provider_seq)::bigint,
  sqlc.arg(identity_state)::text, sqlc.arg(source_payload_digest)::bytea
)
ON CONFLICT (source_message_id) DO UPDATE SET
  customer_id = CASE
    WHEN wecom_message_archive_records.customer_id IS NULL THEN EXCLUDED.customer_id
    ELSE wecom_message_archive_records.customer_id
  END,
  identity_state = CASE
    WHEN wecom_message_archive_records.customer_id IS NULL AND EXCLUDED.customer_id IS NOT NULL THEN 'resolved'
    ELSE wecom_message_archive_records.identity_state
  END
RETURNING id, (xmax = 0) AS inserted;

-- name: AdvanceMessageArchiveSyncState :execrows
UPDATE public.wecom_message_archive_sync_state
SET last_seq = GREATEST(last_seq, sqlc.arg(last_seq)::bigint),
    last_success_at = sqlc.arg(completed_at)::timestamptz,
    updated_at = sqlc.arg(completed_at)::timestamptz
WHERE singleton = TRUE;

-- name: FinishMessageArchiveSyncRun :execrows
UPDATE public.wecom_message_archive_sync_runs
SET state = sqlc.arg(state)::text,
    cursor_to = sqlc.arg(cursor_to)::bigint,
    fetched_count = sqlc.arg(fetched_count)::bigint,
    accepted_count = sqlc.arg(accepted_count)::bigint,
    inserted_count = sqlc.arg(inserted_count)::bigint,
    unresolved_count = sqlc.arg(unresolved_count)::bigint,
    failure_code = sqlc.arg(failure_code)::text,
    finished_at = sqlc.arg(finished_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'running';

-- name: ResolveMessageArchiveRecords :execrows
UPDATE public.wecom_message_archive_records AS archive
SET customer_id = identity.customer_id,
    identity_state = 'resolved'
FROM public.identities AS identity
JOIN public.customers AS customer ON customer.id = identity.customer_id AND customer.is_deleted = FALSE
WHERE archive.identity_state = 'unresolved'
  AND archive.external_userid = identity.normalized_value
  AND identity.kind = 'wecom_external_userid'
  AND identity.scope = sqlc.arg(scope)::text
  AND identity.assurance = 'verified'
  AND identity.customer_id IS NOT NULL;
