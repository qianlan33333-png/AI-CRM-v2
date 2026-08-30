-- name: UpsertHXCCurrent :exec
INSERT INTO hxc_user_current (
  hxc_user_id, customer_id, match_state, subscription_tier, subscription_expires_at,
  monthly_chat_quota, current_period_used, consultation_limit, consultation_used,
  sessions_7d, sessions_30d, sessions_total, user_messages_7d, user_messages_30d,
  user_messages_total, capability_usage, last_used_at, last_capability, business_stage,
  main_line_type, user_segment, focus_topics, pain_tag, source_updated_at, synced_at
) VALUES (
  sqlc.arg(hxc_user_id), sqlc.narg(customer_id), sqlc.arg(match_state), sqlc.arg(subscription_tier), sqlc.narg(subscription_expires_at),
  sqlc.arg(monthly_chat_quota), sqlc.arg(current_period_used), sqlc.arg(consultation_limit), sqlc.arg(consultation_used),
  sqlc.arg(sessions_7d), sqlc.arg(sessions_30d), sqlc.arg(sessions_total), sqlc.arg(user_messages_7d), sqlc.arg(user_messages_30d),
  sqlc.arg(user_messages_total), sqlc.arg(capability_usage), sqlc.narg(last_used_at), sqlc.narg(last_capability), sqlc.narg(business_stage),
  sqlc.narg(main_line_type), sqlc.narg(user_segment), sqlc.arg(focus_topics), sqlc.narg(pain_tag), sqlc.arg(source_updated_at), sqlc.arg(synced_at)
)
ON CONFLICT (hxc_user_id) DO UPDATE SET
  customer_id=EXCLUDED.customer_id, match_state=EXCLUDED.match_state,
  subscription_tier=EXCLUDED.subscription_tier, subscription_expires_at=EXCLUDED.subscription_expires_at,
  monthly_chat_quota=EXCLUDED.monthly_chat_quota, current_period_used=EXCLUDED.current_period_used,
  consultation_limit=EXCLUDED.consultation_limit, consultation_used=EXCLUDED.consultation_used,
  sessions_7d=EXCLUDED.sessions_7d, sessions_30d=EXCLUDED.sessions_30d, sessions_total=EXCLUDED.sessions_total,
  user_messages_7d=EXCLUDED.user_messages_7d, user_messages_30d=EXCLUDED.user_messages_30d,
  user_messages_total=EXCLUDED.user_messages_total, capability_usage=EXCLUDED.capability_usage,
  last_used_at=EXCLUDED.last_used_at, last_capability=EXCLUDED.last_capability,
  business_stage=EXCLUDED.business_stage, main_line_type=EXCLUDED.main_line_type,
  user_segment=EXCLUDED.user_segment, focus_topics=EXCLUDED.focus_topics, pain_tag=EXCLUDED.pain_tag,
  source_updated_at=EXCLUDED.source_updated_at, synced_at=EXCLUDED.synced_at;

-- name: ClearHXCCurrentMatches :exec
UPDATE hxc_user_current SET customer_id = NULL, match_state = 'unmatched';

-- name: DeleteMissingHXCCurrent :exec
DELETE FROM hxc_user_current WHERE NOT (hxc_user_id = ANY(sqlc.arg(hxc_user_ids)::text[]));

-- name: InsertHXCCurrentSyncRun :exec
INSERT INTO hxc_current_sync_runs (
  status, source_count, matched_count, unmatched_count, conflict_count, error_code, created_at, expires_at
) VALUES (
  sqlc.arg(status), sqlc.arg(source_count), sqlc.arg(matched_count), sqlc.arg(unmatched_count), sqlc.arg(conflict_count), sqlc.narg(error_code), sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz + interval '15 days'
);

-- name: DeleteExpiredHXCCurrentSyncRuns :exec
DELETE FROM hxc_current_sync_runs WHERE expires_at < sqlc.arg(now);

-- name: GetLastSuccessfulHXCCurrentSync :one
SELECT created_at FROM hxc_current_sync_runs WHERE status='success' ORDER BY created_at DESC LIMIT 1;

-- name: GetHXCCurrentByCustomer :one
SELECT hxc_user_id, customer_id, match_state, subscription_tier, subscription_expires_at,
monthly_chat_quota, current_period_used, consultation_limit, consultation_used,
sessions_7d, sessions_30d, sessions_total, user_messages_7d, user_messages_30d,
user_messages_total, capability_usage, last_used_at, last_capability, business_stage,
main_line_type, user_segment, focus_topics, pain_tag, source_updated_at, synced_at
FROM hxc_user_current WHERE customer_id=sqlc.arg(customer_id) AND match_state='matched';
