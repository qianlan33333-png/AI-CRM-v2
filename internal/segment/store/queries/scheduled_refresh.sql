-- name: ListScheduledSegmentRefreshes :many
SELECT id, refresh_cron
FROM segments
WHERE refresh_mode = 'scheduled'
  AND refresh_cron IS NOT NULL
  AND lifecycle_status = 'active'
ORDER BY id;
