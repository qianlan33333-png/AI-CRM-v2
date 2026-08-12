-- +goose Up
CREATE TABLE segments (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name           TEXT NOT NULL,
  definition     JSONB NOT NULL,
  refresh_mode   TEXT NOT NULL DEFAULT 'manual',
  refresh_cron   TEXT,
  member_count   BIGINT NOT NULL DEFAULT 0,
  refreshed_at   TIMESTAMPTZ,
  refresh_status TEXT NOT NULL DEFAULT 'idle',
  created_by     BIGINT REFERENCES admin_users(id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT segments_name_not_blank CHECK (btrim(name) <> '' AND char_length(name) <= 200),
  CONSTRAINT segments_definition_object CHECK (jsonb_typeof(definition) = 'object'),
  CONSTRAINT segments_refresh_mode CHECK (refresh_mode IN ('manual', 'scheduled')),
  CONSTRAINT segments_refresh_cron CHECK (
    (refresh_mode = 'manual' AND refresh_cron IS NULL) OR
    (refresh_mode = 'scheduled' AND refresh_cron IS NOT NULL AND btrim(refresh_cron) <> '')
  ),
  CONSTRAINT segments_member_count_nonnegative CHECK (member_count >= 0),
  CONSTRAINT segments_refresh_status CHECK (refresh_status IN ('idle', 'running', 'failed'))
);

CREATE INDEX idx_segments_refresh_due
  ON segments (refresh_mode, refresh_status, refreshed_at, id)
  WHERE refresh_mode = 'scheduled';

CREATE TABLE segment_members (
  segment_id  BIGINT NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
  customer_id BIGINT NOT NULL REFERENCES customers(id),
  computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (segment_id, customer_id)
);

CREATE INDEX idx_segment_members_customer ON segment_members (customer_id, segment_id);

-- +goose Down
DROP TABLE segment_members;
DROP TABLE segments;
