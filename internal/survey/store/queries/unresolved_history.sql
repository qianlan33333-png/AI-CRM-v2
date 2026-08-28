-- name: CreateHistoricalUnresolvedSurveySubmission :one
INSERT INTO survey_v1_unresolved_submissions (source_key_digest, source_payload_digest, source_field_digest, source_id, questionnaire_source_id, questionnaire_id, customer_id, matched_by, source_channel, total_score, final_tags, submitted_at, created_at, union_id_digest, follow_user_user_id_digest, campaign_id_digest, staff_id_digest, redirect_url_digest, assessment_result_digest)
VALUES (sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_field_digest), sqlc.arg(source_id), sqlc.arg(questionnaire_source_id), sqlc.narg(questionnaire_id), sqlc.narg(customer_id), sqlc.arg(matched_by), sqlc.arg(source_channel), sqlc.arg(total_score), sqlc.arg(final_tags), sqlc.arg(submitted_at), sqlc.arg(created_at), sqlc.arg(union_id_digest), sqlc.arg(follow_user_user_id_digest), sqlc.arg(campaign_id_digest), sqlc.arg(staff_id_digest), sqlc.arg(redirect_url_digest), sqlc.arg(assessment_result_digest)) RETURNING *;

-- name: GetHistoricalUnresolvedSurveySubmission :one
SELECT * FROM survey_v1_unresolved_submissions WHERE id=$1;

-- name: CreateHistoricalUnresolvedSurveyAnswer :one
INSERT INTO survey_v1_unresolved_answers (source_key_digest, source_payload_digest, source_field_digest, source_id, submission_id, submission_source_id, question_source_id, question_type, question_title_snapshot, selected_option_ids, selected_option_texts, selected_option_scores, selected_option_tags, text_value, score_contribution, created_at)
VALUES (sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_field_digest), sqlc.arg(source_id), sqlc.arg(submission_id), sqlc.arg(submission_source_id), sqlc.arg(question_source_id), sqlc.arg(question_type), sqlc.arg(question_title_snapshot), sqlc.arg(selected_option_ids), sqlc.arg(selected_option_texts), sqlc.arg(selected_option_scores), sqlc.arg(selected_option_tags), sqlc.arg(text_value), sqlc.arg(score_contribution), sqlc.arg(created_at)) RETURNING *;

-- name: GetHistoricalUnresolvedSurveyAnswer :one
SELECT * FROM survey_v1_unresolved_answers WHERE id=$1;

-- name: ListHistoricalUnresolvedSurveySubmissions :many
SELECT * FROM survey_v1_unresolved_submissions WHERE (sqlc.narg(questionnaire_id)::bigint IS NULL OR questionnaire_id=sqlc.narg(questionnaire_id)::bigint) ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalUnresolvedSurveySubmissions :one
SELECT count(*) FROM survey_v1_unresolved_submissions WHERE (sqlc.narg(questionnaire_id)::bigint IS NULL OR questionnaire_id=sqlc.narg(questionnaire_id)::bigint);

-- name: ListHistoricalUnresolvedSurveyAnswers :many
SELECT * FROM survey_v1_unresolved_answers WHERE submission_id=sqlc.arg(submission_id) ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalUnresolvedSurveyAnswers :one
SELECT count(*) FROM survey_v1_unresolved_answers WHERE submission_id=$1;
