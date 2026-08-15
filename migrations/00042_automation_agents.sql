-- +goose Up
-- Automation owns this local configuration board. It deliberately has no
-- tenant, provider, worker, River, generation, or outbound-send semantics.
CREATE TABLE public.automation_agent_configurations (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  agent_name TEXT NOT NULL,
  agent_code TEXT NOT NULL,
  automation_type TEXT NOT NULL,
  status TEXT NOT NULL,
  draft_role_prompt TEXT NOT NULL DEFAULT '',
  draft_task_prompt TEXT NOT NULL DEFAULT '',
  published_role_prompt TEXT NOT NULL DEFAULT '',
  published_task_prompt TEXT NOT NULL DEFAULT '',
  draft_version BIGINT NOT NULL DEFAULT 1,
  published_version BIGINT NOT NULL DEFAULT 1,
  fixed_content_package_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT automation_agent_configurations_name CHECK (btrim(agent_name) <> '' AND char_length(agent_name) <= 120),
  CONSTRAINT automation_agent_configurations_code CHECK (agent_code ~ '^[a-z0-9_-]{1,120}$'),
  CONSTRAINT automation_agent_configurations_type CHECK (automation_type IN ('agent', 'fixed_script')),
  CONSTRAINT automation_agent_configurations_status CHECK (status IN ('active', 'paused', 'archived')),
  CONSTRAINT automation_agent_configurations_versions CHECK (draft_version >= 1 AND published_version >= 1 AND published_version <= draft_version),
  CONSTRAINT automation_agent_configurations_actor CHECK (created_by > 0 AND updated_by > 0),
  CONSTRAINT automation_agent_configurations_times CHECK (updated_at >= created_at),
  CONSTRAINT automation_agent_configurations_fixed_content CHECK (jsonb_typeof(fixed_content_package_json) = 'object'),
  UNIQUE (agent_code)
);
CREATE INDEX automation_agent_configurations_visible_type_updated
  ON public.automation_agent_configurations (automation_type, updated_at DESC, id DESC) WHERE status <> 'archived';
CREATE INDEX automation_agent_configurations_visible_updated
  ON public.automation_agent_configurations (updated_at DESC, id DESC) WHERE status <> 'archived';

-- Reservation, state change, and Events-owned append occur in one UoW.
CREATE TABLE public.automation_agent_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'reserved',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT automation_agent_receipts_operation CHECK (operation IN ('create', 'update', 'copy', 'publish', 'set_status', 'fixed_content')),
  CONSTRAINT automation_agent_receipts_actor_scope CHECK (actor_scope ~ '^admin:[1-9][0-9]*$'),
  CONSTRAINT automation_agent_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT automation_agent_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT automation_agent_receipts_state CHECK (state IN ('reserved', 'completed')),
  CONSTRAINT automation_agent_receipts_completion CHECK (
    (state = 'reserved' AND result_snapshot IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose Down
DROP TABLE public.automation_agent_operation_receipts;
DROP INDEX public.automation_agent_configurations_visible_updated;
DROP INDEX public.automation_agent_configurations_visible_type_updated;
DROP TABLE public.automation_agent_configurations;
