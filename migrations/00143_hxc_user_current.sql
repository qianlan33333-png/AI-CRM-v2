-- +goose Up
CREATE TABLE hxc_user_current (
  hxc_user_id TEXT PRIMARY KEY CHECK (btrim(hxc_user_id) = hxc_user_id AND hxc_user_id <> ''),
  customer_id BIGINT REFERENCES customers(id),
  match_state TEXT NOT NULL CHECK (match_state IN ('matched', 'unmatched', 'conflict')),
  subscription_tier TEXT NOT NULL CHECK (btrim(subscription_tier) = subscription_tier AND subscription_tier <> ''),
  subscription_expires_at TIMESTAMPTZ,
  monthly_chat_quota INTEGER NOT NULL CHECK (monthly_chat_quota >= 0),
  current_period_used INTEGER NOT NULL CHECK (current_period_used >= 0),
  consultation_limit INTEGER NOT NULL CHECK (consultation_limit >= 0),
  consultation_used INTEGER NOT NULL CHECK (consultation_used >= 0),
  sessions_7d BIGINT NOT NULL CHECK (sessions_7d >= 0),
  sessions_30d BIGINT NOT NULL CHECK (sessions_30d >= sessions_7d),
  sessions_total BIGINT NOT NULL CHECK (sessions_total >= sessions_30d),
  user_messages_7d BIGINT NOT NULL CHECK (user_messages_7d >= 0),
  user_messages_30d BIGINT NOT NULL CHECK (user_messages_30d >= user_messages_7d),
  user_messages_total BIGINT NOT NULL CHECK (user_messages_total >= user_messages_30d),
  capability_usage JSONB NOT NULL CHECK (jsonb_typeof(capability_usage) = 'object'),
  last_used_at TIMESTAMPTZ,
  last_capability TEXT,
  business_stage TEXT,
  main_line_type TEXT,
  user_segment TEXT,
  focus_topics JSONB NOT NULL CHECK (jsonb_typeof(focus_topics) = 'array'),
  pain_tag TEXT,
  source_updated_at TIMESTAMPTZ NOT NULL,
  synced_at TIMESTAMPTZ NOT NULL,
  CHECK ((match_state = 'matched') = (customer_id IS NOT NULL))
);

CREATE UNIQUE INDEX hxc_user_current_customer
  ON hxc_user_current(customer_id)
  WHERE customer_id IS NOT NULL;

CREATE TABLE hxc_current_sync_runs (
  id BIGSERIAL PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('success', 'failed')),
  source_count INTEGER NOT NULL CHECK (source_count >= 0),
  matched_count INTEGER NOT NULL CHECK (matched_count >= 0),
  unmatched_count INTEGER NOT NULL CHECK (unmatched_count >= 0),
  conflict_count INTEGER NOT NULL CHECK (conflict_count >= 0),
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  CHECK (expires_at = created_at + interval '15 days')
);

CREATE INDEX hxc_current_sync_runs_expires_at ON hxc_current_sync_runs(expires_at);

-- +goose Down
DROP TABLE hxc_current_sync_runs;
DROP TABLE hxc_user_current;
