-- +goose Up
CREATE TABLE IF NOT EXISTS stats_daily (
  stat_date  DATE NOT NULL,
  metric_key TEXT NOT NULL,
  dims       JSONB NOT NULL DEFAULT '{}'::jsonb,
  value      NUMERIC NOT NULL,
  PRIMARY KEY (stat_date, metric_key, dims),
  CONSTRAINT stats_daily_metric_key CHECK (
    btrim(metric_key) = metric_key AND char_length(metric_key) BETWEEN 1 AND 200
  ),
  CONSTRAINT stats_daily_dims CHECK (jsonb_typeof(dims) = 'object'),
  CONSTRAINT stats_daily_value CHECK (value >= 0)
);

CREATE INDEX IF NOT EXISTS stats_daily_metric_date_idx
  ON stats_daily (metric_key, stat_date DESC)
  INCLUDE (dims, value);

CREATE TABLE IF NOT EXISTS stats_event_receipts (
  event_id    BIGINT NOT NULL,
  consumer    TEXT NOT NULL,
  stat_date   DATE NOT NULL,
  metric_key  TEXT NOT NULL,
  dims        JSONB NOT NULL,
  value_delta BIGINT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (event_id, consumer),
  CONSTRAINT stats_event_receipts_consumer CHECK (consumer = 'stats.tag-applied.v1'),
  CONSTRAINT stats_event_receipts_event CHECK (event_id > 0),
  CONSTRAINT stats_event_receipts_metric_key CHECK (metric_key = 'customer.tag_applied'),
  CONSTRAINT stats_event_receipts_dims CHECK (jsonb_typeof(dims) = 'object'),
  CONSTRAINT stats_event_receipts_delta CHECK (value_delta = 1)
);

-- +goose Down
-- L01 receipts and projections are durable business observations. Application
-- rollback preserves them so a later forward replay cannot double-count events.
SELECT 1;
