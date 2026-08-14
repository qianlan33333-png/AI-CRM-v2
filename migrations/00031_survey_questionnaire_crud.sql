-- +goose Up
CREATE TABLE public.questionnaires (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  answer_display_mode TEXT NOT NULL,
  assessment_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  assessment_config JSONB NOT NULL DEFAULT '{}',
  is_disabled BOOLEAN NOT NULL DEFAULT FALSE,
  created_by BIGINT NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  submission_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT questionnaires_slug CHECK (btrim(slug) = slug AND char_length(slug) <= 200),
  CONSTRAINT questionnaires_name CHECK (btrim(name) = name AND name <> '' AND char_length(name) <= 120),
  CONSTRAINT questionnaires_title CHECK (btrim(title) = title AND title <> '' AND char_length(title) <= 300),
  CONSTRAINT questionnaires_description CHECK (btrim(description) = description AND char_length(description) <= 10000),
  CONSTRAINT questionnaires_answer_mode CHECK (answer_display_mode IN ('all_in_one', 'one_by_one')),
  CONSTRAINT questionnaires_f01_assessment CHECK (NOT assessment_enabled AND assessment_config = '{}'::jsonb),
  CONSTRAINT questionnaires_actor CHECK (created_by > 0),
  CONSTRAINT questionnaires_version CHECK (version > 0),
  CONSTRAINT questionnaires_submission_count CHECK (submission_count >= 0),
  CONSTRAINT questionnaires_timestamps CHECK (updated_at >= created_at)
);
CREATE UNIQUE INDEX questionnaires_slug_unique ON public.questionnaires (slug) WHERE slug <> '';

CREATE TABLE public.questionnaire_questions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  questionnaire_id BIGINT NOT NULL REFERENCES public.questionnaires(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  title TEXT NOT NULL,
  required BOOLEAN NOT NULL,
  sort_order INTEGER NOT NULL,
  placeholder_text TEXT NOT NULL DEFAULT '',
  assessment_dimension_key TEXT NOT NULL DEFAULT '',
  sidebar_profile_field TEXT NOT NULL DEFAULT '',
  validation JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT questionnaire_questions_type CHECK (type IN ('single_choice', 'multi_choice', 'textarea', 'mobile')),
  CONSTRAINT questionnaire_questions_title CHECK (btrim(title) = title AND title <> '' AND char_length(title) <= 500),
  CONSTRAINT questionnaire_questions_sort CHECK (sort_order >= 0),
  CONSTRAINT questionnaire_questions_placeholder CHECK (btrim(placeholder_text) = placeholder_text AND char_length(placeholder_text) <= 500),
  CONSTRAINT questionnaire_questions_dimension CHECK (btrim(assessment_dimension_key) = assessment_dimension_key AND char_length(assessment_dimension_key) <= 200),
  CONSTRAINT questionnaire_questions_profile CHECK (btrim(sidebar_profile_field) = sidebar_profile_field AND char_length(sidebar_profile_field) <= 200),
  CONSTRAINT questionnaire_questions_validation CHECK (jsonb_typeof(validation) = 'object'),
  CONSTRAINT questionnaire_questions_timestamps CHECK (updated_at >= created_at),
  UNIQUE (questionnaire_id, sort_order)
);

CREATE TABLE public.questionnaire_options (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  question_id BIGINT NOT NULL REFERENCES public.questionnaire_questions(id) ON DELETE CASCADE,
  option_text TEXT NOT NULL,
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  assessment_type_key TEXT NOT NULL DEFAULT '',
  tag_codes JSONB NOT NULL DEFAULT '[]',
  is_other BOOLEAN NOT NULL DEFAULT FALSE,
  other_placeholder TEXT NOT NULL DEFAULT '',
  other_max_length INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT questionnaire_options_text CHECK (btrim(option_text) = option_text AND option_text <> '' AND char_length(option_text) <= 500),
  CONSTRAINT questionnaire_options_score CHECK (score NOT IN ('NaN'::double precision, 'Infinity'::double precision, '-Infinity'::double precision)),
  CONSTRAINT questionnaire_options_assessment_key CHECK (btrim(assessment_type_key) = assessment_type_key AND char_length(assessment_type_key) <= 200),
  CONSTRAINT questionnaire_options_tag_codes CHECK (jsonb_typeof(tag_codes) = 'array'),
  CONSTRAINT questionnaire_options_other CHECK (
    (is_other AND other_max_length BETWEEN 1 AND 2000 AND btrim(other_placeholder) = other_placeholder AND char_length(other_placeholder) <= 500) OR
    (NOT is_other AND other_placeholder = '' AND other_max_length = 0)
  ),
  CONSTRAINT questionnaire_options_sort CHECK (sort_order >= 0),
  CONSTRAINT questionnaire_options_timestamps CHECK (updated_at >= created_at),
  UNIQUE (question_id, sort_order)
);

CREATE TABLE public.questionnaire_catalog_counters (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE,
  total_questionnaires BIGINT NOT NULL DEFAULT 0,
  CONSTRAINT questionnaire_catalog_counters_singleton CHECK (singleton),
  CONSTRAINT questionnaire_catalog_counters_total CHECK (total_questionnaires >= 0)
);
INSERT INTO public.questionnaire_catalog_counters (singleton, total_questionnaires) VALUES (TRUE, 0);

CREATE TABLE public.questionnaire_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT questionnaire_operation_receipts_operation CHECK (operation = 'create'),
  CONSTRAINT questionnaire_operation_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT questionnaire_operation_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT questionnaire_operation_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT questionnaire_operation_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT questionnaire_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_questionnaire_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.questionnaire_operation_receipts WHERE id = NEW.id AND state = 'completed') THEN
    RAISE EXCEPTION 'questionnaire operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER questionnaire_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.questionnaire_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_questionnaire_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_questionnaire_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed questionnaire operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid questionnaire operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER questionnaire_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.questionnaire_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_questionnaire_receipt_transition_valid();

-- +goose Down
DROP TRIGGER questionnaire_operation_receipts_transition ON public.questionnaire_operation_receipts;
DROP TRIGGER questionnaire_operation_receipts_complete_before_commit ON public.questionnaire_operation_receipts;
DROP TABLE public.questionnaire_operation_receipts;
DROP TABLE public.questionnaire_catalog_counters;
DROP TABLE public.questionnaire_options;
DROP TABLE public.questionnaire_questions;
DROP TABLE public.questionnaires;
DROP FUNCTION public.aicrm_questionnaire_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_questionnaire_receipt();
