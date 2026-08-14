-- name: ReserveStatsEventReceipt :one
INSERT INTO stats_event_receipts (
  event_id,
  consumer,
  stat_date,
  metric_key,
  dims,
  value_delta
) VALUES (
  sqlc.arg(event_id),
  sqlc.arg(consumer),
  sqlc.arg(stat_date),
  sqlc.arg(metric_key),
  sqlc.arg(dims)::jsonb,
  sqlc.arg(value_delta)
)
ON CONFLICT (event_id, consumer) DO NOTHING
RETURNING event_id;

-- name: GetMatchingStatsEventReceipt :one
SELECT event_id
FROM stats_event_receipts
WHERE event_id = sqlc.arg(event_id)
  AND consumer = sqlc.arg(consumer)
  AND stat_date = sqlc.arg(stat_date)
  AND metric_key = sqlc.arg(metric_key)
  AND dims = sqlc.arg(dims)::jsonb
  AND value_delta = sqlc.arg(value_delta);

-- name: IncrementStatsDaily :exec
INSERT INTO stats_daily (stat_date, metric_key, dims, value)
VALUES (
  sqlc.arg(stat_date),
  sqlc.arg(metric_key),
  sqlc.arg(dims)::jsonb,
  sqlc.arg(value_delta)::bigint
)
ON CONFLICT (stat_date, metric_key, dims) DO UPDATE
SET value = stats_daily.value + EXCLUDED.value;

-- name: GetStatsDaily :one
SELECT stat_date, metric_key, dims, value
FROM stats_daily
WHERE stat_date = sqlc.arg(stat_date)
  AND metric_key = sqlc.arg(metric_key)
  AND dims = sqlc.arg(dims)::jsonb;
