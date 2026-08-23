-- +goose Up
-- Campaign-owned initiation snapshots carry canonical OneIDs and copied
-- Campaign steps only. The same-UoW audit event uses the existing local
-- Campaign Fact delivery binding; its consumer only completes an Events
-- receipt. No Outbound, provider, runtime, recipient-delivery, or material
-- reference fact is created here.
CREATE TABLE public.cloud_campaign_touch_plans (
  id TEXT PRIMARY KEY CHECK (id ~ '^ctp_[0-9a-f]{64}$'),
  campaign_code TEXT NOT NULL REFERENCES public.cloud_campaigns(campaign_code) ON DELETE RESTRICT,
  campaign_version BIGINT NOT NULL CHECK (campaign_version > 0),
  source_kind TEXT NOT NULL CHECK (source_kind IN (
    'customer_selection', 'segment_members', 'ai_audience_package_members'
  )),
  customer_selection_id TEXT,
  customer_selection_version TEXT,
  segment_id BIGINT,
  audience_package_id BIGINT,
  audience_package_version BIGINT,
  member_snapshot_watermark TIMESTAMPTZ,
  source_digest BYTEA NOT NULL CHECK (octet_length(source_digest) = 32),
  target_digest BYTEA NOT NULL CHECK (octet_length(target_digest) = 32),
  content_digest BYTEA NOT NULL CHECK (octet_length(content_digest) = 32),
  target_count INTEGER NOT NULL CHECK (target_count BETWEEN 1 AND 1000),
  content_step_count INTEGER NOT NULL CHECK (content_step_count BETWEEN 1 AND 100),
  candidate_count INTEGER NOT NULL CHECK (candidate_count BETWEEN 1 AND 1000),
  active_customer_count INTEGER NOT NULL CHECK (active_customer_count >= 0),
  inactive_excluded_count INTEGER NOT NULL CHECK (inactive_excluded_count >= 0),
  policy_excluded_count INTEGER NOT NULL CHECK (policy_excluded_count >= 0),
  owner_actor_id BIGINT NOT NULL CHECK (owner_actor_id > 0),
  local_only BOOLEAN NOT NULL DEFAULT TRUE CHECK (local_only),
  provider_execution_eligible BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT provider_execution_eligible),
  runtime_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT runtime_executed),
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT real_external_call_executed),
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT delivery_proven),
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT cloud_campaign_touch_plans_source_shape CHECK (
    (source_kind = 'customer_selection'
      AND customer_selection_id = 'local_selection'
      AND customer_selection_version = 'v1'
      AND segment_id IS NULL
      AND audience_package_id IS NULL
      AND audience_package_version IS NULL
      AND member_snapshot_watermark IS NULL)
    OR (source_kind = 'segment_members'
      AND customer_selection_id IS NULL
      AND customer_selection_version IS NULL
      AND segment_id > 0
      AND audience_package_id IS NULL
      AND audience_package_version IS NULL
      AND member_snapshot_watermark IS NOT NULL)
    OR (source_kind = 'ai_audience_package_members'
      AND customer_selection_id IS NULL
      AND customer_selection_version IS NULL
      AND segment_id IS NULL
      AND audience_package_id > 0
      AND audience_package_version > 0
      AND member_snapshot_watermark IS NOT NULL)
  ),
  CONSTRAINT cloud_campaign_touch_plans_exclusions CHECK (
    candidate_count = target_count + inactive_excluded_count + policy_excluded_count
    AND active_customer_count = target_count + policy_excluded_count
  )
);

CREATE TABLE public.cloud_campaign_touch_plan_targets (
  plan_id TEXT NOT NULL REFERENCES public.cloud_campaign_touch_plans(id) ON DELETE RESTRICT,
  -- Deliberately no customers FK: this is an immutable canonical OneID
  -- snapshot and later customer soft/hard lifecycle changes must not erase it.
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  PRIMARY KEY (plan_id, customer_id)
);

CREATE TABLE public.cloud_campaign_touch_plan_steps (
  plan_id TEXT NOT NULL REFERENCES public.cloud_campaign_touch_plans(id) ON DELETE RESTRICT,
  step_index INTEGER NOT NULL CHECK (step_index BETWEEN 1 AND 100),
  delay_minutes INTEGER NOT NULL CHECK (delay_minutes >= 0),
  content TEXT NOT NULL CHECK (content <> ''),
  PRIMARY KEY (plan_id, step_index)
);

CREATE TABLE public.cloud_campaign_touch_plan_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  plan_id TEXT NOT NULL REFERENCES public.cloud_campaign_touch_plans(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  event_id BIGINT REFERENCES public.event_log(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved', 'completed')),
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT cloud_campaign_touch_plan_receipts_completion CHECK (
    (state = 'reserved' AND event_id IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND event_id IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (actor_id, key_digest),
  UNIQUE (plan_id),
  UNIQUE (event_id)
);

CREATE INDEX cloud_campaign_touch_plans_campaign_created_idx
  ON public.cloud_campaign_touch_plans (campaign_code, created_at DESC, id DESC);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'cloud campaign touch plan snapshots are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER cloud_campaign_touch_plans_immutable
BEFORE UPDATE OR DELETE ON public.cloud_campaign_touch_plans
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_immutable();

CREATE TRIGGER cloud_campaign_touch_plan_targets_immutable
BEFORE UPDATE OR DELETE ON public.cloud_campaign_touch_plan_targets
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_immutable();

CREATE TRIGGER cloud_campaign_touch_plan_steps_immutable
BEFORE UPDATE OR DELETE ON public.cloud_campaign_touch_plan_steps
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_immutable();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_child_insert_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM public.cloud_campaign_touch_plan_receipts
    WHERE plan_id = NEW.plan_id AND state = 'completed'
  ) THEN
    RAISE EXCEPTION 'cannot append to a completed campaign touch plan snapshot' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER cloud_campaign_touch_plan_targets_insert_valid
BEFORE INSERT ON public.cloud_campaign_touch_plan_targets
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_child_insert_valid();

CREATE TRIGGER cloud_campaign_touch_plan_steps_insert_valid
BEFORE INSERT ON public.cloud_campaign_touch_plan_steps
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_child_insert_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  actual_targets INTEGER;
  actual_steps INTEGER;
BEGIN
  SELECT count(*) INTO actual_targets
  FROM public.cloud_campaign_touch_plan_targets WHERE plan_id = NEW.id;
  SELECT count(*) INTO actual_steps
  FROM public.cloud_campaign_touch_plan_steps WHERE plan_id = NEW.id;
  IF NOT EXISTS (
    SELECT 1
    FROM public.cloud_campaign_touch_plan_receipts AS receipt
    JOIN public.event_log AS event ON event.id = receipt.event_id
    WHERE receipt.plan_id = NEW.id
      AND receipt.actor_id = NEW.owner_actor_id
      AND receipt.state = 'completed'
      AND receipt.completed_at IS NOT NULL
      AND event.event_type = 'cloud_campaign.fact_recorded'
      AND event.payload ->> 'audit_type' = 'touch_plan_created'
      AND event.payload ->> 'plan_id' = NEW.id
      AND event.payload ->> 'campaign_code' = NEW.campaign_code
      AND event.payload ->> 'owner_actor_id' = NEW.owner_actor_id::text
      AND event.payload ->> 'target_digest' = encode(NEW.target_digest, 'hex')
      AND event.payload ->> 'target_count' = NEW.target_count::text
      AND event.payload ->> 'content_digest' = encode(NEW.content_digest, 'hex')
  ) OR actual_targets <> NEW.target_count OR actual_steps <> NEW.content_step_count THEN
    RAISE EXCEPTION 'campaign touch plan must complete with its receipt, event, and declared snapshot rows'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER cloud_campaign_touch_plans_complete_before_commit
AFTER INSERT ON public.cloud_campaign_touch_plans
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM public.cloud_campaign_touch_plan_receipts AS receipt
    JOIN public.cloud_campaign_touch_plans AS plan ON plan.id = receipt.plan_id
    JOIN public.event_log AS event ON event.id = receipt.event_id
    WHERE receipt.id = NEW.id
      AND receipt.state = 'completed'
      AND receipt.completed_at IS NOT NULL
      AND plan.owner_actor_id = receipt.actor_id
      AND event.event_type = 'cloud_campaign.fact_recorded'
      AND event.payload ->> 'audit_type' = 'touch_plan_created'
      AND event.payload ->> 'plan_id' = plan.id
      AND event.payload ->> 'campaign_code' = plan.campaign_code
      AND event.payload ->> 'owner_actor_id' = plan.owner_actor_id::text
      AND event.payload ->> 'target_digest' = encode(plan.target_digest, 'hex')
      AND event.payload ->> 'target_count' = plan.target_count::text
      AND event.payload ->> 'content_digest' = encode(plan.content_digest, 'hex')
  ) THEN
    RAISE EXCEPTION 'campaign touch plan receipt must complete with its local plan and event in one transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER cloud_campaign_touch_plan_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.cloud_campaign_touch_plan_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed campaign touch plan receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed'
     OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.plan_id IS DISTINCT FROM OLD.plan_id
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.event_id IS NULL
     OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid campaign touch plan receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER cloud_campaign_touch_plan_receipts_transition
BEFORE UPDATE OR DELETE ON public.cloud_campaign_touch_plan_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_receipt_transition_valid();

COMMENT ON TABLE public.cloud_campaign_touch_plans IS
  'Campaign-owned immutable local draft touch-plan snapshots; no external execution evidence.';

-- +goose Down
LOCK TABLE public.cloud_campaign_touch_plan_receipts,
  public.cloud_campaign_touch_plan_targets,
  public.cloud_campaign_touch_plan_steps,
  public.cloud_campaign_touch_plans IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plans)
     OR EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plan_receipts) THEN
    RAISE EXCEPTION 'cannot roll back cloud campaign touch plan facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER cloud_campaign_touch_plan_receipts_transition ON public.cloud_campaign_touch_plan_receipts;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_receipt_transition_valid();
DROP TRIGGER cloud_campaign_touch_plan_receipts_complete_before_commit ON public.cloud_campaign_touch_plan_receipts;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_receipt_complete_before_commit();
DROP TRIGGER cloud_campaign_touch_plan_steps_immutable ON public.cloud_campaign_touch_plan_steps;
DROP TRIGGER cloud_campaign_touch_plan_targets_immutable ON public.cloud_campaign_touch_plan_targets;
DROP TRIGGER cloud_campaign_touch_plans_immutable ON public.cloud_campaign_touch_plans;
DROP TRIGGER cloud_campaign_touch_plan_steps_insert_valid ON public.cloud_campaign_touch_plan_steps;
DROP TRIGGER cloud_campaign_touch_plan_targets_insert_valid ON public.cloud_campaign_touch_plan_targets;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_child_insert_valid();
DROP TRIGGER cloud_campaign_touch_plans_complete_before_commit ON public.cloud_campaign_touch_plans;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_complete_before_commit();
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_immutable();
DROP TABLE public.cloud_campaign_touch_plan_receipts;
DROP TABLE public.cloud_campaign_touch_plan_steps;
DROP TABLE public.cloud_campaign_touch_plan_targets;
DROP TABLE public.cloud_campaign_touch_plans;
