-- name: InsertHistoricalQuestionnaire :one
INSERT INTO public.questionnaires (
  slug, name, title, description, answer_display_mode,
  assessment_enabled, assessment_config, is_disabled, created_by,
  version, submission_count, created_at, updated_at
)
VALUES (
  sqlc.arg(slug)::text, sqlc.arg(name)::text, sqlc.arg(title)::text,
  sqlc.arg(description)::text, sqlc.arg(answer_display_mode)::text,
  sqlc.arg(assessment_enabled)::boolean, sqlc.arg(assessment_config)::jsonb,
  sqlc.arg(is_disabled)::boolean, sqlc.arg(created_by)::bigint,
  sqlc.arg(version)::integer, sqlc.arg(submission_count)::integer,
  sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
)
RETURNING id;

-- name: InsertHistoricalQuestion :one
INSERT INTO public.questionnaire_questions (
  questionnaire_id, type, title, required, sort_order, placeholder_text,
  assessment_dimension_key, sidebar_profile_field, validation,
  created_at, updated_at
)
VALUES (
  sqlc.arg(questionnaire_id)::bigint, sqlc.arg(question_type)::text,
  sqlc.arg(title)::text, sqlc.arg(required)::boolean, sqlc.arg(sort_order)::integer,
  sqlc.arg(placeholder_text)::text, sqlc.arg(assessment_dimension_key)::text,
  sqlc.arg(sidebar_profile_field)::text, sqlc.arg(validation)::jsonb,
  sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
)
RETURNING id;

-- name: InsertHistoricalOption :one
INSERT INTO public.questionnaire_options (
  question_id, option_text, score, assessment_type_key, tag_codes,
  is_other, other_placeholder, other_max_length, sort_order,
  created_at, updated_at
)
VALUES (
  sqlc.arg(question_id)::bigint, sqlc.arg(option_text)::text,
  sqlc.arg(score)::double precision, sqlc.arg(assessment_type_key)::text,
  sqlc.arg(tag_codes)::jsonb, sqlc.arg(is_other)::boolean,
  sqlc.arg(other_placeholder)::text, sqlc.arg(other_max_length)::integer,
  sqlc.arg(sort_order)::integer, sqlc.arg(created_at)::timestamptz,
  sqlc.arg(updated_at)::timestamptz
)
RETURNING id;

-- name: InsertHistoricalSubmission :one
INSERT INTO public.questionnaire_submissions (
  questionnaire_id, respondent_key, openid, unionid, external_userid,
  customer_name, follow_user_userid, matched_by, mobile, source_channel,
  campaign_id, staff_id, total_score, final_tags, result_token,
  redirect_url_snapshot, submitted_at, created_at
)
VALUES (
  sqlc.arg(questionnaire_id)::bigint, '', '', sqlc.arg(unionid)::text, '', '',
  sqlc.arg(follow_user_userid)::text, sqlc.arg(matched_by)::text,
  sqlc.arg(mobile)::text, sqlc.arg(source_channel)::text,
  sqlc.arg(campaign_id)::bigint, sqlc.arg(staff_id)::bigint,
  sqlc.arg(total_score)::double precision, sqlc.arg(final_tags)::jsonb,
  sqlc.arg(result_token)::text, sqlc.arg(redirect_url_snapshot)::text,
  sqlc.arg(submitted_at)::timestamptz, sqlc.arg(created_at)::timestamptz
)
RETURNING id;

-- name: InsertHistoricalAnswer :one
INSERT INTO public.questionnaire_submission_answers (
  submission_id, question_id, question_type, question_title, sort_order,
  selected_options, text_value, created_at
)
VALUES (
  sqlc.arg(submission_id)::bigint, sqlc.arg(question_id)::bigint,
  sqlc.arg(question_type)::text, sqlc.arg(question_title)::text,
  sqlc.arg(sort_order)::integer, sqlc.arg(selected_options)::jsonb,
  sqlc.arg(text_value)::text, sqlc.arg(created_at)::timestamptz
)
RETURNING id;
