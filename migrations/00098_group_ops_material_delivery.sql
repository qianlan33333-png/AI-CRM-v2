-- +goose Up
-- Group Ops material delivery is a two-stage business flow. Stable source
-- facts are captured first; provider-ready media IDs are frozen only close to
-- dispatch. Media upload remains a separate external effect from broadcast.
ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_owner_check,
  ADD CONSTRAINT external_effects_owner_check CHECK (owner IN (
    'campaign','contact','outbound','wecom','survey','audience','order','group_ops','product','media'
  )),
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'contact_acquisition_asset_publish','outbound_message','outbound_media',
    'wecom_tag_sync','wecom_profile_sync','survey_webhook','audience_webhook',
    'order_payment_prepay','order_payment_capture','order_refund',
    'group_ops_broadcast','product_external_push_test','media_wecom_upload'
  ));

ALTER TABLE public.group_ops_plan_nodes
  ADD COLUMN material_plan JSONB NOT NULL DEFAULT '{"references":[]}'::jsonb,
  ADD CONSTRAINT group_ops_plan_nodes_material_plan CHECK (
    jsonb_typeof(material_plan) = 'object'
    AND material_plan ? 'references'
    AND jsonb_typeof(material_plan->'references') = 'array'
    AND jsonb_array_length(material_plan->'references') BETWEEN 0 AND 9
  );

CREATE TABLE public.group_ops_protocol_replays (
  client_id TEXT NOT NULL CHECK (client_id = 'aicrm-webhook-group-ops'),
  resource_reference TEXT NOT NULL,
  event_id TEXT NOT NULL,
  event_id_digest BYTEA NOT NULL CHECK (octet_length(event_id_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (client_id, event_id_digest),
  CONSTRAINT group_ops_protocol_replays_resource CHECK (
    resource_reference ~ '^[A-Za-z0-9._:-]{1,128}$'
    AND position('://' IN resource_reference) = 0
  ),
  CONSTRAINT group_ops_protocol_replays_event CHECK (
    octet_length(event_id) BETWEEN 16 AND 256 AND btrim(event_id) = event_id
  )
);

CREATE TABLE public.media_wecom_upload_preparations (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('image','attachment')),
  source_id BIGINT NOT NULL CHECK (source_id > 0),
  source_digest TEXT NOT NULL CHECK (source_digest ~ '^sha256:[0-9a-f]{64}$'),
  provider_scope_digest TEXT NOT NULL CHECK (provider_scope_digest ~ '^sha256:[0-9a-f]{64}$'),
  upload_kind TEXT NOT NULL CHECK (upload_kind IN ('image','file')),
  external_effect_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  state TEXT NOT NULL DEFAULT 'preparing' CHECK (state IN (
    'preparing','ready','retryable_failed','outcome_unknown','final_failed','expired'
  )),
  provider_media_id TEXT,
  provider_created_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  provider_receipt_digest TEXT CHECK (
    provider_receipt_digest IS NULL OR provider_receipt_digest ~ '^sha256:[0-9a-f]{64}$'
  ),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_wecom_upload_preparations_media_id CHECK (
    provider_media_id IS NULL OR provider_media_id ~ '^[^[:space:]]{1,1024}$'
  ),
  CONSTRAINT media_wecom_upload_preparations_outcome CHECK (
    (state IN ('preparing','retryable_failed','outcome_unknown','final_failed')
      AND provider_media_id IS NULL AND provider_created_at IS NULL
      AND expires_at IS NULL AND provider_receipt_digest IS NULL)
    OR (state IN ('ready','expired') AND provider_media_id IS NOT NULL
      AND provider_created_at IS NOT NULL AND expires_at > provider_created_at
      AND provider_receipt_digest IS NOT NULL)
  ),
  CONSTRAINT media_wecom_upload_preparations_timestamps CHECK (updated_at >= created_at),
  UNIQUE (source_kind, source_id, source_digest, provider_scope_digest, upload_kind, external_effect_id)
);
CREATE INDEX media_wecom_upload_preparations_source_idx
  ON public.media_wecom_upload_preparations (
    source_kind, source_id, source_digest, provider_scope_digest, upload_kind, created_at DESC
  );

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_wecom_upload_effect_binding()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.external_effects
    WHERE id = NEW.external_effect_id AND owner = 'media' AND kind = 'media_wecom_upload'
  ) THEN
    RAISE EXCEPTION 'media upload preparation must bind a media upload effect' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_wecom_upload_preparations_effect_binding
BEFORE INSERT OR UPDATE ON public.media_wecom_upload_preparations
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_wecom_upload_effect_binding();

CREATE TABLE public.media_wecom_upload_receipts (
  external_effect_id BIGINT PRIMARY KEY REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  preparation_id BIGINT NOT NULL UNIQUE REFERENCES public.media_wecom_upload_preparations(id) ON DELETE RESTRICT,
  provider_media_id TEXT NOT NULL CHECK (provider_media_id ~ '^[^[:space:]]{1,1024}$'),
  provider_created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  receipt_digest TEXT NOT NULL CHECK (receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_wecom_upload_receipts_expiry CHECK (expires_at > provider_created_at)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_wecom_upload_receipt_binding()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.media_wecom_upload_preparations p
    WHERE p.id = NEW.preparation_id AND p.external_effect_id = NEW.external_effect_id
      AND p.state = 'ready' AND p.provider_media_id = NEW.provider_media_id
      AND p.provider_created_at = NEW.provider_created_at AND p.expires_at = NEW.expires_at
      AND p.provider_receipt_digest = NEW.receipt_digest
  ) THEN
    RAISE EXCEPTION 'media upload receipt does not match its ready preparation' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER media_wecom_upload_receipts_binding
AFTER INSERT ON public.media_wecom_upload_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_wecom_upload_receipt_binding();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_wecom_upload_ready_receipt_required()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NEW.state IN ('ready','expired') AND NOT EXISTS (
    SELECT 1 FROM public.media_wecom_upload_receipts r
    WHERE r.preparation_id = NEW.id AND r.external_effect_id = NEW.external_effect_id
      AND r.provider_media_id = NEW.provider_media_id
      AND r.provider_created_at = NEW.provider_created_at AND r.expires_at = NEW.expires_at
      AND r.receipt_digest = NEW.provider_receipt_digest
  ) THEN
    RAISE EXCEPTION 'ready media upload preparation requires its exact receipt' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER media_wecom_upload_preparations_ready_receipt
AFTER INSERT OR UPDATE ON public.media_wecom_upload_preparations DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_wecom_upload_ready_receipt_required();

CREATE TABLE public.group_ops_execution_intents (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES public.group_ops_runs(id) ON DELETE RESTRICT,
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_plans(id) ON DELETE RESTRICT,
  node_id BIGINT NOT NULL REFERENCES public.group_ops_plan_nodes(id) ON DELETE RESTRICT,
  plan_revision BIGINT NOT NULL CHECK (plan_revision > 0),
  node_position INTEGER NOT NULL CHECK (node_position > 0),
  target_reference TEXT NOT NULL,
  target_digest TEXT NOT NULL CHECK (target_digest ~ '^sha256:[0-9a-f]{64}$'),
  sender_userid_snapshot TEXT NOT NULL CHECK (sender_userid_snapshot ~ '^[^[:space:]]{1,128}$'),
  scheduled_for TIMESTAMPTZ NOT NULL,
  content_snapshot JSONB NOT NULL CHECK (jsonb_typeof(content_snapshot) = 'object'),
  content_digest TEXT NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
  material_source_snapshot JSONB NOT NULL,
  material_source_digest TEXT NOT NULL CHECK (material_source_digest ~ '^sha256:[0-9a-f]{64}$'),
  execution_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(execution_key_digest) = 32),
  state TEXT NOT NULL DEFAULT 'material_pending' CHECK (state IN (
    'material_pending','ready_to_accept','accepted','final_failed'
  )),
  continuation_job_id BIGINT NOT NULL CHECK (continuation_job_id > 0),
  continuation_generation BIGINT NOT NULL CHECK (continuation_generation > 0),
  execution_id BIGINT UNIQUE REFERENCES public.group_ops_executions(id) ON DELETE RESTRICT,
  failure_code TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT group_ops_execution_intents_target CHECK (
    target_reference ~ '^[A-Za-z0-9._:-]{1,128}$'
    AND position('://' IN target_reference) = 0
  ),
  CONSTRAINT group_ops_execution_intents_material CHECK (
    jsonb_typeof(material_source_snapshot) = 'object'
    AND material_source_snapshot ? 'references'
    AND jsonb_typeof(material_source_snapshot->'references') = 'array'
    AND jsonb_array_length(material_source_snapshot->'references') BETWEEN 1 AND 9
  ),
  CONSTRAINT group_ops_execution_intents_outcome CHECK (
    (state IN ('material_pending','ready_to_accept') AND execution_id IS NULL AND failure_code IS NULL)
    OR (state = 'accepted' AND execution_id IS NOT NULL AND failure_code IS NULL)
    OR (state = 'final_failed' AND execution_id IS NULL AND failure_code ~ '^[a-z][a-z0-9_]{0,63}$')
  ),
  CONSTRAINT group_ops_execution_intents_timestamps CHECK (updated_at >= created_at),
  UNIQUE (run_id, node_id, target_reference)
);
CREATE INDEX group_ops_execution_intents_state_idx
  ON public.group_ops_execution_intents (state, scheduled_for, id);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_group_ops_material_append_only()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'group ops material facts are append only' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER group_ops_protocol_replays_append_only
BEFORE UPDATE OR DELETE ON public.group_ops_protocol_replays
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_material_append_only();
CREATE TRIGGER media_wecom_upload_receipts_append_only
BEFORE UPDATE OR DELETE ON public.media_wecom_upload_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_material_append_only();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_wecom_upload_preparation_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'media upload preparations cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.source_kind IS DISTINCT FROM OLD.source_kind
     OR NEW.source_id IS DISTINCT FROM OLD.source_id OR NEW.source_digest IS DISTINCT FROM OLD.source_digest
     OR NEW.provider_scope_digest IS DISTINCT FROM OLD.provider_scope_digest
     OR NEW.upload_kind IS DISTINCT FROM OLD.upload_kind OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR (OLD.state IN ('ready','expired') AND (
       NEW.provider_media_id IS DISTINCT FROM OLD.provider_media_id
       OR NEW.provider_created_at IS DISTINCT FROM OLD.provider_created_at
       OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
       OR NEW.provider_receipt_digest IS DISTINCT FROM OLD.provider_receipt_digest
     ))
     OR (OLD.state = 'preparing' AND NEW.state NOT IN ('ready','retryable_failed','outcome_unknown','final_failed'))
     OR (OLD.state = 'retryable_failed' AND NEW.state <> 'preparing')
     OR (OLD.state = 'ready' AND NEW.state <> 'expired')
     OR OLD.state IN ('outcome_unknown','final_failed','expired') THEN
    RAISE EXCEPTION 'invalid media upload preparation transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_wecom_upload_preparations_guard
BEFORE UPDATE OR DELETE ON public.media_wecom_upload_preparations
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_wecom_upload_preparation_guard();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_group_ops_execution_intent_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'group ops execution intents cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.run_id IS DISTINCT FROM OLD.run_id
     OR NEW.plan_id IS DISTINCT FROM OLD.plan_id OR NEW.node_id IS DISTINCT FROM OLD.node_id
     OR NEW.plan_revision IS DISTINCT FROM OLD.plan_revision OR NEW.node_position IS DISTINCT FROM OLD.node_position
     OR NEW.target_reference IS DISTINCT FROM OLD.target_reference OR NEW.target_digest IS DISTINCT FROM OLD.target_digest
     OR NEW.sender_userid_snapshot IS DISTINCT FROM OLD.sender_userid_snapshot
     OR NEW.scheduled_for IS DISTINCT FROM OLD.scheduled_for
     OR NEW.content_snapshot IS DISTINCT FROM OLD.content_snapshot OR NEW.content_digest IS DISTINCT FROM OLD.content_digest
     OR NEW.material_source_snapshot IS DISTINCT FROM OLD.material_source_snapshot
     OR NEW.material_source_digest IS DISTINCT FROM OLD.material_source_digest
     OR NEW.execution_key_digest IS DISTINCT FROM OLD.execution_key_digest
     OR NEW.continuation_job_id IS DISTINCT FROM OLD.continuation_job_id
     OR NEW.continuation_generation IS DISTINCT FROM OLD.continuation_generation
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR (OLD.state = 'material_pending' AND NEW.state NOT IN ('ready_to_accept','final_failed'))
     OR (OLD.state = 'ready_to_accept' AND NEW.state NOT IN ('accepted','final_failed'))
     OR OLD.state IN ('accepted','final_failed') THEN
    RAISE EXCEPTION 'invalid group ops execution intent transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER group_ops_execution_intents_guard
BEFORE UPDATE OR DELETE ON public.group_ops_execution_intents
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_execution_intent_guard();

-- +goose Down
LOCK TABLE public.group_ops_plan_nodes, public.group_ops_execution_intents,
  public.group_ops_protocol_replays, public.media_wecom_upload_preparations,
  public.media_wecom_upload_receipts, public.external_effects IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.group_ops_plan_nodes WHERE material_plan <> '{"references":[]}'::jsonb)
     OR EXISTS (SELECT 1 FROM public.group_ops_execution_intents)
     OR EXISTS (SELECT 1 FROM public.group_ops_protocol_replays)
     OR EXISTS (SELECT 1 FROM public.media_wecom_upload_preparations)
     OR EXISTS (SELECT 1 FROM public.media_wecom_upload_receipts)
     OR EXISTS (SELECT 1 FROM public.external_effects WHERE owner = 'media' OR kind = 'media_wecom_upload') THEN
    RAISE EXCEPTION 'cannot roll back populated group ops material delivery facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER group_ops_execution_intents_guard ON public.group_ops_execution_intents;
DROP TRIGGER media_wecom_upload_preparations_guard ON public.media_wecom_upload_preparations;
DROP TRIGGER media_wecom_upload_receipts_append_only ON public.media_wecom_upload_receipts;
DROP TRIGGER media_wecom_upload_receipts_binding ON public.media_wecom_upload_receipts;
DROP TRIGGER media_wecom_upload_preparations_ready_receipt ON public.media_wecom_upload_preparations;
DROP TRIGGER media_wecom_upload_preparations_effect_binding ON public.media_wecom_upload_preparations;
DROP TRIGGER group_ops_protocol_replays_append_only ON public.group_ops_protocol_replays;
DROP FUNCTION public.aicrm_group_ops_execution_intent_guard();
DROP FUNCTION public.aicrm_media_wecom_upload_preparation_guard();
DROP FUNCTION public.aicrm_media_wecom_upload_ready_receipt_required();
DROP FUNCTION public.aicrm_media_wecom_upload_receipt_binding();
DROP FUNCTION public.aicrm_media_wecom_upload_effect_binding();
DROP FUNCTION public.aicrm_group_ops_material_append_only();
DROP TABLE public.group_ops_execution_intents;
DROP TABLE public.media_wecom_upload_receipts;
DROP TABLE public.media_wecom_upload_preparations;
DROP TABLE public.group_ops_protocol_replays;
ALTER TABLE public.group_ops_plan_nodes DROP CONSTRAINT group_ops_plan_nodes_material_plan;
ALTER TABLE public.group_ops_plan_nodes DROP COLUMN material_plan;
ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'contact_acquisition_asset_publish','outbound_message','outbound_media',
    'wecom_tag_sync','wecom_profile_sync','survey_webhook','audience_webhook',
    'order_payment_prepay','order_payment_capture','order_refund',
    'group_ops_broadcast','product_external_push_test'
  )),
  DROP CONSTRAINT external_effects_owner_check,
  ADD CONSTRAINT external_effects_owner_check CHECK (owner IN (
    'campaign','contact','outbound','wecom','survey','audience','order','group_ops','product'
  ));
