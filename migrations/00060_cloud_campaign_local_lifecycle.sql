-- +goose Up
-- CRM-local control-plane facts only. These tables deliberately contain no
-- recipient, provider, runtime, or external-effect state.
CREATE TABLE public.cloud_campaigns (
  campaign_code TEXT PRIMARY KEY CHECK (campaign_code <> ''),
  name TEXT NOT NULL CHECK (name <> ''),
  approval_status TEXT NOT NULL CHECK (approval_status IN ('draft','approved','rejected')),
  runtime_status TEXT NOT NULL CHECK (runtime_status IN ('idle','planned','paused')),
  version BIGINT NOT NULL CHECK (version > 0),
  created_by BIGINT NOT NULL, updated_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE public.cloud_campaign_steps (
  campaign_code TEXT NOT NULL REFERENCES public.cloud_campaigns(campaign_code) ON DELETE CASCADE,
  step_index INTEGER NOT NULL CHECK (step_index > 0), delay_minutes INTEGER NOT NULL CHECK (delay_minutes >= 0),
  content TEXT NOT NULL CHECK (content <> ''), PRIMARY KEY (campaign_code, step_index)
);
CREATE TABLE public.cloud_campaign_local_plans (
  id BIGSERIAL PRIMARY KEY, campaign_code TEXT NOT NULL REFERENCES public.cloud_campaigns(campaign_code) ON DELETE RESTRICT,
  campaign_version BIGINT NOT NULL CHECK (campaign_version > 0), step_count INTEGER NOT NULL CHECK (step_count > 0), created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE public.cloud_campaign_local_commands (
  id BIGSERIAL PRIMARY KEY, operation TEXT NOT NULL CHECK (operation IN ('start','batch_start')),
  campaign_code TEXT NOT NULL REFERENCES public.cloud_campaigns(campaign_code) ON DELETE RESTRICT,
  plan_id BIGINT NOT NULL REFERENCES public.cloud_campaign_local_plans(id) ON DELETE RESTRICT,
  real_send BOOLEAN NOT NULL DEFAULT FALSE CHECK (real_send = FALSE), runtime_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (runtime_executed = FALSE),
  created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE public.cloud_campaign_operation_receipts (
  id BIGSERIAL PRIMARY KEY, actor_id BIGINT NOT NULL, key_digest BYTEA NOT NULL, operation TEXT NOT NULL,
  payload_digest BYTEA NOT NULL, state TEXT NOT NULL CHECK (state IN ('reserved','completed')),
  result_json JSONB, created_at TIMESTAMPTZ NOT NULL, completed_at TIMESTAMPTZ,
  UNIQUE (actor_id, key_digest)
);
CREATE INDEX cloud_campaigns_status_idx ON public.cloud_campaigns(approval_status, runtime_status, campaign_code);

-- +goose Down
LOCK TABLE public.cloud_campaign_local_commands, public.cloud_campaign_local_plans, public.cloud_campaign_operation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.cloud_campaign_local_commands) OR EXISTS (SELECT 1 FROM public.cloud_campaign_operation_receipts WHERE state = 'completed') THEN
    RAISE EXCEPTION 'cannot roll back recorded cloud campaign command facts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.cloud_campaign_operation_receipts;
DROP TABLE public.cloud_campaign_local_commands;
DROP TABLE public.cloud_campaign_local_plans;
DROP TABLE public.cloud_campaign_steps;
DROP TABLE public.cloud_campaigns;
