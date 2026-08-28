-- +goose Up
-- Source history only: no current click count, enrollment or execution.
CREATE TABLE radar_v1_click_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  link_source_id BIGINT NOT NULL,
  radar_link_id BIGINT REFERENCES radar_links(id),
  customer_id BIGINT REFERENCES customers(id),
  code TEXT NOT NULL,
  raw_stage TEXT NOT NULL,
  source_channel TEXT NOT NULL,
  target_type_snapshot TEXT NOT NULL,
  source_channel_snapshot TEXT NOT NULL,
  error_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  open_id_digest BYTEA NOT NULL CHECK (octet_length(open_id_digest) = 32),
  union_id_digest BYTEA NOT NULL CHECK (octet_length(union_id_digest) = 32),
  external_user_id_digest BYTEA NOT NULL CHECK (octet_length(external_user_id_digest) = 32),
  campaign_id_digest BYTEA NOT NULL CHECK (octet_length(campaign_id_digest) = 32),
  staff_id_digest BYTEA NOT NULL CHECK (octet_length(staff_id_digest) = 32),
  user_agent_digest BYTEA NOT NULL CHECK (octet_length(user_agent_digest) = 32),
  ip_digest BYTEA NOT NULL CHECK (octet_length(ip_digest) = 32),
  person_id_digest BYTEA NOT NULL CHECK (octet_length(person_id_digest) = 32),
  ip_hash_digest BYTEA NOT NULL CHECK (octet_length(ip_hash_digest) = 32),
  campaign_snapshot_digest BYTEA NOT NULL CHECK (octet_length(campaign_snapshot_digest) = 32),
  staff_snapshot_digest BYTEA NOT NULL CHECK (octet_length(staff_snapshot_digest) = 32),
  referer_digest BYTEA NOT NULL CHECK (octet_length(referer_digest) = 32),
  query_params_digest BYTEA NOT NULL CHECK (octet_length(query_params_digest) = 32)
);

CREATE TABLE automation_v1_marketing_config_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  automation_key TEXT NOT NULL,
  automation_name TEXT NOT NULL,
  target_event TEXT NOT NULL,
  channel_type TEXT NOT NULL,
  original_status TEXT NOT NULL,
  do_not_start_after_hour INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  config_payload_digest BYTEA NOT NULL CHECK (octet_length(config_payload_digest) = 32)
);

CREATE TABLE automation_v1_marketing_rule_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  config_id BIGINT NOT NULL REFERENCES automation_v1_marketing_config_history(id),
  config_source_id BIGINT NOT NULL,
  questionnaire_source_id BIGINT,
  question_source_id BIGINT,
  rule_code TEXT NOT NULL,
  rule_name TEXT NOT NULL,
  answer_match_type TEXT NOT NULL,
  score_delta INTEGER NOT NULL,
  segment_hint TEXT NOT NULL,
  stage_hint TEXT NOT NULL,
  original_active BOOLEAN NOT NULL,
  sort_order INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  answer_match_value_digest BYTEA NOT NULL CHECK (octet_length(answer_match_value_digest) = 32),
  rule_payload_digest BYTEA NOT NULL CHECK (octet_length(rule_payload_digest) = 32)
);

-- +goose Down
DROP TABLE automation_v1_marketing_rule_history;
DROP TABLE automation_v1_marketing_config_history;
DROP TABLE radar_v1_click_history;
