-- name: SelectSegmentUniverse :many
SELECT id FROM customers ORDER BY id;

-- name: SelectSegmentStageEqual :many
SELECT id FROM customers WHERE stage_id = sqlc.arg(stage_id)::bigint;

-- name: SelectSegmentStageAny :many
SELECT id FROM customers WHERE stage_id = ANY(sqlc.arg(stage_ids)::bigint[]);

-- name: SelectSegmentOwnerEqual :many
SELECT id FROM customers WHERE owner_staff_id = sqlc.arg(owner_staff_id)::bigint;

-- name: SelectSegmentOwnerAny :many
SELECT id FROM customers WHERE owner_staff_id = ANY(sqlc.arg(owner_staff_ids)::bigint[]);

-- name: SelectSegmentChannelEqual :many
SELECT id FROM customers WHERE channel_id = sqlc.arg(channel_id)::bigint;

-- name: SelectSegmentChannelAny :many
SELECT id FROM customers WHERE channel_id = ANY(sqlc.arg(channel_ids)::bigint[]);

-- name: SelectSegmentTagAny :many
SELECT DISTINCT customer_id
FROM customer_tags
WHERE tag_id = ANY(sqlc.arg(tag_ids)::bigint[]);

-- name: SelectSegmentAddedBefore :many
SELECT id FROM customers WHERE added_at < sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentAddedAfter :many
SELECT id FROM customers WHERE added_at > sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentLastInteractBefore :many
SELECT id FROM customers WHERE last_interact_at < sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentLastInteractAfter :many
SELECT id FROM customers WHERE last_interact_at > sqlc.arg(instant)::timestamptz;

-- name: SelectSegmentDeletedEqual :many
SELECT id FROM customers WHERE is_deleted = sqlc.arg(is_deleted)::boolean;

-- name: SelectSegmentHXCSubscriptionTierEqual :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND subscription_tier = sqlc.arg(value)::text ORDER BY customer_id;

-- name: SelectSegmentHXCSubscriptionActiveEqual :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND COALESCE(subscription_expires_at > CURRENT_TIMESTAMP, false) = sqlc.arg(value)::boolean ORDER BY customer_id;

-- name: SelectSegmentHXCDaysRemainingGTE :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND subscription_expires_at IS NOT NULL AND GREATEST(FLOOR(EXTRACT(EPOCH FROM (subscription_expires_at - CURRENT_TIMESTAMP)) / 86400), 0)::bigint >= sqlc.arg(value)::bigint ORDER BY customer_id;

-- name: SelectSegmentHXCDaysRemainingLTE :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND subscription_expires_at IS NOT NULL AND GREATEST(FLOOR(EXTRACT(EPOCH FROM (subscription_expires_at - CURRENT_TIMESTAMP)) / 86400), 0)::bigint <= sqlc.arg(value)::bigint ORDER BY customer_id;

-- name: SelectSegmentHXCUserMessages7DGTE :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND user_messages_7d >= sqlc.arg(value)::bigint ORDER BY customer_id;

-- name: SelectSegmentHXCUserMessages7DLTE :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND user_messages_7d <= sqlc.arg(value)::bigint ORDER BY customer_id;

-- name: SelectSegmentHXCUserMessages30DGTE :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND user_messages_30d >= sqlc.arg(value)::bigint ORDER BY customer_id;

-- name: SelectSegmentHXCUserMessages30DLTE :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND user_messages_30d <= sqlc.arg(value)::bigint ORDER BY customer_id;

-- name: SelectSegmentHXCLastCapabilityEqual :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND last_capability = sqlc.arg(value)::text ORDER BY customer_id;

-- name: SelectSegmentHXCBusinessStageEqual :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND business_stage = sqlc.arg(value)::text ORDER BY customer_id;

-- name: SelectSegmentHXCMainLineTypeEqual :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND main_line_type = sqlc.arg(value)::text ORDER BY customer_id;

-- name: SelectSegmentHXCUserSegmentEqual :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND user_segment = sqlc.arg(value)::text ORDER BY customer_id;

-- name: SelectSegmentHXCFocusTopicAny :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND focus_topics ?| sqlc.arg(values)::text[] ORDER BY customer_id;

-- name: SelectSegmentHXCPainTagEqual :many
SELECT COALESCE(customer_id, 0)::bigint FROM hxc_user_current WHERE match_state = 'matched' AND pain_tag = sqlc.arg(value)::text ORDER BY customer_id;

-- name: SelectLegacyAudiencePackageSnapshot :many
SELECT DISTINCT COALESCE(member.customer_id, 0)::bigint AS customer_id
FROM segment_v1_audience_members AS member
JOIN segment_v1_audience_packages AS package ON package.id = member.package_history_id
JOIN customers AS customer ON customer.id = member.customer_id
WHERE package.source_id = sqlc.arg(source_id)::bigint
  AND member.customer_id IS NOT NULL
ORDER BY customer_id;
