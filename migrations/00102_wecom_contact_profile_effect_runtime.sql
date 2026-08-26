-- +goose Up
CREATE TABLE public.wecom_contact_profile_effects (
  effect_id BIGINT PRIMARY KEY REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  legacy_receipt_id BIGINT NOT NULL CHECK (legacy_receipt_id > 0),
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  corp_id TEXT NOT NULL CHECK (length(corp_id) BETWEEN 1 AND 256),
  staff_userid TEXT NOT NULL CHECK (length(staff_userid) BETWEEN 1 AND 128),
  external_userid TEXT NOT NULL CHECK (length(external_userid) BETWEEN 1 AND 1024),
  remark TEXT NOT NULL CHECK (length(remark) BETWEEN 1 AND 400),
  description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 1500),
  idempotency_digest TEXT NOT NULL CHECK (idempotency_digest ~ '^sha256:[0-9a-f]{64}$'),
  envelope_fingerprint TEXT NOT NULL CHECK (envelope_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('accepted', 'queued', 'executed', 'outcome_unknown', 'final_failed', 'reconciled')),
  accept_receipt_id BIGINT NOT NULL REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  queue_receipt_id BIGINT REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  river_job_id BIGINT,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BIGINT NOT NULL DEFAULT 0 CHECK (fence >= 0),
  lease_expires_at TIMESTAMPTZ,
  attempt_receipt_id BIGINT REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  attempt_receipt_digest TEXT CHECK (attempt_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  attempt_completed_at TIMESTAMPTZ,
  provider_call_attempted BOOLEAN NOT NULL DEFAULT FALSE,
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE,
  reconcile_receipt_id BIGINT REFERENCES public.external_effect_receipts(id) ON DELETE RESTRICT,
  reconcile_receipt_digest TEXT CHECK (reconcile_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  reconcile_evidence_digest TEXT CHECK (reconcile_evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  reconcile_resolution TEXT CHECK (reconcile_resolution IN ('provider_applied', 'provider_not_applied')),
  reconciled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (actor_id, idempotency_digest),
  CHECK ((provider_call_attempted OR NOT real_external_call_executed) AND
         ((state IN ('accepted', 'queued') AND attempt_receipt_id IS NULL AND attempt_receipt_digest IS NULL AND attempt_completed_at IS NULL AND NOT provider_call_attempted AND NOT real_external_call_executed) OR
          (state IN ('executed', 'outcome_unknown', 'final_failed', 'reconciled') AND attempt_receipt_id IS NOT NULL AND attempt_receipt_digest IS NOT NULL AND attempt_completed_at IS NOT NULL))),
  CHECK ((state = 'reconciled') = (reconcile_receipt_id IS NOT NULL AND reconcile_receipt_digest IS NOT NULL AND reconcile_evidence_digest IS NOT NULL AND reconcile_resolution IS NOT NULL AND reconciled_at IS NOT NULL))
);

CREATE INDEX wecom_contact_profile_effects_corp_state_updated_idx
  ON public.wecom_contact_profile_effects (corp_id, state, updated_at DESC, effect_id DESC);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_wecom_contact_profile_effect_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'WeCom contact profile effect facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF ROW(NEW.effect_id, NEW.legacy_receipt_id, NEW.actor_id, NEW.corp_id, NEW.staff_userid,
         NEW.external_userid, NEW.remark, NEW.description, NEW.idempotency_digest,
         NEW.envelope_fingerprint, NEW.accept_receipt_id, NEW.created_at)
     IS DISTINCT FROM
     ROW(OLD.effect_id, OLD.legacy_receipt_id, OLD.actor_id, OLD.corp_id, OLD.staff_userid,
         OLD.external_userid, OLD.remark, OLD.description, OLD.idempotency_digest,
         OLD.envelope_fingerprint, OLD.accept_receipt_id, OLD.created_at) THEN
    RAISE EXCEPTION 'WeCom contact profile effect command facts are immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state = 'accepted' AND NEW.state = 'queued'
     AND NEW.generation > OLD.generation AND NEW.fence = 0 THEN
    RETURN NEW;
  END IF;
  IF OLD.state = 'queued' AND NEW.state = 'queued'
     AND NEW.generation = OLD.generation AND NEW.fence > OLD.fence
     AND ROW(NEW.queue_receipt_id, NEW.river_job_id, NEW.attempt_receipt_id, NEW.attempt_receipt_digest,
             NEW.attempt_completed_at, NEW.provider_call_attempted, NEW.real_external_call_executed,
             NEW.reconcile_receipt_id, NEW.reconcile_receipt_digest, NEW.reconcile_evidence_digest,
             NEW.reconcile_resolution, NEW.reconciled_at)
         IS NOT DISTINCT FROM
         ROW(OLD.queue_receipt_id, OLD.river_job_id, OLD.attempt_receipt_id, OLD.attempt_receipt_digest,
             OLD.attempt_completed_at, OLD.provider_call_attempted, OLD.real_external_call_executed,
             OLD.reconcile_receipt_id, OLD.reconcile_receipt_digest, OLD.reconcile_evidence_digest,
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
     AND NEW.attempt_receipt_id = OLD.attempt_receipt_id AND NEW.attempt_receipt_digest = OLD.attempt_receipt_digest
     AND NEW.attempt_completed_at = OLD.attempt_completed_at
     AND NEW.provider_call_attempted = OLD.provider_call_attempted
     AND NEW.real_external_call_executed = OLD.real_external_call_executed THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid WeCom contact profile effect transition' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER wecom_contact_profile_effects_transition
BEFORE UPDATE OR DELETE ON public.wecom_contact_profile_effects
FOR EACH ROW EXECUTE FUNCTION public.aicrm_wecom_contact_profile_effect_transition_valid();

-- +goose Down
LOCK TABLE public.wecom_contact_profile_effects IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.wecom_contact_profile_effects LIMIT 1) THEN
    RAISE EXCEPTION 'cannot roll back populated WeCom contact profile effect runtime' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.wecom_contact_profile_effects;
DROP FUNCTION public.aicrm_wecom_contact_profile_effect_transition_valid();
