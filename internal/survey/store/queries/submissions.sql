-- name: GetQuestionnaireSubmissionResults :one
SELECT q.id AS questionnaire_id,
       COUNT(s.id) AS submission_count,
       MAX(s.submitted_at) AS latest_submitted_at,
       COALESCE(AVG(s.total_score), 0)::double precision AS average_score
FROM questionnaires q
LEFT JOIN questionnaire_submissions s ON s.questionnaire_id = q.id
WHERE q.id = sqlc.arg(questionnaire_id)::bigint
GROUP BY q.id;

-- name: CountQuestionnaireSubmissions :one
SELECT COUNT(*) FROM questionnaire_submissions s
WHERE s.questionnaire_id = sqlc.arg(questionnaire_id)::bigint;

-- name: QuestionnaireSubmissionOwnerExists :one
SELECT EXISTS(SELECT 1 FROM questionnaires q WHERE q.id = sqlc.arg(questionnaire_id)::bigint) AS owner_exists;

-- name: ListQuestionnaireSubmissions :many
SELECT s.id, s.questionnaire_id, s.respondent_key, s.openid, s.unionid, s.external_userid,
       s.customer_name, s.follow_user_userid, s.matched_by, s.mobile,
       s.source_channel, s.campaign_id, s.staff_id,
       s.total_score, s.final_tags, s.result_token, s.redirect_url_snapshot,
       s.submitted_at, s.created_at,
       COALESCE((SELECT jsonb_agg(jsonb_build_object(
         'question_id', a.question_id, 'question_type', a.question_type,
         'question_title', a.question_title, 'sort_order', a.sort_order,
         'selected_options', a.selected_options, 'text_value', a.text_value,
         'created_at', a.created_at
       ) ORDER BY a.sort_order, a.id)
       FROM questionnaire_submission_answers a WHERE a.submission_id = s.id), '[]'::jsonb) AS answers
FROM questionnaire_submissions s
WHERE s.questionnaire_id = sqlc.arg(questionnaire_id)::bigint
ORDER BY s.submitted_at DESC, s.id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: GetQuestionnaireSubmissionExportDefinition :one
SELECT q.slug,
       COALESCE((SELECT jsonb_agg(jsonb_build_object(
         'id', qq.id, 'title', qq.title, 'sort_order', qq.sort_order
       ) ORDER BY qq.sort_order)
       FROM questionnaire_questions qq WHERE qq.questionnaire_id = q.id), '[]'::jsonb) AS questions
FROM questionnaires q WHERE q.id = sqlc.arg(questionnaire_id)::bigint;

-- name: ListQuestionnaireSubmissionExportRows :many
SELECT s.id, s.questionnaire_id, s.respondent_key, s.openid, s.unionid, s.external_userid,
       s.customer_name, s.follow_user_userid, s.matched_by, s.mobile,
       s.source_channel, s.campaign_id, s.staff_id,
       s.total_score, s.final_tags, s.result_token, s.redirect_url_snapshot,
       s.submitted_at, s.created_at,
       COALESCE((SELECT jsonb_agg(jsonb_build_object(
         'question_id', a.question_id, 'question_type', a.question_type,
         'question_title', a.question_title, 'sort_order', a.sort_order,
         'selected_options', a.selected_options, 'text_value', a.text_value,
         'created_at', a.created_at
       ) ORDER BY a.sort_order, a.id)
       FROM questionnaire_submission_answers a WHERE a.submission_id = s.id), '[]'::jsonb) AS answers
FROM questionnaire_submissions s
WHERE s.questionnaire_id = sqlc.arg(questionnaire_id)::bigint
ORDER BY s.submitted_at DESC, s.id DESC
LIMIT sqlc.arg(row_limit)::integer;
