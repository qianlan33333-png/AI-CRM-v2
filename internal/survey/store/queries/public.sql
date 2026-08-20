-- name: FindPublicSurveyDefinitionBySlug :one
SELECT d.id AS definition_id, d.questionnaire_id, d.definition_version, d.state,
       d.answer_display_mode, d.title, d.description
FROM questionnaire_public_definitions d
WHERE d.slug = sqlc.arg(slug)::text AND d.state = 'public';

-- name: FindPublicSurveyDefinitionByVersion :one
SELECT id AS definition_id, questionnaire_id, definition_version, state,
       answer_display_mode, title, description
FROM questionnaire_public_definitions
WHERE questionnaire_id = sqlc.arg(questionnaire_id)::bigint
  AND definition_version = sqlc.arg(definition_version)::bigint;

-- name: LookupPublicSurveyResult :one
SELECT s.id AS submission_id, d.definition_version, s.submitted_at
FROM questionnaire_public_submission_receipts r
JOIN questionnaire_public_submissions s ON s.receipt_id = r.id
JOIN questionnaire_public_definitions d ON d.id = r.definition_id
WHERE r.result_token_digest = sqlc.arg(result_token_digest)::bytea
  AND r.state = 'completed';
