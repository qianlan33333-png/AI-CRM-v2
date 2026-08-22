-- CRM-local AI Audience automation selection and sender whitelist metadata.
--
-- This migration neither starts an automation agent nor resolves, synchronizes,
-- or sends to a provider identity. The authoritative local staff read model
-- remains public.staff; this schema stores only package-local references.

-- +goose Up
CREATE TABLE public.ai_audience_package_automation_bindings (
  package_id          BIGINT PRIMARY KEY
                        REFERENCES public.ai_audience_package_metadata(segment_id)
                        ON DELETE CASCADE,
  automation_agent_id BIGINT NOT NULL
                        REFERENCES public.automation_agent_configurations(id)
                        ON DELETE RESTRICT,
  created_by          BIGINT NOT NULL,
  updated_by          BIGINT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ai_audience_package_automation_bindings_actors CHECK (
    created_by > 0 AND updated_by > 0
  ),
  CONSTRAINT ai_audience_package_automation_bindings_timestamps CHECK (
    created_at <= updated_at
  )
);

CREATE INDEX idx_ai_audience_package_automation_bindings_agent
  ON public.ai_audience_package_automation_bindings (automation_agent_id, package_id);

-- An agent may be selected by only one non-archived package. The package
-- lifecycle lives in a different table, so a partial unique index cannot
-- express this invariant. Serialize contenders for one agent before checking
-- the joined lifecycle state; the package row is locked by the caller first.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_automation_binding_conflict()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  PERFORM pg_advisory_xact_lock(
    hashtextextended(
      'ai_audience.package.automation_binding.v1:' || NEW.automation_agent_id::text,
      0
    )
  );

  IF EXISTS (
    SELECT 1
    FROM public.ai_audience_package_automation_bindings AS binding
    JOIN public.ai_audience_package_metadata AS metadata
      ON metadata.segment_id = binding.package_id
    WHERE binding.automation_agent_id = NEW.automation_agent_id
      AND binding.package_id <> NEW.package_id
      AND metadata.lifecycle <> 'archived'
  ) THEN
    RAISE EXCEPTION 'automation agent is already bound by a non-archived AI Audience package'
      USING ERRCODE = '23505';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER ai_audience_package_automation_binding_conflict
BEFORE INSERT OR UPDATE OF package_id, automation_agent_id
ON public.ai_audience_package_automation_bindings
FOR EACH ROW
EXECUTE FUNCTION public.aicrm_ai_audience_automation_binding_conflict();

-- An archived package may retain its local reference for audit/history. A
-- later activation must re-check the same uniqueness rule before it becomes a
-- non-archived owner again.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_package_activation_binding_conflict()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
  selected_agent_id BIGINT;
BEGIN
  IF OLD.lifecycle = 'archived' AND NEW.lifecycle <> 'archived' THEN
    SELECT binding.automation_agent_id
      INTO selected_agent_id
    FROM public.ai_audience_package_automation_bindings AS binding
    WHERE binding.package_id = NEW.segment_id;

    IF selected_agent_id IS NOT NULL THEN
      PERFORM pg_advisory_xact_lock(
        hashtextextended(
          'ai_audience.package.automation_binding.v1:' || selected_agent_id::text,
          0
        )
      );
      IF EXISTS (
        SELECT 1
        FROM public.ai_audience_package_automation_bindings AS binding
        JOIN public.ai_audience_package_metadata AS metadata
          ON metadata.segment_id = binding.package_id
        WHERE binding.automation_agent_id = selected_agent_id
          AND binding.package_id <> NEW.segment_id
          AND metadata.lifecycle <> 'archived'
      ) THEN
        RAISE EXCEPTION 'automation agent is already bound by a non-archived AI Audience package'
          USING ERRCODE = '23505';
      END IF;
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER ai_audience_package_activation_binding_conflict
BEFORE UPDATE OF lifecycle
ON public.ai_audience_package_metadata
FOR EACH ROW
EXECUTE FUNCTION public.aicrm_ai_audience_package_activation_binding_conflict();

CREATE TABLE public.ai_audience_package_senders (
  package_id     BIGINT NOT NULL
                   REFERENCES public.ai_audience_package_metadata(segment_id)
                   ON DELETE CASCADE,
  sender_userid  TEXT NOT NULL,
  sort_order     SMALLINT NOT NULL,
  is_enabled     BOOLEAN NOT NULL,
  created_by     BIGINT NOT NULL,
  updated_by     BIGINT NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (package_id, sender_userid),
  CONSTRAINT ai_audience_package_senders_userid CHECK (
    sender_userid = btrim(sender_userid) AND sender_userid <> ''
  ),
  CONSTRAINT ai_audience_package_senders_sort_order CHECK (
    sort_order BETWEEN 1 AND 5
  ),
  CONSTRAINT ai_audience_package_senders_unique_order UNIQUE (package_id, sort_order),
  CONSTRAINT ai_audience_package_senders_actors CHECK (
    created_by > 0 AND updated_by > 0
  ),
  CONSTRAINT ai_audience_package_senders_timestamps CHECK (
    created_at <= updated_at
  )
);

CREATE TABLE public.ai_audience_local_configuration_receipts (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation      TEXT NOT NULL,
  actor_id       BIGINT NOT NULL,
  key_digest     BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state          TEXT NOT NULL DEFAULT 'in_progress',
  result_json    JSONB,
  created_at     TIMESTAMPTZ NOT NULL,
  completed_at   TIMESTAMPTZ,
  CONSTRAINT ai_audience_local_configuration_receipts_operation CHECK (
    operation IN ('automation_binding_put', 'automation_binding_delete', 'senders_put')
  ),
  CONSTRAINT ai_audience_local_configuration_receipts_actor CHECK (actor_id > 0),
  CONSTRAINT ai_audience_local_configuration_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT ai_audience_local_configuration_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT ai_audience_local_configuration_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT ai_audience_local_configuration_receipts_result CHECK (
    result_json IS NULL OR jsonb_typeof(result_json) = 'object'
  ),
  CONSTRAINT ai_audience_local_configuration_receipts_completion CHECK (
    (state = 'in_progress' AND result_json IS NULL AND completed_at IS NULL)
    OR
    (state = 'completed' AND result_json IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_id, key_digest)
);

-- Each receipt and its corresponding local configuration change complete in
-- one transaction. Completed receipts are immutable.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_local_configuration_receipt_complete_before_commit()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM public.ai_audience_local_configuration_receipts
    WHERE id = NEW.id
      AND state = 'completed'
      AND result_json IS NOT NULL
      AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'AI Audience local configuration receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER ai_audience_local_configuration_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.ai_audience_local_configuration_receipts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION public.aicrm_ai_audience_local_configuration_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_local_configuration_receipt_transition()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed AI Audience local configuration receipts are immutable'
      USING ERRCODE = '55000';
  END IF;

  IF NEW.state <> 'completed'
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.result_json IS NULL
     OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid AI Audience local configuration receipt transition'
      USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER ai_audience_local_configuration_receipts_transition
BEFORE UPDATE OR DELETE ON public.ai_audience_local_configuration_receipts
FOR EACH ROW
EXECUTE FUNCTION public.aicrm_ai_audience_local_configuration_receipt_transition();

-- +goose Down
DROP TRIGGER ai_audience_local_configuration_receipts_transition
  ON public.ai_audience_local_configuration_receipts;
DROP TRIGGER ai_audience_local_configuration_receipts_complete_before_commit
  ON public.ai_audience_local_configuration_receipts;
DROP FUNCTION public.aicrm_ai_audience_local_configuration_receipt_transition();
DROP FUNCTION public.aicrm_ai_audience_local_configuration_receipt_complete_before_commit();
DROP TABLE public.ai_audience_local_configuration_receipts;
DROP TABLE public.ai_audience_package_senders;
DROP TRIGGER ai_audience_package_activation_binding_conflict
  ON public.ai_audience_package_metadata;
DROP FUNCTION public.aicrm_ai_audience_package_activation_binding_conflict();
DROP TRIGGER ai_audience_package_automation_binding_conflict
  ON public.ai_audience_package_automation_bindings;
DROP FUNCTION public.aicrm_ai_audience_automation_binding_conflict();
DROP INDEX public.idx_ai_audience_package_automation_bindings_agent;
DROP TABLE public.ai_audience_package_automation_bindings;
