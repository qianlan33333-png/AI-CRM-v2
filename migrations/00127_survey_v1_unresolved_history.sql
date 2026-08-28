-- +goose Up
-- Preserve unresolved source answer snapshots without creating current definitions or effects.
CREATE TABLE survey_v1_unresolved_submissions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest)=32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest)=32),
  source_id BIGINT NOT NULL,
  questionnaire_source_id BIGINT NOT NULL,
  questionnaire_id BIGINT REFERENCES questionnaires(id),
  customer_id BIGINT REFERENCES customers(id),
  matched_by TEXT NOT NULL,
  source_channel TEXT NOT NULL,
  total_score DOUBLE PRECISION NOT NULL,
  final_tags JSONB NOT NULL,
  submitted_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  union_id_digest BYTEA NOT NULL CHECK (octet_length(union_id_digest)=32),
  follow_user_user_id_digest BYTEA NOT NULL CHECK (octet_length(follow_user_user_id_digest)=32),
  campaign_id_digest BYTEA NOT NULL CHECK (octet_length(campaign_id_digest)=32),
  staff_id_digest BYTEA NOT NULL CHECK (octet_length(staff_id_digest)=32),
  redirect_url_digest BYTEA NOT NULL CHECK (octet_length(redirect_url_digest)=32),
  assessment_result_digest BYTEA NOT NULL CHECK (octet_length(assessment_result_digest)=32)
);

CREATE TABLE survey_v1_unresolved_answers (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest)=32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest)=32),
  source_id BIGINT NOT NULL,
  submission_id BIGINT NOT NULL REFERENCES survey_v1_unresolved_submissions(id),
  submission_source_id BIGINT NOT NULL,
  question_source_id BIGINT NOT NULL,
  question_type TEXT NOT NULL,
  question_title_snapshot TEXT NOT NULL,
  selected_option_ids JSONB NOT NULL,
  selected_option_texts JSONB NOT NULL,
  selected_option_scores JSONB NOT NULL,
  selected_option_tags JSONB NOT NULL,
  text_value TEXT NOT NULL,
  score_contribution DOUBLE PRECISION NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX survey_v1_unresolved_submissions_questionnaire_idx ON survey_v1_unresolved_submissions (questionnaire_id, id);
CREATE INDEX survey_v1_unresolved_answers_submission_idx ON survey_v1_unresolved_answers (submission_id, id);

-- +goose Down
DROP TABLE survey_v1_unresolved_answers;
DROP TABLE survey_v1_unresolved_submissions;
