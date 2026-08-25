-- name: InsertWeComContactInbox :one
INSERT INTO wecom_contact_inbox (
  source, source_key, corp_id, event_type, external_userid, raw_payload,
  payload_digest, occurred_at, state, processed_at,
  external_contact_change_type, external_contact_wecom_userid, external_contact_state,
  external_contact_welcome_present, external_contact_welcome_digest,
  external_contact_source_digest, external_contact_fail_reason_digest
) VALUES (
  sqlc.arg(source)::text, sqlc.arg(source_key)::text, sqlc.arg(corp_id)::text,
  sqlc.arg(event_type)::text, sqlc.arg(external_userid)::text, sqlc.arg(raw_payload)::bytea,
  sqlc.arg(payload_digest)::text, sqlc.arg(occurred_at)::timestamptz, sqlc.arg(state)::text,
  CASE WHEN sqlc.arg(state)::text = 'skipped' THEN now() ELSE NULL END,
  sqlc.arg(external_contact_change_type)::text, sqlc.arg(external_contact_wecom_userid)::text,
  sqlc.arg(external_contact_state)::text, sqlc.arg(external_contact_welcome_present)::boolean,
  sqlc.arg(external_contact_welcome_digest)::text, sqlc.arg(external_contact_source_digest)::text,
  sqlc.arg(external_contact_fail_reason_digest)::text
)
ON CONFLICT (source, source_key) DO NOTHING
RETURNING id, state, river_job_id;

-- name: GetWeComContactInboxByKey :one
SELECT id, state, river_job_id
FROM wecom_contact_inbox
WHERE source = sqlc.arg(source)::text AND source_key = sqlc.arg(source_key)::text;

-- name: MarkWeComContactInboxQueued :one
UPDATE wecom_contact_inbox
SET river_job_id = sqlc.arg(river_job_id)::bigint, updated_at = now()
WHERE id = sqlc.arg(id)::bigint AND river_job_id IS NULL
RETURNING id, river_job_id;

-- name: ClaimWeComContactInbox :one
UPDATE wecom_contact_inbox
SET state = 'processing',
    attempt_count = attempt_count + 1,
    lease_fence = lease_fence + 1,
    lease_owner = sqlc.arg(lease_owner)::text,
    lease_expires_at = sqlc.arg(lease_expires_at)::timestamptz,
    updated_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND (
    state IN ('pending', 'failed')
    OR (state = 'processing' AND lease_expires_at < now() AND lease_owner = sqlc.arg(lease_owner)::text)
  )
RETURNING id, source, source_key, corp_id, event_type, external_userid, raw_payload,
  occurred_at, state, attempt_count, lease_fence, lease_owner, river_job_id,
  external_contact_change_type, external_contact_wecom_userid, external_contact_state,
  external_contact_welcome_present, external_contact_welcome_digest,
  external_contact_source_digest, external_contact_fail_reason_digest;

-- name: CompleteWeComContactInbox :one
UPDATE wecom_contact_inbox
SET state = sqlc.arg(state)::text,
    lease_owner = '', lease_expires_at = NULL,
    last_error = '', processed_at = now(), updated_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'processing'
  AND lease_fence = sqlc.arg(lease_fence)::bigint
RETURNING id, state;

-- name: FailWeComContactInbox :one
UPDATE wecom_contact_inbox
SET state = 'failed',
    lease_owner = '', lease_expires_at = NULL,
    last_error = sqlc.arg(last_error)::text, updated_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'processing'
  AND lease_fence = sqlc.arg(lease_fence)::bigint
RETURNING id, state;

-- name: CountWeComContactInbox :one
SELECT count(*)::bigint
FROM wecom_contact_inbox
WHERE (sqlc.arg(state)::text = '' OR state = sqlc.arg(state)::text)
  AND (sqlc.arg(source)::text = '' OR source = sqlc.arg(source)::text);
