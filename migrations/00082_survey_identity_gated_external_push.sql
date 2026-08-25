-- +goose Up
-- 00079/00080/00081 are independently reserved. This package only requires
-- its own migration to have been applied; do not infer a maximum migration.
-- The OAuth nonce remains owned by auth.admin_oauth_states. H5 callback state
-- is claimed there exactly once; no raw OAuth code or identity is persisted.
CREATE TABLE public.questionnaire_submission_external_push_bindings (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  questionnaire_id BIGINT NOT NULL REFERENCES public.questionnaires(id) ON DELETE RESTRICT,
  public_submission_id BIGINT NOT NULL UNIQUE REFERENCES public.questionnaire_public_submissions(id) ON DELETE RESTRICT,
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  external_effect_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  source_ref_digest TEXT NOT NULL CHECK (source_ref_digest ~ '^sha256:[0-9a-f]{64}$'),
  target_ref_digest TEXT NOT NULL CHECK (target_ref_digest ~ '^sha256:[0-9a-f]{64}$'),
  payload_digest TEXT NOT NULL CHECK (payload_digest ~ '^sha256:[0-9a-f]{64}$'),
  policy_version_hash TEXT NOT NULL CHECK (policy_version_hash ~ '^sha256:[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX questionnaire_submission_external_push_bindings_questionnaire_idx
  ON public.questionnaire_submission_external_push_bindings(questionnaire_id, id DESC);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_survey_external_push_binding_effect_kind()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.external_effects e
    WHERE e.id = NEW.external_effect_id AND e.owner = 'survey' AND e.kind = 'survey_webhook'
  ) THEN
    RAISE EXCEPTION 'survey push binding requires survey_webhook effect' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER questionnaire_submission_external_push_bindings_effect_kind
BEFORE INSERT OR UPDATE ON public.questionnaire_submission_external_push_bindings
FOR EACH ROW EXECUTE FUNCTION public.aicrm_survey_external_push_binding_effect_kind();

-- This is an immutable operator projection. `provider_accepted` is a provider
-- transport acknowledgement only; `delivery_proven` remains false until a
-- separately verified delivery receipt is recorded. Unknown dispatches have
-- no retry flag and must be manually reconciled through External Effects.
CREATE TABLE public.questionnaire_external_push_delivery_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  binding_id BIGINT NOT NULL REFERENCES public.questionnaire_submission_external_push_bindings(id) ON DELETE RESTRICT,
  effect_attempt_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effect_attempts(id) ON DELETE RESTRICT,
  provider_accepted BOOLEAN NOT NULL DEFAULT FALSE,
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE,
  evidence_digest TEXT CHECK (evidence_digest IS NULL OR evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((delivery_proven = FALSE AND evidence_digest IS NULL) OR (delivery_proven = TRUE AND evidence_digest IS NOT NULL))
);
CREATE INDEX questionnaire_external_push_delivery_receipts_binding_idx
  ON public.questionnaire_external_push_delivery_receipts(binding_id, id DESC);

-- Binding identity and digest facts cannot be retargeted after acceptance.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_survey_external_push_binding_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR NEW.questionnaire_id IS DISTINCT FROM OLD.questionnaire_id
     OR NEW.public_submission_id IS DISTINCT FROM OLD.public_submission_id
     OR NEW.customer_id IS DISTINCT FROM OLD.customer_id
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id
     OR NEW.source_ref_digest IS DISTINCT FROM OLD.source_ref_digest
     OR NEW.target_ref_digest IS DISTINCT FROM OLD.target_ref_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.policy_version_hash IS DISTINCT FROM OLD.policy_version_hash
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'survey external push binding is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER questionnaire_submission_external_push_bindings_immutable
BEFORE UPDATE OR DELETE ON public.questionnaire_submission_external_push_bindings
FOR EACH ROW EXECUTE FUNCTION public.aicrm_survey_external_push_binding_immutable();
CREATE TRIGGER questionnaire_external_push_delivery_receipts_no_delete
BEFORE DELETE ON public.questionnaire_external_push_delivery_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_external_effects_reject_delete();

-- +goose Down
LOCK TABLE public.questionnaire_external_push_delivery_receipts, public.questionnaire_submission_external_push_bindings IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.questionnaire_external_push_delivery_receipts)
     OR EXISTS (SELECT 1 FROM public.questionnaire_submission_external_push_bindings) THEN
    RAISE EXCEPTION 'cannot roll back populated survey external push facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER questionnaire_external_push_delivery_receipts_no_delete ON public.questionnaire_external_push_delivery_receipts;
DROP TRIGGER questionnaire_submission_external_push_bindings_immutable ON public.questionnaire_submission_external_push_bindings;
DROP FUNCTION public.aicrm_survey_external_push_binding_immutable();
DROP TRIGGER questionnaire_submission_external_push_bindings_effect_kind ON public.questionnaire_submission_external_push_bindings;
DROP FUNCTION public.aicrm_survey_external_push_binding_effect_kind();
DROP TABLE public.questionnaire_external_push_delivery_receipts;
DROP TABLE public.questionnaire_submission_external_push_bindings;
