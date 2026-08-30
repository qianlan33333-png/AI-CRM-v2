-- +goose Up
-- Keep the complete V1-local Agent configuration editable without making a
-- migrated Agent executable. Runtime/provider activation remains a separate
-- gate; projected rows are always paused.
ALTER TABLE public.automation_agent_configurations
  ADD COLUMN legacy_configuration_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN execution_enabled BOOLEAN NOT NULL DEFAULT false,
  ADD CONSTRAINT automation_agent_configurations_legacy_configuration
    CHECK (jsonb_typeof(legacy_configuration_json) = 'object');

UPDATE public.automation_agent_configurations
SET status = 'paused',
    execution_enabled = false,
    fixed_content_package_json = jsonb_build_object(
      'content_text', COALESCE(fixed_content_package_json ->> 'content_text', ''),
      'image_library_ids', '[]'::jsonb,
      'miniprogram_library_ids', '[]'::jsonb,
      'attachment_library_ids', '[]'::jsonb,
      'group_invite_library_ids', '[]'::jsonb
    )
WHERE status <> 'archived';

ALTER TABLE public.automation_agent_configurations
  ADD CONSTRAINT automation_agent_configurations_execution_closed
    CHECK (execution_enabled = false AND status <> 'active');

CREATE TABLE public.automation_v1_editable_agent_projections (
  agent_history_id BIGINT PRIMARY KEY
    REFERENCES public.automation_v1_agent_history(id) ON DELETE RESTRICT,
  config_history_id BIGINT UNIQUE
    REFERENCES public.automation_v1_agent_config_history(id) ON DELETE RESTRICT,
  prompt_history_id BIGINT NOT NULL UNIQUE
    REFERENCES public.automation_v1_prompt_history(id) ON DELETE RESTRICT,
  agent_id BIGINT NOT NULL UNIQUE
    REFERENCES public.automation_agent_configurations(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT automation_v1_editable_agent_projection_created
    CHECK (created_at > '-infinity'::timestamptz)
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.automation_v1_editable_agent_projections) THEN
    RAISE EXCEPTION 'refusing to remove populated V1 editable Agent projections';
  END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE public.automation_v1_editable_agent_projections;
ALTER TABLE public.automation_agent_configurations
  DROP CONSTRAINT automation_agent_configurations_execution_closed,
  DROP CONSTRAINT automation_agent_configurations_legacy_configuration,
  DROP COLUMN execution_enabled,
  DROP COLUMN legacy_configuration_json;
