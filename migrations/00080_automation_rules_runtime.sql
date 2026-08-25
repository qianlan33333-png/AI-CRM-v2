-- +goose Up
-- A01 owns rule configuration and the durable local execution ledger.  It
-- deliberately stores only local snapshots/digests: provider effects belong
-- to Outbound + External Effects Runtime and are linked by an opaque ID.
CREATE TABLE public.automations (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  automation_code TEXT NOT NULL UNIQUE,
  automation_name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'paused',
  current_version BIGINT NOT NULL DEFAULT 1,
  trigger_type TEXT NOT NULL,
  condition_json JSONB NOT NULL,
  action_json JSONB NOT NULL,
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT automations_code CHECK (automation_code ~ '^[a-z0-9_-]{1,120}$'),
  CONSTRAINT automations_name CHECK (btrim(automation_name) <> '' AND char_length(automation_name) <= 120),
  CONSTRAINT automations_status CHECK (status IN ('active','paused','archived')),
  CONSTRAINT automations_version CHECK (current_version > 0),
  CONSTRAINT automations_trigger CHECK (trigger_type = 'customer.tag_applied'),
  CONSTRAINT automations_condition CHECK (jsonb_typeof(condition_json) = 'object'),
  CONSTRAINT automations_action CHECK (jsonb_typeof(action_json) = 'object'),
  CONSTRAINT automations_actors CHECK (created_by > 0 AND updated_by > 0),
  CONSTRAINT automations_times CHECK (updated_at >= created_at)
);
CREATE INDEX automations_active_trigger_idx ON public.automations(trigger_type, id) WHERE status = 'active';

-- Every changed or activated version is retained as the source of an
-- enrollment snapshot.  Executions never reread a mutable rule definition.
CREATE TABLE public.automation_rule_versions (
  automation_id BIGINT NOT NULL REFERENCES public.automations(id) ON DELETE RESTRICT,
  version BIGINT NOT NULL,
  trigger_type TEXT NOT NULL,
  condition_json JSONB NOT NULL,
  action_json JSONB NOT NULL,
  published_at TIMESTAMPTZ NOT NULL,
  published_by BIGINT NOT NULL,
  PRIMARY KEY (automation_id, version),
  CONSTRAINT automation_rule_versions_version CHECK (version > 0),
  CONSTRAINT automation_rule_versions_trigger CHECK (trigger_type = 'customer.tag_applied'),
  CONSTRAINT automation_rule_versions_condition CHECK (jsonb_typeof(condition_json) = 'object'),
  CONSTRAINT automation_rule_versions_action CHECK (jsonb_typeof(action_json) = 'object'),
  CONSTRAINT automation_rule_versions_actor CHECK (published_by > 0)
);

CREATE TABLE public.automation_enrollments (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  automation_id BIGINT NOT NULL,
  automation_version BIGINT NOT NULL,
  source_event_id BIGINT NOT NULL,
  customer_id BIGINT NOT NULL,
  trigger_payload JSONB NOT NULL,
  state TEXT NOT NULL DEFAULT 'enrolled',
  enrolled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  CONSTRAINT automation_enrollments_rule_version FOREIGN KEY (automation_id, automation_version)
    REFERENCES public.automation_rule_versions(automation_id, version) ON DELETE RESTRICT,
  CONSTRAINT automation_enrollments_source CHECK (source_event_id > 0 AND customer_id > 0),
  CONSTRAINT automation_enrollments_payload CHECK (jsonb_typeof(trigger_payload) = 'object'),
  CONSTRAINT automation_enrollments_state CHECK (state IN ('enrolled','completed','final_failed','outcome_unknown')),
  CONSTRAINT automation_enrollments_completion CHECK ((state = 'enrolled' AND completed_at IS NULL) OR (state <> 'enrolled' AND completed_at IS NOT NULL)),
  UNIQUE (automation_id, source_event_id)
);
CREATE INDEX automation_enrollments_customer_idx ON public.automation_enrollments(customer_id, enrolled_at DESC, id DESC);
CREATE INDEX automation_enrollments_open_idx ON public.automation_enrollments(id) WHERE state = 'enrolled';

CREATE TABLE public.automation_execution_actions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  enrollment_id BIGINT NOT NULL UNIQUE REFERENCES public.automation_enrollments(id) ON DELETE RESTRICT,
  action_type TEXT NOT NULL,
  action_snapshot JSONB NOT NULL,
  state TEXT NOT NULL DEFAULT 'queued',
  external_effect_id TEXT,
  receipt_digest TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  CONSTRAINT automation_execution_actions_type CHECK (action_type IN ('record','outbound_message')),
  CONSTRAINT automation_execution_actions_snapshot CHECK (jsonb_typeof(action_snapshot) = 'object'),
  CONSTRAINT automation_execution_actions_state CHECK (state IN ('queued','completed','final_failed','outcome_unknown')),
  CONSTRAINT automation_execution_actions_effect CHECK (external_effect_id IS NULL OR external_effect_id ~ '^eer_[1-9][0-9]*$'),
  CONSTRAINT automation_execution_actions_receipt CHECK (receipt_digest IS NULL OR receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT automation_execution_actions_completion CHECK ((state = 'queued' AND completed_at IS NULL) OR (state <> 'queued' AND completed_at IS NOT NULL))
);
CREATE INDEX automation_execution_actions_state_idx ON public.automation_execution_actions(state, id) WHERE state = 'queued';

CREATE TABLE public.automation_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  result_snapshot JSONB,
  completed_at TIMESTAMPTZ,
  CONSTRAINT automation_operation_receipts_operation CHECK (operation IN ('create','update','set_status')),
  CONSTRAINT automation_operation_receipts_actor CHECK (actor_scope ~ '^admin:[1-9][0-9]*$'),
  CONSTRAINT automation_operation_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT automation_operation_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT automation_operation_receipts_completed CHECK ((result_snapshot IS NULL AND completed_at IS NULL) OR (result_snapshot IS NOT NULL AND completed_at IS NOT NULL)),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.automations LIMIT 1)
     OR EXISTS (SELECT 1 FROM public.automation_operation_receipts LIMIT 1) THEN
    RAISE EXCEPTION 'cannot roll back populated automation runtime' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.automation_operation_receipts;
DROP TABLE public.automation_execution_actions;
DROP TABLE public.automation_enrollments;
DROP TABLE public.automation_rule_versions;
DROP INDEX public.automations_active_trigger_idx;
DROP TABLE public.automations;
