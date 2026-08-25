-- AI Audience local configuration closure.
--
-- This migration stores immutable typed snapshots of the owning Segment
-- definition. It does not start an automation agent, invoke a provider, or
-- enqueue/send a message.

-- +goose Up
ALTER TABLE public.ai_audience_package_automation_bindings
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE public.ai_audience_package_automation_bindings
  ADD CONSTRAINT ai_audience_package_automation_bindings_version CHECK (version >= 1);

ALTER TABLE public.ai_audience_local_configuration_receipts
  DROP CONSTRAINT ai_audience_local_configuration_receipts_operation;
ALTER TABLE public.ai_audience_local_configuration_receipts
  ADD CONSTRAINT ai_audience_local_configuration_receipts_operation CHECK (
    operation IN ('automation_binding_put', 'automation_binding_delete', 'senders_put', 'configuration_version_put', 'configuration_materialize')
  );

CREATE TABLE public.ai_audience_package_configuration_versions (
  package_id       BIGINT NOT NULL REFERENCES public.ai_audience_package_metadata(segment_id) ON DELETE CASCADE,
  version          BIGINT NOT NULL,
  schema_version   TEXT NOT NULL,
  package_version  BIGINT NOT NULL,
  definition       JSONB NOT NULL,
  definition_digest BYTEA NOT NULL,
  refresh_mode     TEXT NOT NULL,
  refresh_cron     TEXT,
  created_by       BIGINT NOT NULL REFERENCES public.admin_users(id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (package_id, version),
  CONSTRAINT ai_audience_package_configuration_versions_version CHECK (version >= 1),
  CONSTRAINT ai_audience_package_configuration_versions_schema CHECK (schema_version = 'ai_audience_local_configuration.v1'),
  CONSTRAINT ai_audience_package_configuration_versions_package_version CHECK (package_version >= 1),
  CONSTRAINT ai_audience_package_configuration_versions_definition CHECK (jsonb_typeof(definition) = 'object'),
  CONSTRAINT ai_audience_package_configuration_versions_digest CHECK (octet_length(definition_digest) = 32),
  CONSTRAINT ai_audience_package_configuration_versions_refresh CHECK (
    (refresh_mode = 'manual' AND refresh_cron IS NULL)
    OR (refresh_mode = 'scheduled' AND refresh_cron = btrim(refresh_cron) AND refresh_cron <> '')
  )
);

-- Versions are append-only. A later configuration change creates a later
-- local snapshot, so a stale writer can be rejected with a CAS version.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_configuration_version_immutable()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  RAISE EXCEPTION 'AI Audience configuration versions are immutable'
    USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER ai_audience_configuration_version_immutable
BEFORE UPDATE OR DELETE ON public.ai_audience_package_configuration_versions
FOR EACH ROW EXECUTE FUNCTION public.aicrm_ai_audience_configuration_version_immutable();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.ai_audience_package_configuration_versions)
     OR EXISTS (SELECT 1 FROM public.ai_audience_package_automation_bindings WHERE version <> 1)
     OR EXISTS (
       SELECT 1 FROM public.ai_audience_local_configuration_receipts
       WHERE operation IN ('configuration_version_put', 'configuration_materialize')
     ) THEN
    RAISE EXCEPTION 'cannot roll back populated AI Audience local configuration closure facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER ai_audience_configuration_version_immutable
  ON public.ai_audience_package_configuration_versions;
DROP FUNCTION public.aicrm_ai_audience_configuration_version_immutable();
DROP TABLE public.ai_audience_package_configuration_versions;
ALTER TABLE public.ai_audience_package_automation_bindings
  DROP CONSTRAINT ai_audience_package_automation_bindings_version;
ALTER TABLE public.ai_audience_package_automation_bindings
  DROP COLUMN version;
ALTER TABLE public.ai_audience_local_configuration_receipts
  DROP CONSTRAINT ai_audience_local_configuration_receipts_operation;
ALTER TABLE public.ai_audience_local_configuration_receipts
  ADD CONSTRAINT ai_audience_local_configuration_receipts_operation CHECK (
    operation IN ('automation_binding_put', 'automation_binding_delete', 'senders_put')
  );
