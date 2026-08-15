-- +goose Up
-- Admin Ops owns local control-plane state only. Credential material is never
-- stored here: rows may contain a secret-store reference and a display mask.
CREATE TABLE public.admin_ops_credentials (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  credential_kind TEXT NOT NULL,
  client_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  state TEXT NOT NULL,
  secret_ref TEXT NOT NULL,
  secret_mask TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  version BIGINT NOT NULL DEFAULT 1,
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT admin_ops_credentials_kind CHECK (credential_kind IN ('direct_api_key', 'api_client')),
  CONSTRAINT admin_ops_credentials_client CHECK (btrim(client_id) = client_id AND client_id <> '' AND char_length(client_id) <= 120),
  CONSTRAINT admin_ops_credentials_name CHECK (btrim(display_name) = display_name AND display_name <> '' AND char_length(display_name) <= 200),
  CONSTRAINT admin_ops_credentials_state CHECK (state IN ('pending_activation', 'active', 'disabled')),
  CONSTRAINT admin_ops_credentials_secret_ref CHECK (secret_ref ~ '^secret://[A-Za-z0-9._/-]{1,240}$'),
  CONSTRAINT admin_ops_credentials_secret_mask CHECK (secret_mask <> '' AND char_length(secret_mask) <= 80),
  CONSTRAINT admin_ops_credentials_actor CHECK (btrim(created_by) = created_by AND created_by <> '' AND char_length(created_by) <= 200),
  CONSTRAINT admin_ops_credentials_version CHECK (version > 0),
  CONSTRAINT admin_ops_credentials_updated CHECK (updated_at >= created_at),
  UNIQUE (credential_kind, client_id),
  UNIQUE (secret_ref)
);

CREATE TABLE public.admin_ops_config_categories (
  category_key TEXT PRIMARY KEY,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  settings JSONB NOT NULL DEFAULT '{}'::jsonb,
  version BIGINT NOT NULL DEFAULT 1,
  updated_by TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT admin_ops_categories_key CHECK (category_key ~ '^[a-z][a-z0-9_]{1,79}$'),
  CONSTRAINT admin_ops_categories_version CHECK (version > 0),
  CONSTRAINT admin_ops_categories_actor CHECK (btrim(updated_by) = updated_by AND updated_by <> '' AND char_length(updated_by) <= 200)
);

CREATE TABLE public.admin_ops_config_releases (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  state TEXT NOT NULL,
  changes JSONB NOT NULL,
  checksum TEXT NOT NULL,
  based_on_release_id BIGINT,
  rollback_of_release_id BIGINT,
  created_by TEXT NOT NULL,
  published_by TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  validated_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  CONSTRAINT admin_ops_releases_state CHECK (state IN ('draft', 'validated', 'published', 'rolled_back')),
  CONSTRAINT admin_ops_releases_checksum CHECK (checksum ~ '^[a-f0-9]{64}$'),
  CONSTRAINT admin_ops_releases_actor CHECK (btrim(created_by) = created_by AND created_by <> '' AND char_length(created_by) <= 200),
  CONSTRAINT admin_ops_releases_publish_actor CHECK (published_by IS NULL OR (btrim(published_by) = published_by AND published_by <> '' AND char_length(published_by) <= 200)),
  CONSTRAINT admin_ops_releases_base_fk FOREIGN KEY (based_on_release_id) REFERENCES public.admin_ops_config_releases(id) ON DELETE RESTRICT,
  CONSTRAINT admin_ops_releases_rollback_fk FOREIGN KEY (rollback_of_release_id) REFERENCES public.admin_ops_config_releases(id) ON DELETE RESTRICT,
  CONSTRAINT admin_ops_releases_timestamps CHECK (
    (state = 'draft' AND validated_at IS NULL AND published_at IS NULL) OR
    (state = 'validated' AND validated_at IS NOT NULL AND published_at IS NULL) OR
    (state IN ('published', 'rolled_back') AND validated_at IS NOT NULL AND published_at IS NOT NULL)
  )
);
CREATE INDEX admin_ops_config_releases_created_idx ON public.admin_ops_config_releases (created_at DESC, id DESC);

CREATE TABLE public.admin_ops_jobs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  job_key TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  state TEXT NOT NULL,
  target_ref TEXT NOT NULL,
  request_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_summary JSONB,
  version BIGINT NOT NULL DEFAULT 1,
  requested_by TEXT NOT NULL,
  failure_code TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT admin_ops_jobs_key CHECK (job_key ~ '^admjob_[A-Za-z0-9_-]{12,80}$'),
  CONSTRAINT admin_ops_jobs_kind CHECK (kind IN ('archive_sync', 'message_batch_ack', 'feishu_webhook_validate', 'feishu_hourly_report', 'order_identity_repair')),
  CONSTRAINT admin_ops_jobs_state CHECK (state IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'outcome_unknown', 'retired')),
  CONSTRAINT admin_ops_jobs_target CHECK (btrim(target_ref) = target_ref AND target_ref <> '' AND char_length(target_ref) <= 240),
  CONSTRAINT admin_ops_jobs_actor CHECK (btrim(requested_by) = requested_by AND requested_by <> '' AND char_length(requested_by) <= 200),
  CONSTRAINT admin_ops_jobs_version CHECK (version > 0),
  CONSTRAINT admin_ops_jobs_times CHECK (updated_at >= created_at),
  CONSTRAINT admin_ops_jobs_completion CHECK (
    (state IN ('queued', 'running') AND completed_at IS NULL) OR
    (state IN ('completed', 'failed', 'cancelled', 'outcome_unknown', 'retired') AND completed_at IS NOT NULL)
  )
);
CREATE INDEX admin_ops_jobs_state_created_idx ON public.admin_ops_jobs (state, created_at DESC, id DESC);
CREATE INDEX admin_ops_jobs_kind_created_idx ON public.admin_ops_jobs (kind, created_at DESC, id DESC);

CREATE TABLE public.admin_ops_action_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  action TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT admin_ops_receipts_action CHECK (action ~ '^[a-z][a-z0-9_.-]{2,120}$'),
  CONSTRAINT admin_ops_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT admin_ops_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT admin_ops_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT admin_ops_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT admin_ops_receipts_completion CHECK ((state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)),
  UNIQUE (action, actor_scope, key_digest)
);

CREATE TABLE public.admin_ops_notification_settings (
  channel TEXT PRIMARY KEY,
  enabled BOOLEAN NOT NULL,
  secret_ref TEXT NOT NULL,
  secret_mask TEXT NOT NULL,
  validation_state TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT admin_ops_notifications_channel CHECK (channel = 'feishu'),
  CONSTRAINT admin_ops_notifications_ref CHECK (secret_ref ~ '^secret://[A-Za-z0-9._/-]{1,240}$' OR secret_ref ~ '^secretref:[A-Za-z0-9._/-]{1,240}$'),
  CONSTRAINT admin_ops_notifications_mask CHECK (secret_mask <> '' AND char_length(secret_mask) <= 80),
  CONSTRAINT admin_ops_notifications_state CHECK (validation_state IN ('unconfigured', 'unverified', 'queued', 'valid', 'invalid')),
  CONSTRAINT admin_ops_notifications_actor CHECK (btrim(updated_by) = updated_by AND updated_by <> '' AND char_length(updated_by) <= 200)
);

-- +goose Down
DROP TABLE public.admin_ops_notification_settings;
DROP TABLE public.admin_ops_action_receipts;
DROP TABLE public.admin_ops_jobs;
DROP TABLE public.admin_ops_config_releases;
DROP TABLE public.admin_ops_config_categories;
DROP TABLE public.admin_ops_credentials;
