-- +goose Up
-- V1 profile definitions and signup rules are history only, never active rules.
CREATE TABLE segment_v1_profile_templates (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL UNIQUE,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  template_code TEXT NOT NULL,
  template_name TEXT NOT NULL,
  questionnaire_source_id BIGINT,
  segmentation_question_source_id BIGINT,
  program_source_id BIGINT,
  description TEXT NOT NULL,
  original_enabled BOOLEAN NOT NULL,
  version BIGINT NOT NULL,
  created_by_digest BYTEA NOT NULL CHECK (octet_length(created_by_digest)=32),
  updated_by_digest BYTEA NOT NULL CHECK (octet_length(updated_by_digest)=32),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE segment_v1_profile_categories (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL UNIQUE,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  template_source_id BIGINT NOT NULL,
  template_history_id BIGINT NOT NULL REFERENCES segment_v1_profile_templates(id),
  category_key TEXT NOT NULL,
  category_name TEXT NOT NULL,
  description TEXT NOT NULL,
  sort_order BIGINT NOT NULL,
  original_enabled BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (id,template_history_id)
);
CREATE INDEX segment_v1_profile_categories_parent_idx ON segment_v1_profile_categories(template_history_id,id);
CREATE TABLE segment_v1_profile_option_mappings (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL UNIQUE,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  template_source_id BIGINT NOT NULL,
  category_source_id BIGINT NOT NULL,
  template_history_id BIGINT NOT NULL REFERENCES segment_v1_profile_templates(id),
  category_history_id BIGINT NOT NULL,
  question_source_id BIGINT NOT NULL,
  option_source_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  FOREIGN KEY (category_history_id,template_history_id) REFERENCES segment_v1_profile_categories(id,template_history_id)
);
CREATE INDEX segment_v1_profile_option_mappings_parent_idx ON segment_v1_profile_option_mappings(template_history_id,id);
CREATE TABLE contact_v1_signup_tag_rules (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  tag_source_id TEXT NOT NULL,
  tag_name TEXT NOT NULL,
  signup_status TEXT NOT NULL,
  original_active BOOLEAN NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE contact_v1_signup_tag_rules;
DROP TABLE segment_v1_profile_option_mappings;
DROP TABLE segment_v1_profile_categories;
DROP TABLE segment_v1_profile_templates;
