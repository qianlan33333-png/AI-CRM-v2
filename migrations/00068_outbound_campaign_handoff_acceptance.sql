-- +goose Up
-- Outbound-owned local acceptance of an immutable approved Campaign handoff.
-- It never creates outbound_tasks, a send job, or a Provider execution fact.
CREATE TABLE public.outbound_campaign_handoffs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  campaign_code TEXT NOT NULL CHECK (campaign_code ~ '^[A-Za-z0-9._-]{1,96}$'),
  plan_id TEXT NOT NULL CHECK (plan_id ~ '^ctp_[0-9a-f]{64}$'),
  review_version BIGINT NOT NULL CHECK (review_version >= 3),
  source_digest BYTEA NOT NULL CHECK (octet_length(source_digest) = 32),
  target_digest BYTEA NOT NULL CHECK (octet_length(target_digest) = 32),
  content_digest BYTEA NOT NULL CHECK (octet_length(content_digest) = 32),
  target_count INTEGER NOT NULL CHECK (target_count BETWEEN 1 AND 1000),
  step_count INTEGER NOT NULL CHECK (step_count BETWEEN 1 AND 100),
  status TEXT NOT NULL CHECK (status = 'held'),
  accepted_by_actor_id BIGINT NOT NULL CHECK (accepted_by_actor_id > 0),
  accepted_at TIMESTAMPTZ NOT NULL,
  local_only BOOLEAN NOT NULL DEFAULT TRUE CHECK (local_only),
  provider_execution_eligible BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT provider_execution_eligible),
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT real_external_call_executed),
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT delivery_proven),
  UNIQUE (campaign_code, plan_id),
  UNIQUE (plan_id)
);

CREATE TABLE public.outbound_campaign_handoff_steps (
  handoff_id BIGINT NOT NULL REFERENCES public.outbound_campaign_handoffs(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  step_index INTEGER NOT NULL CHECK (step_index BETWEEN 1 AND 100),
  delay_minutes INTEGER NOT NULL CHECK (delay_minutes >= 0),
  content TEXT NOT NULL CHECK (content <> ''),
  PRIMARY KEY (handoff_id, step_index)
);

CREATE TABLE public.outbound_campaign_handoff_customer_tasks (
  handoff_id BIGINT NOT NULL REFERENCES public.outbound_campaign_handoffs(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  state TEXT NOT NULL CHECK (state IN ('held', 'blocked', 'pending')),
  eligibility TEXT NOT NULL CHECK (eligibility IN ('not_evaluated', 'eligible', 'inactive', 'contact_policy')),
  outbound_task_id BIGINT REFERENCES public.outbound_tasks(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT outbound_campaign_handoff_customer_tasks_shape CHECK (
    (state = 'held' AND eligibility = 'not_evaluated' AND outbound_task_id IS NULL)
    OR (state = 'blocked' AND eligibility IN ('inactive', 'contact_policy') AND outbound_task_id IS NULL)
    OR (state = 'pending' AND eligibility = 'eligible' AND outbound_task_id IS NOT NULL)
  ),
  PRIMARY KEY (handoff_id, customer_id),
  UNIQUE (outbound_task_id)
);

CREATE TABLE public.outbound_campaign_handoff_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  campaign_code TEXT NOT NULL CHECK (campaign_code ~ '^[A-Za-z0-9._-]{1,96}$'),
  plan_id TEXT NOT NULL CHECK (plan_id ~ '^ctp_[0-9a-f]{64}$'),
  handoff_id BIGINT REFERENCES public.outbound_campaign_handoffs(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  event_id BIGINT REFERENCES public.event_log(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved', 'completed')),
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  result_snapshot JSONB,
  CONSTRAINT outbound_campaign_handoff_receipts_shape CHECK (
    (state = 'reserved' AND handoff_id IS NULL AND event_id IS NULL AND completed_at IS NULL AND result_snapshot IS NULL)
    OR (state = 'completed' AND handoff_id IS NOT NULL AND event_id IS NOT NULL AND completed_at IS NOT NULL AND result_snapshot IS NOT NULL)
  ),
  UNIQUE (actor_id, key_digest),
  UNIQUE (handoff_id),
  UNIQUE (event_id)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_outbound_campaign_handoff_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'outbound campaign handoff acceptance facts are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER outbound_campaign_handoffs_immutable
BEFORE UPDATE OR DELETE ON public.outbound_campaign_handoffs
FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_immutable();
CREATE TRIGGER outbound_campaign_handoff_steps_immutable
BEFORE UPDATE OR DELETE ON public.outbound_campaign_handoff_steps
FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_immutable();
CREATE TRIGGER outbound_campaign_handoff_customer_tasks_immutable
BEFORE UPDATE OR DELETE ON public.outbound_campaign_handoff_customer_tasks
FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_immutable();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_outbound_campaign_handoff_child_insert_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.outbound_campaign_handoff_receipts WHERE handoff_id = NEW.handoff_id AND state = 'completed') THEN
    RAISE EXCEPTION 'cannot append to a completed outbound campaign handoff snapshot' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER outbound_campaign_handoff_steps_insert_valid
BEFORE INSERT ON public.outbound_campaign_handoff_steps
FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_child_insert_valid();
CREATE TRIGGER outbound_campaign_handoff_customer_tasks_insert_valid
BEFORE INSERT ON public.outbound_campaign_handoff_customer_tasks
FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_child_insert_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_outbound_campaign_handoff_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed outbound campaign handoff receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.campaign_code IS DISTINCT FROM OLD.campaign_code OR NEW.plan_id IS DISTINCT FROM OLD.plan_id
     OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.handoff_id IS NULL OR NEW.event_id IS NULL
     OR NEW.completed_at IS NULL OR NEW.result_snapshot IS NULL THEN
    RAISE EXCEPTION 'invalid outbound campaign handoff receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER outbound_campaign_handoff_receipts_transition
BEFORE UPDATE OR DELETE ON public.outbound_campaign_handoff_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_receipt_transition_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_outbound_campaign_handoff_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  stored public.outbound_campaign_handoffs%ROWTYPE;
  receipt public.outbound_campaign_handoff_receipts%ROWTYPE;
  actual_steps INTEGER;
  actual_links INTEGER;
BEGIN
  SELECT * INTO stored FROM public.outbound_campaign_handoffs WHERE id = NEW.id;
  SELECT * INTO receipt FROM public.outbound_campaign_handoff_receipts WHERE handoff_id = NEW.id;
  SELECT count(*) INTO actual_steps FROM public.outbound_campaign_handoff_steps WHERE handoff_id = NEW.id;
  SELECT count(*) INTO actual_links FROM public.outbound_campaign_handoff_customer_tasks WHERE handoff_id = NEW.id;
  IF stored.id IS NULL OR receipt.id IS NULL OR receipt.state <> 'completed' OR actual_steps <> stored.step_count OR actual_links <> stored.target_count
     OR EXISTS (SELECT 1 FROM public.outbound_campaign_handoff_steps WHERE handoff_id = NEW.id AND step_index > stored.step_count)
     OR EXISTS (SELECT 1 FROM public.outbound_campaign_handoff_customer_tasks WHERE handoff_id = NEW.id AND (state <> 'held' OR eligibility <> 'not_evaluated' OR outbound_task_id IS NOT NULL)) THEN
    RAISE EXCEPTION 'outbound campaign handoff must complete with exact held snapshots and receipt' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER outbound_campaign_handoffs_complete_before_commit
AFTER INSERT ON public.outbound_campaign_handoffs
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_outbound_campaign_handoff_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  receipt public.outbound_campaign_handoff_receipts%ROWTYPE;
  handoff public.outbound_campaign_handoffs%ROWTYPE;
  held_count INTEGER;
  not_evaluated_count INTEGER;
BEGIN
  SELECT * INTO receipt FROM public.outbound_campaign_handoff_receipts WHERE id = NEW.id;
  IF receipt.id IS NULL OR receipt.state <> 'completed' OR receipt.handoff_id IS NULL OR receipt.event_id IS NULL THEN
    RAISE EXCEPTION 'outbound campaign handoff receipt cannot commit reserved' USING ERRCODE = '23514';
  END IF;
  SELECT * INTO handoff FROM public.outbound_campaign_handoffs WHERE id = receipt.handoff_id;
  SELECT count(*) FILTER (WHERE state = 'held'), count(*) FILTER (WHERE eligibility = 'not_evaluated')
    INTO held_count, not_evaluated_count
  FROM public.outbound_campaign_handoff_customer_tasks WHERE handoff_id = handoff.id;
  IF handoff.id IS NULL OR receipt.campaign_code <> handoff.campaign_code OR receipt.plan_id <> handoff.plan_id
     OR NOT EXISTS (
       SELECT 1 FROM public.event_log AS event
       WHERE event.id = receipt.event_id AND event.event_type = 'outbound.campaign_handoff_fact_recorded'
         AND event.payload = jsonb_build_object(
           'audit_type', 'accepted', 'handoff_id', handoff.id,
           'campaign_code', handoff.campaign_code, 'plan_id', handoff.plan_id,
           'review_version', handoff.review_version,
           'target_digest', encode(handoff.target_digest, 'hex'),
           'content_digest', encode(handoff.content_digest, 'hex'),
           'target_count', handoff.target_count, 'step_count', handoff.step_count,
           'actor_id', handoff.accepted_by_actor_id,
           'local_only', handoff.local_only,
           'provider_execution_eligible', handoff.provider_execution_eligible,
           'real_external_call_executed', handoff.real_external_call_executed,
           'delivery_proven', handoff.delivery_proven
         )
     )
     OR receipt.result_snapshot <> jsonb_build_object(
       'id', handoff.id, 'campaign_code', handoff.campaign_code, 'plan_id', handoff.plan_id,
       'review_version', handoff.review_version, 'status', handoff.status,
       'target_count', handoff.target_count, 'step_count', handoff.step_count,
       'held_count', held_count, 'blocked_count', 0, 'pending_count', 0,
       'not_evaluated_count', not_evaluated_count, 'eligible_count', 0,
       'inactive_count', 0, 'contact_policy_count', 0,
       'accepted_at_unix_micro', floor(extract(epoch FROM handoff.accepted_at) * 1000000)::bigint,
       'local_only', handoff.local_only,
       'provider_execution_eligible', handoff.provider_execution_eligible,
       'real_external_call_executed', handoff.real_external_call_executed,
       'delivery_proven', handoff.delivery_proven
     ) THEN
    RAISE EXCEPTION 'outbound campaign handoff receipt requires exact header, links, result, and event' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER outbound_campaign_handoff_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.outbound_campaign_handoff_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_handoff_receipt_complete_before_commit();

COMMENT ON TABLE public.outbound_campaign_handoffs IS 'Outbound-owned local-only accepted Campaign handoff; no send task or Provider execution fact.';
COMMENT ON TABLE public.outbound_campaign_handoff_customer_tasks IS 'Held canonical OneID links; dispatch eligibility remains not evaluated until the EXTERNAL_GATE.';

-- +goose Down
LOCK TABLE public.outbound_campaign_handoff_receipts,
  public.outbound_campaign_handoff_customer_tasks,
  public.outbound_campaign_handoff_steps,
  public.outbound_campaign_handoffs IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.outbound_campaign_handoff_receipts)
     OR EXISTS (SELECT 1 FROM public.outbound_campaign_handoffs) THEN
    RAISE EXCEPTION 'cannot roll back outbound campaign handoff facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER outbound_campaign_handoff_receipts_complete_before_commit ON public.outbound_campaign_handoff_receipts;
DROP FUNCTION public.aicrm_outbound_campaign_handoff_receipt_complete_before_commit();
DROP TRIGGER outbound_campaign_handoffs_complete_before_commit ON public.outbound_campaign_handoffs;
DROP FUNCTION public.aicrm_outbound_campaign_handoff_complete_before_commit();
DROP TRIGGER outbound_campaign_handoff_receipts_transition ON public.outbound_campaign_handoff_receipts;
DROP FUNCTION public.aicrm_outbound_campaign_handoff_receipt_transition_valid();
DROP TRIGGER outbound_campaign_handoff_customer_tasks_insert_valid ON public.outbound_campaign_handoff_customer_tasks;
DROP TRIGGER outbound_campaign_handoff_steps_insert_valid ON public.outbound_campaign_handoff_steps;
DROP FUNCTION public.aicrm_outbound_campaign_handoff_child_insert_valid();
DROP TRIGGER outbound_campaign_handoff_customer_tasks_immutable ON public.outbound_campaign_handoff_customer_tasks;
DROP TRIGGER outbound_campaign_handoff_steps_immutable ON public.outbound_campaign_handoff_steps;
DROP TRIGGER outbound_campaign_handoffs_immutable ON public.outbound_campaign_handoffs;
DROP FUNCTION public.aicrm_outbound_campaign_handoff_immutable();
DROP TABLE public.outbound_campaign_handoff_receipts;
DROP TABLE public.outbound_campaign_handoff_customer_tasks;
DROP TABLE public.outbound_campaign_handoff_steps;
DROP TABLE public.outbound_campaign_handoffs;
