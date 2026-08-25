-- +goose Up
-- CH02 keeps Contact's business facts separate from the shared external-effects
-- runtime.  Only digests and bounded opaque references cross this boundary.
ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'contact_acquisition_asset_publish',
    'outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync',
    'survey_webhook','audience_webhook','order_payment_prepay',
    'order_payment_capture','order_refund','group_ops_broadcast','product_external_push_test'
  ));

CREATE TABLE public.channel_acquisition_asset_bindings (
  effect_id BIGINT PRIMARY KEY REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  channel_id BIGINT NOT NULL REFERENCES public.channels(id) ON DELETE RESTRICT,
  asset_kind TEXT NOT NULL CHECK (asset_kind IN ('contact_way_qrcode', 'customer_acquisition_link')),
  asset_version BIGINT NOT NULL CHECK (asset_version > 0),
  supersedes_version BIGINT NOT NULL DEFAULT 0 CHECK (supersedes_version >= 0 AND asset_version > supersedes_version),
  channel_revision BIGINT NOT NULL CHECK (channel_revision > 0),
  channel_code TEXT NOT NULL CHECK (btrim(channel_code) = channel_code AND char_length(channel_code) BETWEEN 1 AND 200),
  channel_name TEXT NOT NULL CHECK (btrim(channel_name) = channel_name AND char_length(channel_name) BETWEEN 1 AND 200),
  channel_status TEXT NOT NULL CHECK (channel_status = 'active'),
  scene_value TEXT NOT NULL DEFAULT '' CHECK (btrim(scene_value) = scene_value AND char_length(scene_value) <= 512),
  assignee_wecom_userids TEXT[] NOT NULL CHECK (cardinality(assignee_wecom_userids) BETWEEN 1 AND 200),
  snapshot_digest TEXT NOT NULL CHECK (snapshot_digest ~ '^sha256:[0-9a-f]{64}$'),
  idempotency_digest TEXT NOT NULL CHECK (idempotency_digest ~ '^sha256:[0-9a-f]{64}$'),
  envelope_fingerprint TEXT NOT NULL UNIQUE CHECK (envelope_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('accepted', 'queued', 'attempted', 'executed', 'outcome_unknown', 'reconciled', 'final_failed')),
  accept_receipt_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  accept_receipt_digest TEXT NOT NULL CHECK (accept_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  queue_receipt_id BIGINT UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  queue_receipt_digest TEXT CHECK (queue_receipt_digest IS NULL OR queue_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  river_job_id BIGINT,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BIGINT NOT NULL DEFAULT 0 CHECK (fence >= 0),
  lease_expires_at TIMESTAMPTZ,
  attempt_receipt_id BIGINT UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  attempt_receipt_digest TEXT CHECK (attempt_receipt_digest IS NULL OR attempt_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  provider_asset_reference_digest TEXT CHECK (provider_asset_reference_digest IS NULL OR provider_asset_reference_digest ~ '^sha256:[0-9a-f]{64}$'),
  provider_call_attempted BOOLEAN NOT NULL DEFAULT FALSE,
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE,
  reconcile_receipt_id BIGINT UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  reconcile_receipt_digest TEXT CHECK (reconcile_receipt_digest IS NULL OR reconcile_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  reconcile_evidence_digest TEXT CHECK (reconcile_evidence_digest IS NULL OR reconcile_evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  reconcile_resolution TEXT CHECK (reconcile_resolution IS NULL OR reconcile_resolution IN ('provider_applied', 'provider_not_applied')),
  reconciled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (channel_id, asset_kind, asset_version),
  UNIQUE (effect_id, channel_id, asset_kind, asset_version),
  UNIQUE (effect_id, generation, fence),
  CONSTRAINT channel_acquisition_asset_scene_shape CHECK (
    (asset_kind = 'contact_way_qrcode' AND scene_value <> '') OR
    (asset_kind = 'customer_acquisition_link')
  ),
  CONSTRAINT channel_acquisition_asset_lease_shape CHECK (
    (fence = 0 AND lease_expires_at IS NULL) OR (fence > 0 AND lease_expires_at IS NOT NULL)
  ),
  CONSTRAINT channel_acquisition_asset_state_shape CHECK (
    (state = 'accepted' AND queue_receipt_id IS NULL AND queue_receipt_digest IS NULL AND river_job_id IS NULL AND fence = 0 AND attempt_receipt_id IS NULL AND reconcile_receipt_id IS NULL) OR
    (state = 'queued' AND queue_receipt_id IS NOT NULL AND queue_receipt_digest IS NOT NULL AND river_job_id > 0 AND fence = 0 AND attempt_receipt_id IS NULL AND reconcile_receipt_id IS NULL) OR
    (state = 'attempted' AND queue_receipt_id IS NOT NULL AND river_job_id > 0 AND fence > 0 AND attempt_receipt_id IS NULL AND reconcile_receipt_id IS NULL) OR
    (state IN ('executed', 'final_failed', 'outcome_unknown') AND queue_receipt_id IS NOT NULL AND river_job_id > 0 AND fence > 0 AND attempt_receipt_id IS NOT NULL AND attempt_receipt_digest IS NOT NULL AND reconcile_receipt_id IS NULL) OR
    (state = 'reconciled' AND queue_receipt_id IS NOT NULL AND river_job_id > 0 AND fence > 0 AND attempt_receipt_id IS NOT NULL AND attempt_receipt_digest IS NOT NULL AND reconcile_receipt_id IS NOT NULL AND reconcile_receipt_digest IS NOT NULL AND reconcile_evidence_digest IS NOT NULL AND reconcile_resolution IS NOT NULL AND reconciled_at IS NOT NULL)
  )
);
CREATE INDEX channel_acquisition_asset_bindings_channel_state_idx ON public.channel_acquisition_asset_bindings(channel_id, state, updated_at DESC, effect_id DESC);

CREATE TABLE public.channel_acquisition_asset_current (
  channel_id BIGINT NOT NULL,
  asset_kind TEXT NOT NULL CHECK (asset_kind IN ('contact_way_qrcode', 'customer_acquisition_link')),
  effect_id BIGINT NOT NULL UNIQUE,
  asset_version BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (channel_id, asset_kind),
  FOREIGN KEY (effect_id, channel_id, asset_kind, asset_version)
    REFERENCES public.channel_acquisition_asset_bindings(effect_id, channel_id, asset_kind, asset_version) ON DELETE RESTRICT
);

CREATE TABLE public.channel_acquisition_asset_actor_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN ('publish', 'reconcile')),
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  key_digest TEXT NOT NULL CHECK (key_digest ~ '^sha256:[0-9a-f]{64}$'),
  payload_digest TEXT NOT NULL CHECK (payload_digest ~ '^sha256:[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('in_progress', 'completed')),
  result_effect_id BIGINT REFERENCES public.channel_acquisition_asset_bindings(effect_id) ON DELETE RESTRICT,
  replacement_effect_id BIGINT REFERENCES public.channel_acquisition_asset_bindings(effect_id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  UNIQUE(operation, actor_id, key_digest),
  CHECK ((state = 'in_progress' AND result_effect_id IS NULL AND replacement_effect_id IS NULL AND completed_at IS NULL) OR
         (state = 'completed' AND result_effect_id IS NOT NULL AND completed_at IS NOT NULL)),
  CHECK (replacement_effect_id IS NULL OR operation = 'reconcile')
);

CREATE TABLE public.channel_acquisition_asset_attempt_facts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  effect_id BIGINT NOT NULL,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BIGINT NOT NULL CHECK (fence > 0),
  state TEXT NOT NULL CHECK (state IN ('executed', 'final_failed', 'outcome_unknown')),
  receipt_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  receipt_digest TEXT NOT NULL CHECK (receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  provider_call_attempted BOOLEAN NOT NULL,
  real_external_call_executed BOOLEAN NOT NULL,
  completed_at TIMESTAMPTZ NOT NULL,
  UNIQUE(effect_id, generation, fence),
  UNIQUE(id, effect_id),
  FOREIGN KEY (effect_id, generation, fence)
    REFERENCES public.channel_acquisition_asset_bindings(effect_id, generation, fence) ON DELETE RESTRICT
);

CREATE TABLE public.channel_acquisition_asset_observed_results (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  attempt_fact_id BIGINT NOT NULL UNIQUE,
  effect_id BIGINT NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('executed', 'final_failed', 'outcome_unknown')),
  asset_reference_digest TEXT CHECK (asset_reference_digest IS NULL OR asset_reference_digest ~ '^sha256:[0-9a-f]{64}$'),
  observed_at TIMESTAMPTZ NOT NULL,
  CHECK ((outcome = 'executed' AND asset_reference_digest IS NOT NULL) OR (outcome <> 'executed' AND asset_reference_digest IS NULL)),
  FOREIGN KEY (attempt_fact_id, effect_id)
    REFERENCES public.channel_acquisition_asset_attempt_facts(id, effect_id) ON DELETE RESTRICT
);

CREATE TABLE public.channel_acquisition_asset_reconciliation_facts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  effect_id BIGINT NOT NULL,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BIGINT NOT NULL CHECK (fence > 0),
  receipt_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  receipt_digest TEXT NOT NULL CHECK (receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  evidence_digest TEXT NOT NULL CHECK (evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  resolution TEXT NOT NULL CHECK (resolution IN ('provider_applied', 'provider_not_applied')),
  reconciled_at TIMESTAMPTZ NOT NULL,
  UNIQUE(effect_id, generation, fence),
  FOREIGN KEY (effect_id, generation, fence)
    REFERENCES public.channel_acquisition_asset_bindings(effect_id, generation, fence) ON DELETE RESTRICT
);

-- Legacy channels.config is intentionally not converted into a publishable
-- asset.  Any future importer may archive only its digest as unverified input;
-- it must not fabricate an EER job, provider receipt, or external outcome.
CREATE TABLE public.channel_acquisition_legacy_archives (
  channel_id BIGINT PRIMARY KEY REFERENCES public.channels(id) ON DELETE RESTRICT,
  config_digest TEXT NOT NULL CHECK (config_digest ~ '^sha256:[0-9a-f]{64}$'),
  status TEXT NOT NULL DEFAULT 'legacy_unverified' CHECK (status = 'legacy_unverified'),
  archived_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_channel_acquisition_asset_facts_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'channel acquisition asset facts cannot be changed or deleted' USING ERRCODE = '55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER channel_acquisition_asset_attempt_facts_immutable BEFORE UPDATE OR DELETE ON public.channel_acquisition_asset_attempt_facts FOR EACH ROW EXECUTE FUNCTION public.aicrm_channel_acquisition_asset_facts_immutable();
CREATE TRIGGER channel_acquisition_asset_observed_results_immutable BEFORE UPDATE OR DELETE ON public.channel_acquisition_asset_observed_results FOR EACH ROW EXECUTE FUNCTION public.aicrm_channel_acquisition_asset_facts_immutable();
CREATE TRIGGER channel_acquisition_asset_reconciliation_facts_immutable BEFORE UPDATE OR DELETE ON public.channel_acquisition_asset_reconciliation_facts FOR EACH ROW EXECUTE FUNCTION public.aicrm_channel_acquisition_asset_facts_immutable();

-- +goose Down
LOCK TABLE public.channel_acquisition_asset_reconciliation_facts, public.channel_acquisition_asset_observed_results, public.channel_acquisition_asset_attempt_facts, public.channel_acquisition_asset_actor_receipts, public.channel_acquisition_asset_current, public.channel_acquisition_asset_bindings, public.channel_acquisition_legacy_archives IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.channel_acquisition_asset_reconciliation_facts) OR EXISTS (SELECT 1 FROM public.channel_acquisition_asset_observed_results) OR EXISTS (SELECT 1 FROM public.channel_acquisition_asset_attempt_facts) OR EXISTS (SELECT 1 FROM public.channel_acquisition_asset_actor_receipts) OR EXISTS (SELECT 1 FROM public.channel_acquisition_asset_current) OR EXISTS (SELECT 1 FROM public.channel_acquisition_asset_bindings) OR EXISTS (SELECT 1 FROM public.channel_acquisition_legacy_archives) OR EXISTS (SELECT 1 FROM public.external_effects WHERE kind = 'contact_acquisition_asset_publish') THEN
    RAISE EXCEPTION 'cannot roll back populated channel acquisition asset facts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync',
    'survey_webhook','audience_webhook','order_payment_prepay',
    'order_payment_capture','order_refund','group_ops_broadcast','product_external_push_test'
  ));
DROP TRIGGER channel_acquisition_asset_reconciliation_facts_immutable ON public.channel_acquisition_asset_reconciliation_facts;
DROP TRIGGER channel_acquisition_asset_observed_results_immutable ON public.channel_acquisition_asset_observed_results;
DROP TRIGGER channel_acquisition_asset_attempt_facts_immutable ON public.channel_acquisition_asset_attempt_facts;
DROP FUNCTION public.aicrm_channel_acquisition_asset_facts_immutable();
DROP TABLE public.channel_acquisition_legacy_archives;
DROP TABLE public.channel_acquisition_asset_reconciliation_facts;
DROP TABLE public.channel_acquisition_asset_observed_results;
DROP TABLE public.channel_acquisition_asset_attempt_facts;
DROP TABLE public.channel_acquisition_asset_actor_receipts;
DROP TABLE public.channel_acquisition_asset_current;
DROP TABLE public.channel_acquisition_asset_bindings;
