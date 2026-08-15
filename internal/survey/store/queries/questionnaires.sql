-- name: ListQuestionnairesOffset :many
SELECT q.id, q.slug, q.name, q.title, q.description, q.answer_display_mode,
       q.assessment_enabled, q.assessment_config, q.is_disabled, q.created_by,
       q.version, q.submission_count, q.created_at, q.updated_at,
       COALESCE((SELECT jsonb_agg(jsonb_build_object(
         'id', qq.id, 'type', qq.type, 'title', qq.title,
         'assessment_dimension_key', qq.assessment_dimension_key,
         'sidebar_profile_field', qq.sidebar_profile_field, 'required', qq.required,
         'sort_order', qq.sort_order, 'placeholder_text', qq.placeholder_text,
         'validation', qq.validation, 'options', COALESCE((SELECT jsonb_agg(jsonb_build_object(
           'id', qo.id, 'option_text', qo.option_text, 'score', qo.score,
           'assessment_type_key', qo.assessment_type_key, 'tag_codes', qo.tag_codes,
           'is_other', qo.is_other, 'other_placeholder', qo.other_placeholder,
           'other_max_length', qo.other_max_length, 'sort_order', qo.sort_order
         ) ORDER BY qo.sort_order) FROM questionnaire_options qo WHERE qo.question_id = qq.id), '[]'::jsonb)
       ) ORDER BY qq.sort_order) FROM questionnaire_questions qq WHERE qq.questionnaire_id = q.id), '[]'::jsonb) AS questions
FROM questionnaires q ORDER BY q.id
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountQuestionnaires :one
SELECT total_questionnaires FROM questionnaire_catalog_counters WHERE singleton = TRUE;

-- name: GetQuestionnaire :one
SELECT q.id, q.slug, q.name, q.title, q.description, q.answer_display_mode,
       q.assessment_enabled, q.assessment_config, q.is_disabled, q.created_by,
       q.version, q.submission_count, q.created_at, q.updated_at,
       COALESCE((SELECT jsonb_agg(jsonb_build_object(
         'id', qq.id, 'type', qq.type, 'title', qq.title,
         'assessment_dimension_key', qq.assessment_dimension_key,
         'sidebar_profile_field', qq.sidebar_profile_field, 'required', qq.required,
         'sort_order', qq.sort_order, 'placeholder_text', qq.placeholder_text,
         'validation', qq.validation, 'options', COALESCE((SELECT jsonb_agg(jsonb_build_object(
           'id', qo.id, 'option_text', qo.option_text, 'score', qo.score,
           'assessment_type_key', qo.assessment_type_key, 'tag_codes', qo.tag_codes,
           'is_other', qo.is_other, 'other_placeholder', qo.other_placeholder,
           'other_max_length', qo.other_max_length, 'sort_order', qo.sort_order
         ) ORDER BY qo.sort_order) FROM questionnaire_options qo WHERE qo.question_id = qq.id), '[]'::jsonb)
       ) ORDER BY qq.sort_order) FROM questionnaire_questions qq WHERE qq.questionnaire_id = q.id), '[]'::jsonb) AS questions
FROM questionnaires q WHERE q.id = sqlc.arg(questionnaire_id)::bigint;

-- name: CreateQuestionnaire :one
INSERT INTO questionnaires (slug, name, title, description, answer_display_mode, assessment_enabled,
  assessment_config, is_disabled, created_by, created_at, updated_at)
VALUES (sqlc.arg(slug)::text, sqlc.arg(name)::text, sqlc.arg(title)::text,
  sqlc.arg(description)::text, sqlc.arg(answer_display_mode)::text, FALSE, '{}'::jsonb,
  sqlc.arg(is_disabled)::boolean, sqlc.arg(created_by)::bigint,
  sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz)
RETURNING id;

-- name: FinalizeQuestionnaireSlug :one
UPDATE questionnaires SET slug = CASE WHEN slug = '' THEN 'questionnaire-' || id::text ELSE slug END
WHERE id = sqlc.arg(questionnaire_id)::bigint RETURNING slug;

-- name: InsertQuestionnaireQuestion :one
INSERT INTO questionnaire_questions (questionnaire_id, type, title, required, sort_order, placeholder_text,
  assessment_dimension_key, sidebar_profile_field, validation, created_at, updated_at)
VALUES (sqlc.arg(questionnaire_id)::bigint, sqlc.arg(question_type)::text, sqlc.arg(title)::text,
  sqlc.arg(required)::boolean, sqlc.arg(sort_order)::integer, sqlc.arg(placeholder_text)::text,
  sqlc.arg(assessment_dimension_key)::text, sqlc.arg(sidebar_profile_field)::text,
  sqlc.arg(validation)::jsonb, sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz)
RETURNING id;

-- name: InsertQuestionnaireOption :one
INSERT INTO questionnaire_options (question_id, option_text, score, assessment_type_key, tag_codes,
  is_other, other_placeholder, other_max_length, sort_order, created_at, updated_at)
VALUES (sqlc.arg(question_id)::bigint, sqlc.arg(option_text)::text, sqlc.arg(score)::double precision,
  sqlc.arg(assessment_type_key)::text, sqlc.arg(tag_codes)::jsonb, sqlc.arg(is_other)::boolean,
  sqlc.arg(other_placeholder)::text, sqlc.arg(other_max_length)::integer, sqlc.arg(sort_order)::integer,
  sqlc.arg(created_at)::timestamptz, sqlc.arg(created_at)::timestamptz)
RETURNING id;

-- name: IncrementQuestionnaireCount :one
UPDATE questionnaire_catalog_counters SET total_questionnaires = total_questionnaires + 1
WHERE singleton = TRUE RETURNING total_questionnaires;

-- name: DecrementQuestionnaireCount :one
UPDATE questionnaire_catalog_counters SET total_questionnaires = total_questionnaires - 1
WHERE singleton = TRUE AND total_questionnaires > 0 RETURNING total_questionnaires;

-- name: UpdateQuestionnaire :one
UPDATE questionnaires
SET slug = sqlc.arg(slug)::text,
    name = sqlc.arg(name)::text,
    title = sqlc.arg(title)::text,
    description = sqlc.arg(description)::text,
    answer_display_mode = sqlc.arg(answer_display_mode)::text,
    is_disabled = sqlc.arg(is_disabled)::boolean,
    version = version + 1,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(questionnaire_id)::bigint
RETURNING id;

-- name: DeleteQuestionnaireChildren :exec
DELETE FROM questionnaire_questions WHERE questionnaire_id = sqlc.arg(questionnaire_id)::bigint;

-- name: SetQuestionnaireDisabled :one
UPDATE questionnaires SET is_disabled = sqlc.arg(is_disabled)::boolean,
  version = version + 1, updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(questionnaire_id)::bigint
RETURNING id;

-- name: DeleteDisabledQuestionnaire :one
DELETE FROM questionnaires WHERE id = sqlc.arg(questionnaire_id)::bigint AND is_disabled = TRUE
RETURNING id;

-- name: ReserveQuestionnaireOperationReceipt :one
INSERT INTO questionnaire_operation_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea,
  sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetQuestionnaireOperationReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM questionnaire_operation_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteQuestionnaireOperationReceipt :one
UPDATE questionnaire_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: ReserveQuestionnaireManagementReceipt :one
INSERT INTO questionnaire_management_receipts (operation, actor_scope, key_digest, payload_digest, created_at)
VALUES (sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea,
  sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetQuestionnaireManagementReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM questionnaire_management_receipts
WHERE operation = sqlc.arg(operation)::text AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: CompleteQuestionnaireManagementReceipt :one
UPDATE questionnaire_management_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;
