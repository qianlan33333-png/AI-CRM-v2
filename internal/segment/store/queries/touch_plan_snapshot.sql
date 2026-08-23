-- name: LockSegmentTouchPlanSnapshot :one
SELECT id, member_count, refreshed_at, refresh_status, lifecycle_status
FROM public.segments
WHERE id = sqlc.arg(segment_id)::bigint
FOR SHARE;

-- name: LockAudiencePackageTouchPlanSnapshot :one
SELECT metadata.segment_id AS package_id,
       metadata.version AS package_version,
       metadata.lifecycle AS package_lifecycle,
       segment.member_count,
       segment.refreshed_at,
       segment.refresh_status,
       segment.lifecycle_status
FROM public.ai_audience_package_metadata AS metadata
JOIN public.segments AS segment ON segment.id = metadata.segment_id
WHERE metadata.segment_id = sqlc.arg(package_id)::bigint
FOR SHARE OF metadata, segment;

-- name: ListTouchPlanSnapshotMembers :many
SELECT customer_id
FROM public.segment_members
WHERE segment_id = sqlc.arg(segment_id)::bigint
ORDER BY customer_id ASC;
