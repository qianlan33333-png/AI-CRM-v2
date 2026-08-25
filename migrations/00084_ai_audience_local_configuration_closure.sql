-- AI Audience local configuration closure.
--
-- This migration stores immutable local configuration snapshots, safe sender
-- references and read-only send-record projections. It does not start an
-- automation agent, invoke a model/provider, or enqueue/send a message.

-- +goose Up
ALTER TABLE public.ai_audience_package_automation_bindings
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE public.ai_audience_package_automation_bindings
  ADD CONSTRAINT ai_audience_package_automation_bindings_version CHECK (version >= 1);

ALTER TABLE public.ai_audience_local_configuration_receipts
  DROP CONSTRAINT ai_audience_local_configuration_receipts_operation;
ALTER TABLE public.ai_audience_local_configuration_receipts
  ADD CONSTRAINT ai_audience_local_configuration_receipts_operation CHECK (
    operation IN ('automation_binding_put', 'automation_binding_delete', 'senders_put', 'configuration_version_put')
  );

CREATE TABLE public.ai_audience_package_configuration_versions (
  package_id      BIGINT NOT NULL REFERENCES public.ai_audience_package_metadata(segment_id) ON DELETE CASCADE,
  version         BIGINT NOT NULL,
  template_config JSONB NOT NULL,
  filter_config   JSONB NOT NULL,
  created_by      BIGINT NOT NULL REFERENCES public.admin_users(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (package_id, version),
  CONSTRAINT ai_audience_package_configuration_versions_version CHECK (version >= 1),
  CONSTRAINT ai_audience_package_configuration_versions_template CHECK (jsonb_typeof(template_config) = 'object'),
  CONSTRAINT ai_audience_package_configuration_versions_filter CHECK (jsonb_typeof(filter_config) = 'object')
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

-- Sender configuration deliberately has no provider credential, token, or
-- raw provider payload. sender_ref is a canonical local reference; its digest
-- is retained for immutable audit correlation only.
CREATE TABLE public.ai_audience_package_sender_security_config (
  package_id    BIGINT NOT NULL REFERENCES public.ai_audience_package_metadata(segment_id) ON DELETE CASCADE,
  sender_ref    TEXT NOT NULL,
  sender_digest BYTEA NOT NULL,
  is_enabled    BOOLEAN NOT NULL,
  version       BIGINT NOT NULL DEFAULT 1,
  created_by    BIGINT NOT NULL REFERENCES public.admin_users(id),
  updated_by    BIGINT NOT NULL REFERENCES public.admin_users(id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (package_id, sender_ref),
  CONSTRAINT ai_audience_package_sender_security_config_ref CHECK (
    sender_ref = btrim(sender_ref) AND sender_ref ~ '^staff:[^[:space:]]+$'
  ),
  CONSTRAINT ai_audience_package_sender_security_config_digest CHECK (octet_length(sender_digest) = 32),
  CONSTRAINT ai_audience_package_sender_security_config_version CHECK (version >= 1),
  CONSTRAINT ai_audience_package_sender_security_config_timestamps CHECK (created_at <= updated_at)
);

CREATE UNIQUE INDEX uq_ai_audience_sender_security_config_digest
  ON public.ai_audience_package_sender_security_config (package_id, sender_digest);

-- A future authorized local projection writer may insert a compact receipt,
-- but application routes in this package are read-only. Content, recipients,
-- provider response bodies and provider credentials are intentionally absent.
CREATE TABLE public.ai_audience_package_send_record_projections (
  package_id    BIGINT NOT NULL REFERENCES public.ai_audience_package_metadata(segment_id) ON DELETE CASCADE,
  record_id     UUID NOT NULL,
  record_digest BYTEA NOT NULL,
  state         TEXT NOT NULL,
  occurred_at   TIMESTAMPTZ NOT NULL,
  projected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (package_id, record_id),
  CONSTRAINT ai_audience_package_send_record_projections_digest CHECK (octet_length(record_digest) = 32),
  CONSTRAINT ai_audience_package_send_record_projections_state CHECK (state IN ('pending', 'unknown', 'sent', 'failed')),
  CONSTRAINT ai_audience_package_send_record_projections_times CHECK (occurred_at <= projected_at)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_send_record_projection_immutable()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  RAISE EXCEPTION 'AI Audience send record projections are read-only'
    USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER ai_audience_send_record_projection_immutable
BEFORE UPDATE OR DELETE ON public.ai_audience_package_send_record_projections
FOR EACH ROW EXECUTE FUNCTION public.aicrm_ai_audience_send_record_projection_immutable();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.ai_audience_package_configuration_versions)
     OR EXISTS (SELECT 1 FROM public.ai_audience_package_sender_security_config)
     OR EXISTS (SELECT 1 FROM public.ai_audience_package_send_record_projections)
     OR EXISTS (
       SELECT 1 FROM public.ai_audience_local_configuration_receipts
       WHERE operation = 'configuration_version_put'
     ) THEN
    RAISE EXCEPTION 'cannot roll back populated AI Audience local configuration closure facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER ai_audience_send_record_projection_immutable
  ON public.ai_audience_package_send_record_projections;
DROP FUNCTION public.aicrm_ai_audience_send_record_projection_immutable();
DROP TABLE public.ai_audience_package_send_record_projections;
DROP INDEX public.uq_ai_audience_sender_security_config_digest;
DROP TABLE public.ai_audience_package_sender_security_config;
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
