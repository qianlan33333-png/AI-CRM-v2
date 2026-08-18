-- +goose Up
CREATE TABLE public.questionnaire_submissions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  questionnaire_id BIGINT NOT NULL REFERENCES public.questionnaires(id) ON DELETE CASCADE,
  respondent_key TEXT NOT NULL DEFAULT '',
  openid TEXT NOT NULL DEFAULT '',
  unionid TEXT NOT NULL DEFAULT '',
  external_userid TEXT NOT NULL DEFAULT '',
  customer_name TEXT NOT NULL DEFAULT '',
  follow_user_userid TEXT NOT NULL DEFAULT '',
  matched_by TEXT NOT NULL DEFAULT '',
  mobile TEXT NOT NULL DEFAULT '',
  source_channel TEXT NOT NULL DEFAULT '',
  campaign_id TEXT NOT NULL DEFAULT '',
  staff_id TEXT NOT NULL DEFAULT '',
  total_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  final_tags JSONB NOT NULL DEFAULT '[]',
  result_token TEXT NOT NULL DEFAULT '',
  redirect_url_snapshot TEXT NOT NULL DEFAULT '',
  submitted_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT questionnaire_submissions_respondent_key CHECK (btrim(respondent_key) = respondent_key AND char_length(respondent_key) <= 200),
  CONSTRAINT questionnaire_submissions_openid CHECK (btrim(openid) = openid AND char_length(openid) <= 200),
  CONSTRAINT questionnaire_submissions_unionid CHECK (btrim(unionid) = unionid AND char_length(unionid) <= 200),
  CONSTRAINT questionnaire_submissions_external_userid CHECK (btrim(external_userid) = external_userid AND char_length(external_userid) <= 200),
  CONSTRAINT questionnaire_submissions_customer_name CHECK (char_length(customer_name) <= 300),
  CONSTRAINT questionnaire_submissions_follow_user CHECK (btrim(follow_user_userid) = follow_user_userid AND char_length(follow_user_userid) <= 200),
  CONSTRAINT questionnaire_submissions_matched_by CHECK (btrim(matched_by) = matched_by AND char_length(matched_by) <= 50),
  CONSTRAINT questionnaire_submissions_mobile CHECK (btrim(mobile) = mobile AND char_length(mobile) <= 32),
  CONSTRAINT questionnaire_submissions_source_channel CHECK (btrim(source_channel) = source_channel AND char_length(source_channel) <= 100),
  CONSTRAINT questionnaire_submissions_campaign CHECK (btrim(campaign_id) = campaign_id AND char_length(campaign_id) <= 200),
  CONSTRAINT questionnaire_submissions_staff CHECK (btrim(staff_id) = staff_id AND char_length(staff_id) <= 200),
  CONSTRAINT questionnaire_submissions_total_score CHECK (total_score NOT IN ('NaN'::double precision, 'Infinity'::double precision, '-Infinity'::double precision)),
  CONSTRAINT questionnaire_submissions_final_tags CHECK (jsonb_typeof(final_tags) = 'array'),
  CONSTRAINT questionnaire_submissions_result_token CHECK (btrim(result_token) = result_token AND char_length(result_token) <= 200),
  CONSTRAINT questionnaire_submissions_redirect_url CHECK (char_length(redirect_url_snapshot) <= 2000)
);
CREATE INDEX questionnaire_submissions_page
  ON public.questionnaire_submissions (questionnaire_id, submitted_at DESC, id DESC);

CREATE TABLE public.questionnaire_submission_answers (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  submission_id BIGINT NOT NULL REFERENCES public.questionnaire_submissions(id) ON DELETE CASCADE,
  question_id BIGINT NOT NULL,
  question_type TEXT NOT NULL,
  question_title TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  selected_options JSONB NOT NULL DEFAULT '[]',
  text_value TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT questionnaire_submission_answers_question CHECK (question_id > 0),
  CONSTRAINT questionnaire_submission_answers_type CHECK (question_type IN ('single_choice', 'multi_choice', 'textarea', 'mobile')),
  CONSTRAINT questionnaire_submission_answers_title CHECK (btrim(question_title) = question_title AND question_title <> '' AND char_length(question_title) <= 500),
  CONSTRAINT questionnaire_submission_answers_sort CHECK (sort_order >= 0),
  CONSTRAINT questionnaire_submission_answers_options CHECK (jsonb_typeof(selected_options) = 'array'),
  CONSTRAINT questionnaire_submission_answers_text CHECK (char_length(text_value) <= 10000),
  UNIQUE (submission_id, question_id)
);
CREATE INDEX questionnaire_submission_answers_submission
  ON public.questionnaire_submission_answers (submission_id, sort_order, id);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_questionnaire_submission_count_sync()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE public.questionnaires SET submission_count = submission_count + 1 WHERE id = NEW.questionnaire_id;
    RETURN NEW;
  END IF;
  IF TG_OP = 'DELETE' THEN
    UPDATE public.questionnaires SET submission_count = submission_count - 1
    WHERE id = OLD.questionnaire_id AND submission_count > 0;
    RETURN OLD;
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER questionnaire_submissions_count_sync
AFTER INSERT OR DELETE ON public.questionnaire_submissions
FOR EACH ROW EXECUTE FUNCTION public.aicrm_questionnaire_submission_count_sync();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_questionnaire_submission_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'questionnaire submission snapshots are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER questionnaire_submissions_immutable
BEFORE UPDATE ON public.questionnaire_submissions
FOR EACH ROW EXECUTE FUNCTION public.aicrm_questionnaire_submission_immutable();
CREATE TRIGGER questionnaire_submission_answers_immutable
BEFORE UPDATE ON public.questionnaire_submission_answers
FOR EACH ROW EXECUTE FUNCTION public.aicrm_questionnaire_submission_immutable();

-- +goose Down
DROP TRIGGER questionnaire_submission_answers_immutable ON public.questionnaire_submission_answers;
DROP TRIGGER questionnaire_submissions_immutable ON public.questionnaire_submissions;
DROP TRIGGER questionnaire_submissions_count_sync ON public.questionnaire_submissions;
DROP FUNCTION public.aicrm_questionnaire_submission_immutable();
DROP FUNCTION public.aicrm_questionnaire_submission_count_sync();
DROP TABLE public.questionnaire_submission_answers;
DROP TABLE public.questionnaire_submissions;
