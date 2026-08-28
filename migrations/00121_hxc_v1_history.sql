-- +goose Up
-- Immutable V1 observations only: no current HXC, sender, entitlement or job.
-- DATE columns preserve source calendars; all source IDs keep signed values.
CREATE TABLE hxc_v1_dashboard_refresh_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  started_at TIMESTAMPTZ NOT NULL,
  finished_at TIMESTAMPTZ,
  status TEXT NOT NULL,
  row_count BIGINT NOT NULL,
  member_hit BIGINT NOT NULL,
  user_hit BIGINT NOT NULL,
  only_member BIGINT NOT NULL,
  trigger_source TEXT NOT NULL
);

CREATE TABLE hxc_v1_dashboard_observations (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  customer_id BIGINT CHECK (customer_id > 0),
  observation TEXT NOT NULL CHECK (observation = 'observed_snapshot'),
  observed_at TIMESTAMPTZ NOT NULL,
  in_lead_pool BOOLEAN NOT NULL,
  in_people BOOLEAN NOT NULL,
  in_questionnaire BOOLEAN NOT NULL,
  class_term_no BIGINT,
  class_term_label TEXT NOT NULL,
  crm_hxc_state TEXT NOT NULL,
  crm_created_at DATE,
  last_questionnaire_at DATE,
  hxc_member_hit BOOLEAN NOT NULL,
  hxc_user_hit BOOLEAN NOT NULL,
  funnel_state TEXT NOT NULL,
  hxc_member_status TEXT NOT NULL,
  hxc_registered_at TIMESTAMPTZ,
  hxc_last_login_at TIMESTAMPTZ,
  membership_type TEXT NOT NULL,
  membership_status TEXT NOT NULL,
  membership_end_at TIMESTAMPTZ,
  membership_days_left BIGINT,
  consultation_used BIGINT,
  consultation_limit BIGINT,
  conversation_chat BIGINT NOT NULL,
  conversation_consult BIGINT NOT NULL,
  conversation_lesson BIGINT NOT NULL,
  messages_user BIGINT NOT NULL,
  messages_ai BIGINT NOT NULL,
  consult_completed BIGINT NOT NULL,
  last_message_at TIMESTAMPTZ,
  subscription_tier TEXT NOT NULL,
  subscription_expires TIMESTAMPTZ,
  subscription_quota BIGINT,
  subscription_used BIGINT,
  subscription_period_start DATE
);

CREATE TABLE hxc_v1_activation_observations (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_table TEXT NOT NULL CHECK (source_table IN ('public/user_ops_activation_status_source', 'public/user_ops_huangxiaocan_activation_source')),
  original_state TEXT NOT NULL,
  is_active BOOLEAN NOT NULL,
  legacy_import_batch_ref TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE hxc_v1_experience_lead_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  original_type TEXT NOT NULL,
  is_active BOOLEAN NOT NULL,
  legacy_import_batch_ref TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE hxc_v1_import_batch_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  import_type TEXT NOT NULL,
  total_rows BIGINT NOT NULL,
  success_rows BIGINT NOT NULL,
  failed_rows BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX hxc_v1_dashboard_observations_customer_idx ON hxc_v1_dashboard_observations (customer_id, id);
CREATE INDEX hxc_v1_activation_observations_source_idx ON hxc_v1_activation_observations (source_table, id);

-- +goose Down
DROP TABLE hxc_v1_import_batch_history;
DROP TABLE hxc_v1_experience_lead_history;
DROP TABLE hxc_v1_activation_observations;
DROP TABLE hxc_v1_dashboard_observations;
DROP TABLE hxc_v1_dashboard_refresh_history;
