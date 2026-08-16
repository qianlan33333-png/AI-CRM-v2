-- +goose Up
-- This is a deliberately detached, read-only projection. It is not a second
-- outbound state machine and it has no provider, worker, tenant, or event API.
CREATE TABLE public.push_center_read_model_state (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
  production_data_ready BOOLEAN NOT NULL DEFAULT FALSE,
  fixture_mode BOOLEAN NOT NULL DEFAULT FALSE,
  allow_fixture_repo_in_prod BOOLEAN NOT NULL DEFAULT FALSE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO public.push_center_read_model_state (singleton) VALUES (TRUE);

CREATE TABLE public.push_center_read_model_entries (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  section TEXT NOT NULL,
  effect_type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  effective_status TEXT NOT NULL,
  business_type TEXT NOT NULL DEFAULT '',
  business_id TEXT NOT NULL DEFAULT '',
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  external_userid TEXT NOT NULL DEFAULT '',
  owner_userid TEXT NOT NULL DEFAULT '',
  trace_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL DEFAULT '',
  source_module TEXT NOT NULL DEFAULT '',
  source_route TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT push_center_read_model_entries_section CHECK (section IN ('questionnaire', 'order', 'ai_assist', 'private_broadcast', 'group_ops', 'group_broadcast', 'customer_webhook', 'tags', 'welcome', 'payment', 'integrations', 'test_receiver', 'other')),
  CONSTRAINT push_center_read_model_entries_status CHECK (status IN ('pending', 'running', 'succeeded', 'sent', 'simulated', 'unknown_after_dispatch', 'failed', 'sent_with_shadow_warning', 'shadow_failed_not_business_failed')),
  CONSTRAINT push_center_read_model_entries_effective_status CHECK (effective_status IN ('pending', 'running', 'succeeded', 'sent', 'simulated', 'unknown_after_dispatch', 'failed', 'sent_with_shadow_warning', 'shadow_failed_not_business_failed', 'reconciled')),
  CONSTRAINT push_center_read_model_entries_text_trimmed CHECK (
    effect_type = btrim(effect_type) AND business_type = btrim(business_type) AND business_id = btrim(business_id)
    AND target_type = btrim(target_type) AND target_id = btrim(target_id) AND external_userid = btrim(external_userid)
    AND owner_userid = btrim(owner_userid) AND trace_id = btrim(trace_id) AND idempotency_key = btrim(idempotency_key)
    AND source_module = btrim(source_module) AND source_route = btrim(source_route)
  )
);

CREATE INDEX push_center_read_model_entries_section_created_idx ON public.push_center_read_model_entries (section, created_at DESC, id DESC);
CREATE INDEX push_center_read_model_entries_status_created_idx ON public.push_center_read_model_entries (status, created_at DESC, id DESC);
CREATE INDEX push_center_read_model_entries_effective_status_created_idx ON public.push_center_read_model_entries (effective_status, created_at DESC, id DESC);
CREATE INDEX push_center_read_model_entries_created_idx ON public.push_center_read_model_entries (created_at DESC, id DESC);
CREATE INDEX push_center_read_model_entries_text_trgm_idx ON public.push_center_read_model_entries USING gin (
  effect_type gin_trgm_ops, business_type gin_trgm_ops, business_id gin_trgm_ops, target_type gin_trgm_ops,
  target_id gin_trgm_ops, external_userid gin_trgm_ops, owner_userid gin_trgm_ops, trace_id gin_trgm_ops,
  idempotency_key gin_trgm_ops, source_module gin_trgm_ops, source_route gin_trgm_ops
);

-- +goose Down
DROP INDEX public.push_center_read_model_entries_text_trgm_idx;
DROP INDEX public.push_center_read_model_entries_created_idx;
DROP INDEX public.push_center_read_model_entries_effective_status_created_idx;
DROP INDEX public.push_center_read_model_entries_status_created_idx;
DROP INDEX public.push_center_read_model_entries_section_created_idx;
DROP TABLE public.push_center_read_model_entries;
DROP TABLE public.push_center_read_model_state;
