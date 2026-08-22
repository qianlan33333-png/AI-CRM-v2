-- +goose Up
-- Survey Operations persists only local opaque configuration and queued test
-- records. It creates no provider client, webhook payload, River job, or
-- automatic retry path.
CREATE TABLE public.questionnaire_operations (
  questionnaire_id BIGINT PRIMARY KEY REFERENCES public.questionnaires(id) ON DELETE CASCADE,
  navigation_target_id TEXT NOT NULL DEFAULT '',
  channel_id BIGINT NOT NULL DEFAULT 0,
  external_push_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  external_push_configuration_reference TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT questionnaire_operations_navigation_target CHECK (
    btrim(navigation_target_id) = navigation_target_id
    AND char_length(navigation_target_id) <= 128
    AND position('://' IN navigation_target_id) = 0
  ),
  CONSTRAINT questionnaire_operations_channel CHECK (
    channel_id >= 0
    AND (navigation_target_id <> '' OR channel_id = 0)
  ),
  CONSTRAINT questionnaire_operations_external_push_reference CHECK (
    btrim(external_push_configuration_reference) = external_push_configuration_reference
    AND char_length(external_push_configuration_reference) <= 128
    AND position('://' IN external_push_configuration_reference) = 0
    AND (
      (external_push_enabled = FALSE AND external_push_configuration_reference = '')
      OR (external_push_enabled = TRUE AND external_push_configuration_reference <> '')
    )
  ),
  CONSTRAINT questionnaire_operations_timestamps CHECK (updated_at >= created_at)
);

CREATE TABLE public.questionnaire_operations_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN (
    'operations_completion_save',
    'operations_external_push_save',
    'operations_external_push_test_queue'
  )),
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress' CHECK (state IN ('in_progress', 'completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT questionnaire_operations_receipts_actor CHECK (
    btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200
  ),
  CONSTRAINT questionnaire_operations_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT questionnaire_operations_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT questionnaire_operations_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_questionnaire_operations_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.questionnaire_operations_receipts
    WHERE id = NEW.id AND state = 'completed'
  ) THEN
    RAISE EXCEPTION 'questionnaire operations receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER questionnaire_operations_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.questionnaire_operations_receipts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_questionnaire_operations_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_questionnaire_operations_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'questionnaire operations receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed questionnaire operations receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.operation <> OLD.operation
    OR NEW.actor_scope <> OLD.actor_scope
    OR NEW.key_digest <> OLD.key_digest
    OR NEW.payload_digest <> OLD.payload_digest
    OR NEW.created_at <> OLD.created_at
    OR NEW.state <> 'completed'
    OR NEW.result_snapshot IS NULL
    OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid questionnaire operations receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER questionnaire_operations_receipts_transition
BEFORE UPDATE OR DELETE ON public.questionnaire_operations_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_questionnaire_operations_receipt_transition_valid();

CREATE TABLE public.questionnaire_external_push_test_runs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  questionnaire_id BIGINT NOT NULL REFERENCES public.questionnaires(id) ON DELETE CASCADE,
  operation_receipt_id BIGINT NOT NULL UNIQUE REFERENCES public.questionnaire_operations_receipts(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status = 'queued'),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count = 0),
  side_effect_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (side_effect_executed = FALSE),
  provider_result_received BOOLEAN NOT NULL DEFAULT FALSE CHECK (provider_result_received = FALSE),
  unknown_after_dispatch BOOLEAN NOT NULL DEFAULT FALSE CHECK (unknown_after_dispatch = FALSE),
  auto_retry_allowed BOOLEAN NOT NULL DEFAULT FALSE CHECK (auto_retry_allowed = FALSE),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT questionnaire_external_push_test_runs_timestamps CHECK (updated_at >= created_at)
);
CREATE INDEX questionnaire_external_push_test_runs_questionnaire_created_idx
  ON public.questionnaire_external_push_test_runs (questionnaire_id, created_at DESC, id DESC);
CREATE INDEX questionnaire_external_push_test_runs_created_idx
  ON public.questionnaire_external_push_test_runs (created_at DESC, id DESC);

-- +goose Down
DROP INDEX public.questionnaire_external_push_test_runs_created_idx;
DROP INDEX public.questionnaire_external_push_test_runs_questionnaire_created_idx;
DROP TABLE public.questionnaire_external_push_test_runs;
DROP TRIGGER questionnaire_operations_receipts_transition ON public.questionnaire_operations_receipts;
DROP FUNCTION public.aicrm_questionnaire_operations_receipt_transition_valid();
DROP TRIGGER questionnaire_operations_receipts_complete_before_commit ON public.questionnaire_operations_receipts;
DROP FUNCTION public.aicrm_questionnaire_operations_receipt_complete_before_commit();
DROP TABLE public.questionnaire_operations_receipts;
DROP TABLE public.questionnaire_operations;
