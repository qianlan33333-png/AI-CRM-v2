-- +goose Up
-- This is the typed WeCom binding for the shared external-effects runtime.
-- The 00038 queues remain legacy acceptance facts and are deliberately not
-- copied or replayed by this migration.
CREATE TABLE public.wecom_tag_effects (
  effect_id BIGINT PRIMARY KEY REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  -- operation identifies which Contact-owned 00038 receipt type was bound;
  -- WeCom does not read or foreign-key across that domain boundary.
  legacy_receipt_id BIGINT NOT NULL CHECK (legacy_receipt_id > 0),
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  corp_id TEXT NOT NULL CHECK (btrim(corp_id) = corp_id AND char_length(corp_id) BETWEEN 1 AND 256),
  operation TEXT NOT NULL CHECK (operation IN ('catalog_sync', 'mark', 'unmark')),
  sync_trigger TEXT NOT NULL DEFAULT '' CHECK (sync_trigger IN ('', 'manual', 'due')),
  external_userid TEXT NOT NULL DEFAULT '' CHECK (btrim(external_userid) = external_userid AND char_length(external_userid) <= 1024),
  provider_tag_ids TEXT[] NOT NULL DEFAULT '{}',
  idempotency_digest TEXT NOT NULL CHECK (idempotency_digest ~ '^sha256:[0-9a-f]{64}$'),
  envelope_fingerprint TEXT NOT NULL UNIQUE CHECK (envelope_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('accepted', 'queued', 'executed', 'outcome_unknown', 'reconciled', 'final_failed')),
  accept_receipt_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  queue_receipt_id BIGINT UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  attempt_receipt_id BIGINT UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  reconcile_receipt_id BIGINT UNIQUE REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  river_job_id BIGINT,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BIGINT NOT NULL DEFAULT 0 CHECK (fence >= 0),
  lease_expires_at TIMESTAMPTZ,
  attempt_receipt_digest TEXT CHECK (attempt_receipt_digest IS NULL OR attempt_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  attempt_completed_at TIMESTAMPTZ,
  reconcile_receipt_digest TEXT CHECK (reconcile_receipt_digest IS NULL OR reconcile_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  reconcile_evidence_digest TEXT CHECK (reconcile_evidence_digest IS NULL OR reconcile_evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  reconcile_resolution TEXT CHECK (reconcile_resolution IS NULL OR reconcile_resolution IN ('provider_applied', 'provider_not_applied')),
  reconciled_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (actor_id, idempotency_digest),
  CONSTRAINT wecom_tag_effect_command_shape CHECK (
    (operation = 'catalog_sync' AND sync_trigger IN ('manual', 'due') AND external_userid = '' AND cardinality(provider_tag_ids) = 0) OR
    (operation IN ('mark', 'unmark') AND sync_trigger = '' AND external_userid <> '' AND cardinality(provider_tag_ids) BETWEEN 1 AND 100)
  ),
  CONSTRAINT wecom_tag_effect_lease_shape CHECK (
    (fence = 0 AND lease_expires_at IS NULL) OR (fence > 0 AND lease_expires_at IS NOT NULL)
  ),
  CONSTRAINT wecom_tag_effect_state_shape CHECK (
    (state = 'accepted' AND queue_receipt_id IS NULL AND river_job_id IS NULL AND fence = 0
      AND attempt_receipt_id IS NULL AND attempt_receipt_digest IS NULL AND attempt_completed_at IS NULL
      AND reconcile_receipt_id IS NULL AND reconcile_receipt_digest IS NULL AND reconcile_evidence_digest IS NULL
      AND reconcile_resolution IS NULL AND reconciled_at IS NULL) OR
    (state = 'queued' AND queue_receipt_id IS NOT NULL AND river_job_id > 0
      AND attempt_receipt_id IS NULL AND attempt_receipt_digest IS NULL AND attempt_completed_at IS NULL
      AND reconcile_receipt_id IS NULL AND reconcile_receipt_digest IS NULL AND reconcile_evidence_digest IS NULL
      AND reconcile_resolution IS NULL AND reconciled_at IS NULL) OR
    (state IN ('executed', 'outcome_unknown', 'final_failed') AND queue_receipt_id IS NOT NULL AND river_job_id > 0
      AND fence > 0 AND attempt_receipt_id IS NOT NULL AND attempt_receipt_digest IS NOT NULL AND attempt_completed_at IS NOT NULL
      AND reconcile_receipt_id IS NULL AND reconcile_receipt_digest IS NULL AND reconcile_evidence_digest IS NULL
      AND reconcile_resolution IS NULL AND reconciled_at IS NULL) OR
    (state = 'reconciled' AND queue_receipt_id IS NOT NULL AND river_job_id > 0
      AND fence > 0 AND attempt_receipt_id IS NOT NULL AND attempt_receipt_digest IS NOT NULL AND attempt_completed_at IS NOT NULL
      AND reconcile_receipt_id IS NOT NULL AND reconcile_receipt_digest IS NOT NULL AND reconcile_evidence_digest IS NOT NULL
      AND reconcile_resolution IS NOT NULL AND reconciled_at IS NOT NULL)
  )
);
CREATE INDEX wecom_tag_effects_corp_state_updated_idx
  ON public.wecom_tag_effects (corp_id, state, updated_at DESC, effect_id DESC);

CREATE TABLE public.wecom_tag_catalog_snapshots (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  effect_id BIGINT NOT NULL UNIQUE REFERENCES public.wecom_tag_effects(effect_id) ON DELETE RESTRICT,
  corp_id TEXT NOT NULL CHECK (btrim(corp_id) = corp_id AND char_length(corp_id) BETWEEN 1 AND 256),
  receipt_digest TEXT NOT NULL CHECK (receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  observed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX wecom_tag_catalog_snapshots_corp_observed_idx
  ON public.wecom_tag_catalog_snapshots (corp_id, observed_at DESC, id DESC);

CREATE TABLE public.wecom_tag_catalog_groups (
  snapshot_id BIGINT NOT NULL REFERENCES public.wecom_tag_catalog_snapshots(id) ON DELETE RESTRICT,
  provider_group_id TEXT NOT NULL CHECK (btrim(provider_group_id) = provider_group_id AND char_length(provider_group_id) BETWEEN 1 AND 128),
  name TEXT NOT NULL CHECK (btrim(name) = name AND char_length(name) BETWEEN 1 AND 256),
  provider_order INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, provider_group_id)
);

CREATE TABLE public.wecom_tag_catalog_tags (
  snapshot_id BIGINT NOT NULL,
  provider_tag_id TEXT NOT NULL CHECK (btrim(provider_tag_id) = provider_tag_id AND char_length(provider_tag_id) BETWEEN 1 AND 128),
  provider_group_id TEXT NOT NULL CHECK (btrim(provider_group_id) = provider_group_id AND char_length(provider_group_id) BETWEEN 1 AND 128),
  name TEXT NOT NULL CHECK (btrim(name) = name AND char_length(name) BETWEEN 1 AND 256),
  provider_order INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, provider_tag_id),
  FOREIGN KEY (snapshot_id, provider_group_id)
    REFERENCES public.wecom_tag_catalog_groups(snapshot_id, provider_group_id) ON DELETE RESTRICT
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_wecom_tag_effect_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'WeCom tag effect facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF ROW(NEW.effect_id, NEW.legacy_receipt_id, NEW.actor_id, NEW.corp_id, NEW.operation,
         NEW.sync_trigger, NEW.external_userid, NEW.provider_tag_ids, NEW.idempotency_digest,
         NEW.envelope_fingerprint, NEW.accept_receipt_id, NEW.created_at)
     IS DISTINCT FROM
     ROW(OLD.effect_id, OLD.legacy_receipt_id, OLD.actor_id, OLD.corp_id, OLD.operation,
         OLD.sync_trigger, OLD.external_userid, OLD.provider_tag_ids, OLD.idempotency_digest,
         OLD.envelope_fingerprint, OLD.accept_receipt_id, OLD.created_at) THEN
    RAISE EXCEPTION 'WeCom tag effect command facts are immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state = 'accepted' AND NEW.state = 'queued'
     AND NEW.generation > OLD.generation AND NEW.fence = 0 THEN
    RETURN NEW;
  END IF;
  IF OLD.state = 'queued' AND NEW.state = 'queued'
     AND NEW.generation = OLD.generation AND NEW.fence > OLD.fence
     AND ROW(NEW.queue_receipt_id, NEW.river_job_id, NEW.attempt_receipt_id,
             NEW.attempt_receipt_digest, NEW.attempt_completed_at, NEW.reconcile_receipt_id,
             NEW.reconcile_receipt_digest, NEW.reconcile_evidence_digest,
             NEW.reconcile_resolution, NEW.reconciled_at)
         IS NOT DISTINCT FROM
         ROW(OLD.queue_receipt_id, OLD.river_job_id, OLD.attempt_receipt_id,
             OLD.attempt_receipt_digest, OLD.attempt_completed_at, OLD.reconcile_receipt_id,
             OLD.reconcile_receipt_digest, OLD.reconcile_evidence_digest,
             OLD.reconcile_resolution, OLD.reconciled_at) THEN
    RETURN NEW;
  END IF;
  IF OLD.state = 'queued' AND NEW.state IN ('executed', 'outcome_unknown', 'final_failed')
     AND NEW.generation = OLD.generation AND NEW.fence = OLD.fence
     AND NEW.lease_expires_at IS NOT DISTINCT FROM OLD.lease_expires_at
     AND NEW.queue_receipt_id = OLD.queue_receipt_id AND NEW.river_job_id = OLD.river_job_id THEN
    RETURN NEW;
  END IF;
  IF OLD.state = 'outcome_unknown' AND NEW.state = 'reconciled'
     AND NEW.generation = OLD.generation AND NEW.fence = OLD.fence
     AND NEW.lease_expires_at IS NOT DISTINCT FROM OLD.lease_expires_at
     AND NEW.queue_receipt_id = OLD.queue_receipt_id AND NEW.river_job_id = OLD.river_job_id
     AND NEW.attempt_receipt_id = OLD.attempt_receipt_id
     AND NEW.attempt_receipt_digest = OLD.attempt_receipt_digest
     AND NEW.attempt_completed_at = OLD.attempt_completed_at THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid WeCom tag effect transition' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER wecom_tag_effects_transition
BEFORE UPDATE OR DELETE ON public.wecom_tag_effects
FOR EACH ROW EXECUTE FUNCTION public.aicrm_wecom_tag_effect_transition_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_wecom_tag_catalog_reject_mutation()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'WeCom tag catalog snapshots are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER wecom_tag_catalog_snapshots_immutable
BEFORE UPDATE OR DELETE ON public.wecom_tag_catalog_snapshots
FOR EACH ROW EXECUTE FUNCTION public.aicrm_wecom_tag_catalog_reject_mutation();
CREATE TRIGGER wecom_tag_catalog_groups_immutable
BEFORE UPDATE OR DELETE ON public.wecom_tag_catalog_groups
FOR EACH ROW EXECUTE FUNCTION public.aicrm_wecom_tag_catalog_reject_mutation();
CREATE TRIGGER wecom_tag_catalog_tags_immutable
BEFORE UPDATE OR DELETE ON public.wecom_tag_catalog_tags
FOR EACH ROW EXECUTE FUNCTION public.aicrm_wecom_tag_catalog_reject_mutation();

-- +goose Down
LOCK TABLE public.wecom_tag_effects,
  public.wecom_tag_catalog_snapshots,
  public.wecom_tag_catalog_groups,
  public.wecom_tag_catalog_tags
IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.wecom_tag_effects LIMIT 1)
     OR EXISTS (SELECT 1 FROM public.wecom_tag_catalog_snapshots LIMIT 1)
     OR EXISTS (SELECT 1 FROM public.wecom_tag_catalog_groups LIMIT 1)
     OR EXISTS (SELECT 1 FROM public.wecom_tag_catalog_tags LIMIT 1) THEN
    RAISE EXCEPTION 'cannot roll back populated WeCom tag effect runtime' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.wecom_tag_catalog_tags;
DROP TABLE public.wecom_tag_catalog_groups;
DROP TABLE public.wecom_tag_catalog_snapshots;
DROP TABLE public.wecom_tag_effects;
DROP FUNCTION public.aicrm_wecom_tag_catalog_reject_mutation();
DROP FUNCTION public.aicrm_wecom_tag_effect_transition_valid();
