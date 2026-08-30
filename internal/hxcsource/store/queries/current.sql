-- name: ReadHXCCurrentView :many
SELECT hxc_user_id, unionid, phone, subscription_tier, subscription_expires_at,
monthly_chat_quota, current_period_used, consultation_limit, consultation_used,
sessions_7d, sessions_30d, sessions_total, user_messages_7d, user_messages_30d,
user_messages_total, capability_usage, last_used_at, last_capability, business_stage,
main_line_type, user_segment, focus_topics, pain_tag, source_updated_at
FROM aicrm_hxc_user_current_v1
ORDER BY hxc_user_id
LIMIT 10001;
