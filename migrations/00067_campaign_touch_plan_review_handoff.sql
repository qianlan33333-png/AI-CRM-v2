-- +goose Up
-- Campaign-owned review and handoff facts. Approval creates no Outbound task
-- and invokes no Provider; EvCloudCampaignFact is delivered only to the local
-- Events receipt consumer after commit.
CREATE TABLE public.cloud_campaign_touch_plan_reviews (
  plan_id TEXT PRIMARY KEY REFERENCES public.cloud_campaign_touch_plans(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  campaign_code TEXT NOT NULL REFERENCES public.cloud_campaigns(campaign_code) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('draft', 'pending_review', 'approved', 'rejected')),
  version BIGINT NOT NULL CHECK (version > 0),
  submitted_by_actor_id BIGINT CHECK (submitted_by_actor_id > 0),
  submitted_at TIMESTAMPTZ,
  reviewed_by_actor_id BIGINT CHECK (reviewed_by_actor_id > 0),
  reviewed_at TIMESTAMPTZ,
  confirmation_digest BYTEA CHECK (confirmation_digest IS NULL OR octet_length(confirmation_digest) = 32),
  CONSTRAINT cloud_campaign_touch_plan_reviews_shape CHECK (
    (status = 'draft' AND version = 1 AND submitted_by_actor_id IS NULL AND submitted_at IS NULL AND reviewed_by_actor_id IS NULL AND reviewed_at IS NULL AND confirmation_digest IS NULL)
    OR (status = 'pending_review' AND version > 1 AND submitted_by_actor_id IS NOT NULL AND submitted_at IS NOT NULL AND reviewed_by_actor_id IS NULL AND reviewed_at IS NULL AND confirmation_digest IS NULL)
    OR (status IN ('approved', 'rejected') AND version > 2 AND submitted_by_actor_id IS NOT NULL AND submitted_at IS NOT NULL AND reviewed_by_actor_id IS NOT NULL AND reviewed_at IS NOT NULL AND confirmation_digest IS NOT NULL)
  )
);

INSERT INTO public.cloud_campaign_touch_plan_reviews (plan_id, campaign_code, status, version)
SELECT id, campaign_code, 'draft', 1 FROM public.cloud_campaign_touch_plans;

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_create_draft()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  INSERT INTO public.cloud_campaign_touch_plan_reviews (plan_id, campaign_code, status, version)
  VALUES (NEW.id, NEW.campaign_code, 'draft', 1);
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER cloud_campaign_touch_plan_review_create_draft
AFTER INSERT ON public.cloud_campaign_touch_plans
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_create_draft();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_scope_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plans WHERE id = NEW.plan_id AND campaign_code = NEW.campaign_code) THEN
    RAISE EXCEPTION 'campaign touch plan review must match immutable plan campaign' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER cloud_campaign_touch_plan_reviews_scope
BEFORE INSERT OR UPDATE ON public.cloud_campaign_touch_plan_reviews
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_scope_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'campaign touch plan review facts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.plan_id IS DISTINCT FROM OLD.plan_id OR NEW.campaign_code IS DISTINCT FROM OLD.campaign_code THEN
    RAISE EXCEPTION 'campaign touch plan review identity is immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.status = 'draft' AND NEW.status = 'pending_review'
     AND NEW.version = OLD.version + 1
     AND NEW.submitted_by_actor_id IS NOT NULL AND NEW.submitted_at IS NOT NULL
     AND NEW.reviewed_by_actor_id IS NULL AND NEW.reviewed_at IS NULL
     AND NEW.confirmation_digest IS NULL THEN
    RETURN NEW;
  END IF;
  IF OLD.status = 'pending_review' AND NEW.status IN ('approved', 'rejected')
     AND NEW.version = OLD.version + 1
     AND NEW.submitted_by_actor_id IS NOT DISTINCT FROM OLD.submitted_by_actor_id
     AND NEW.submitted_at IS NOT DISTINCT FROM OLD.submitted_at
     AND NEW.reviewed_by_actor_id IS NOT NULL AND NEW.reviewed_at IS NOT NULL
     AND NEW.confirmation_digest IS NOT NULL THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid campaign touch plan review transition' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER cloud_campaign_touch_plan_reviews_transition
BEFORE UPDATE OR DELETE ON public.cloud_campaign_touch_plan_reviews
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_transition_valid();

CREATE TABLE public.cloud_campaign_touch_plan_review_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  operation TEXT NOT NULL CHECK (operation IN ('submit', 'approve', 'reject')),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  plan_id TEXT NOT NULL REFERENCES public.cloud_campaign_touch_plans(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  campaign_code TEXT NOT NULL REFERENCES public.cloud_campaigns(campaign_code) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  event_id BIGINT REFERENCES public.event_log(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  handoff_event_id BIGINT REFERENCES public.event_log(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved', 'completed')),
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  result_snapshot JSONB,
  CONSTRAINT cloud_campaign_touch_plan_review_receipts_completion CHECK (
    (state = 'reserved' AND event_id IS NULL AND handoff_event_id IS NULL AND completed_at IS NULL AND result_snapshot IS NULL)
    OR (state = 'completed' AND event_id IS NOT NULL AND completed_at IS NOT NULL AND result_snapshot IS NOT NULL AND ((operation = 'approve' AND handoff_event_id IS NOT NULL) OR (operation <> 'approve' AND handoff_event_id IS NULL)))
  ),
  UNIQUE (actor_id, key_digest),
  UNIQUE (event_id),
  UNIQUE (handoff_event_id)
);

CREATE TABLE public.cloud_campaign_touch_plan_handoffs (
  plan_id TEXT PRIMARY KEY REFERENCES public.cloud_campaign_touch_plans(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  review_version BIGINT NOT NULL CHECK (review_version > 2),
  status TEXT NOT NULL CHECK (status = 'pending_outbound_acceptance'),
  local_only BOOLEAN NOT NULL DEFAULT TRUE CHECK (local_only),
  provider_execution_eligible BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT provider_execution_eligible),
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT real_external_call_executed),
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT delivery_proven),
  created_at TIMESTAMPTZ NOT NULL
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_handoff_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'campaign touch plan handoffs are immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER cloud_campaign_touch_plan_handoffs_immutable
BEFORE UPDATE OR DELETE ON public.cloud_campaign_touch_plan_handoffs
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_handoff_immutable();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_receipt_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed campaign touch plan review receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.operation IS DISTINCT FROM OLD.operation OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.plan_id IS DISTINCT FROM OLD.plan_id OR NEW.campaign_code IS DISTINCT FROM OLD.campaign_code
     OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.event_id IS NULL OR NEW.completed_at IS NULL OR NEW.result_snapshot IS NULL THEN
    RAISE EXCEPTION 'invalid campaign touch plan review receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER cloud_campaign_touch_plan_review_receipts_transition
BEFORE UPDATE OR DELETE ON public.cloud_campaign_touch_plan_review_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_receipt_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE required_audit_type TEXT; required_status TEXT; receipt public.cloud_campaign_touch_plan_review_receipts%ROWTYPE;
BEGIN
  SELECT * INTO receipt FROM public.cloud_campaign_touch_plan_review_receipts WHERE id = NEW.id;
  IF NOT FOUND OR receipt.state <> 'completed' OR receipt.event_id IS NULL
     OR receipt.completed_at IS NULL OR receipt.result_snapshot IS NULL
     OR (receipt.operation = 'approve' AND receipt.handoff_event_id IS NULL)
     OR (receipt.operation <> 'approve' AND receipt.handoff_event_id IS NOT NULL) THEN
    RAISE EXCEPTION 'campaign touch plan review receipt must complete with its local fact events' USING ERRCODE = '23514';
  END IF;
  required_audit_type := CASE receipt.operation WHEN 'submit' THEN 'touch_plan_submitted' WHEN 'approve' THEN 'approved' ELSE 'rejected' END;
  required_status := CASE receipt.operation WHEN 'submit' THEN 'pending_review' WHEN 'approve' THEN 'approved' ELSE 'rejected' END;
  IF NOT EXISTS (
    SELECT 1 FROM public.cloud_campaign_touch_plan_reviews AS review
    JOIN public.event_log AS event ON event.id = receipt.event_id
    LEFT JOIN public.cloud_campaign_touch_plan_handoffs AS handoff ON handoff.plan_id = receipt.plan_id
    LEFT JOIN public.event_log AS handoff_event ON handoff_event.id = receipt.handoff_event_id
    WHERE review.plan_id = receipt.plan_id AND review.campaign_code = receipt.campaign_code
      AND review.status = required_status AND event.payload ->> 'review_version' = review.version::text
      AND ((receipt.operation = 'submit' AND review.submitted_by_actor_id = receipt.actor_id)
           OR (receipt.operation <> 'submit' AND review.reviewed_by_actor_id = receipt.actor_id))
      AND event.event_type = 'cloud_campaign.fact_recorded'
      AND event.payload ->> 'audit_type' = required_audit_type
      AND event.payload ->> 'plan_id' = receipt.plan_id
      AND event.payload ->> 'campaign_code' = receipt.campaign_code
      AND event.payload ->> 'actor_id' = receipt.actor_id::text
      AND ((receipt.operation <> 'approve' AND handoff_event.id IS NULL)
           OR (receipt.operation = 'approve' AND handoff.review_version = review.version
               AND handoff_event.event_type = 'cloud_campaign.fact_recorded'
               AND handoff_event.payload ->> 'audit_type' = 'handoff_created'
               AND handoff_event.payload ->> 'plan_id' = receipt.plan_id
               AND handoff_event.payload ->> 'campaign_code' = receipt.campaign_code
               AND handoff_event.payload ->> 'review_version' = review.version::text
               AND handoff_event.payload ->> 'actor_id' = receipt.actor_id::text))
      AND receipt.result_snapshot = jsonb_build_object(
        'review', jsonb_build_object(
          'plan_id', review.plan_id, 'campaign_code', review.campaign_code,
          'status', review.status, 'version', review.version,
          'submitted_by_actor_id', review.submitted_by_actor_id,
          'submitted_at_unix_micro', floor(extract(epoch FROM review.submitted_at) * 1000000)::bigint,
          'reviewed_by_actor_id', COALESCE(review.reviewed_by_actor_id, 0),
          'reviewed_at_unix_micro', COALESCE(floor(extract(epoch FROM review.reviewed_at) * 1000000)::bigint, 0),
          'confirmation_digest', COALESCE(encode(review.confirmation_digest, 'hex'), '')
        ),
        'handoff', CASE WHEN receipt.operation = 'approve' THEN jsonb_build_object(
          'plan_id', handoff.plan_id, 'campaign_code', receipt.campaign_code,
          'review_version', handoff.review_version, 'status', handoff.status,
          'created_at_unix_micro', floor(extract(epoch FROM handoff.created_at) * 1000000)::bigint,
          'local_only', handoff.local_only, 'provider_execution_eligible', handoff.provider_execution_eligible,
          'real_external_call_executed', handoff.real_external_call_executed, 'delivery_proven', handoff.delivery_proven
        ) ELSE 'null'::jsonb END,
        'event_ids', CASE WHEN receipt.operation = 'approve' THEN jsonb_build_array(receipt.event_id, receipt.handoff_event_id) ELSE jsonb_build_array(receipt.event_id) END
      )
  ) THEN
    RAISE EXCEPTION 'campaign touch plan review receipt requires its exact immutable fact snapshot and events' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER cloud_campaign_touch_plan_review_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.cloud_campaign_touch_plan_review_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_handoff_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.cloud_campaign_touch_plan_reviews AS review
    WHERE review.plan_id = NEW.plan_id AND review.status = 'approved' AND review.version = NEW.review_version
  ) THEN
    RAISE EXCEPTION 'campaign handoff requires approved immutable review' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER cloud_campaign_touch_plan_handoffs_complete_before_commit
AFTER INSERT ON public.cloud_campaign_touch_plan_handoffs
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_handoff_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE operation_name TEXT; audit_type TEXT; actor BIGINT;
BEGIN
  IF NEW.version = 1 THEN RETURN NULL; END IF;
  SELECT CASE status WHEN 'pending_review' THEN 'submit' WHEN 'approved' THEN 'approve' ELSE 'reject' END,
         status,
         CASE WHEN status = 'pending_review' THEN submitted_by_actor_id ELSE reviewed_by_actor_id END
  INTO operation_name, audit_type, actor
  FROM public.cloud_campaign_touch_plan_reviews
  WHERE plan_id = NEW.plan_id AND campaign_code = NEW.campaign_code;
  IF NOT FOUND OR NOT EXISTS (
    SELECT 1
    FROM public.cloud_campaign_touch_plan_review_receipts AS receipt
    JOIN public.event_log AS event ON event.id = receipt.event_id
    WHERE receipt.plan_id = NEW.plan_id AND receipt.campaign_code = NEW.campaign_code
      AND receipt.operation = operation_name AND receipt.actor_id = actor AND receipt.state = 'completed'
      AND event.event_type = 'cloud_campaign.fact_recorded'
      AND event.payload ->> 'audit_type' = CASE audit_type WHEN 'pending_review' THEN 'touch_plan_submitted' ELSE audit_type END
      AND event.payload ->> 'plan_id' = NEW.plan_id
      AND event.payload ->> 'campaign_code' = NEW.campaign_code
      AND event.payload ->> 'review_version' = NEW.version::text
      AND event.payload ->> 'actor_id' = actor::text
  ) THEN
    RAISE EXCEPTION 'campaign touch plan review requires its completed local receipt and fact event' USING ERRCODE = '23514';
  END IF;
  IF audit_type = 'approved' AND NOT EXISTS (
    SELECT 1
    FROM public.cloud_campaign_touch_plan_handoffs AS handoff
    JOIN public.cloud_campaign_touch_plan_review_receipts AS receipt ON receipt.plan_id = handoff.plan_id AND receipt.operation = 'approve' AND receipt.state = 'completed'
    JOIN public.event_log AS event ON event.id = receipt.handoff_event_id
    WHERE handoff.plan_id = NEW.plan_id AND handoff.review_version = NEW.version
      AND event.event_type = 'cloud_campaign.fact_recorded'
      AND event.payload ->> 'audit_type' = 'handoff_created'
      AND event.payload ->> 'plan_id' = NEW.plan_id
      AND event.payload ->> 'campaign_code' = NEW.campaign_code
      AND event.payload ->> 'review_version' = NEW.version::text
      AND event.payload ->> 'actor_id' = actor::text
  ) THEN
    RAISE EXCEPTION 'approved campaign touch plan review requires its immutable local handoff fact' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER cloud_campaign_touch_plan_reviews_complete_before_commit
AFTER UPDATE ON public.cloud_campaign_touch_plan_reviews
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.aicrm_cloud_campaign_touch_plan_review_complete_before_commit();

COMMENT ON TABLE public.cloud_campaign_touch_plan_handoffs IS 'Campaign-owned immutable local handoff pending later Outbound acceptance; no Provider execution fact.';

-- +goose Down
LOCK TABLE public.cloud_campaign_touch_plan_review_receipts, public.cloud_campaign_touch_plan_handoffs, public.cloud_campaign_touch_plan_reviews, public.cloud_campaign_touch_plans IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plan_review_receipts)
     OR EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plan_handoffs)
     OR EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plans) THEN
    RAISE EXCEPTION 'cannot roll back cloud campaign touch plan review facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER cloud_campaign_touch_plan_handoffs_complete_before_commit ON public.cloud_campaign_touch_plan_handoffs;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_handoff_before_commit();
DROP TRIGGER cloud_campaign_touch_plan_handoffs_immutable ON public.cloud_campaign_touch_plan_handoffs;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_handoff_immutable();
DROP TRIGGER cloud_campaign_touch_plan_review_receipts_complete_before_commit ON public.cloud_campaign_touch_plan_review_receipts;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_review_receipt_complete_before_commit();
DROP TRIGGER cloud_campaign_touch_plan_reviews_complete_before_commit ON public.cloud_campaign_touch_plan_reviews;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_review_complete_before_commit();
DROP TRIGGER cloud_campaign_touch_plan_review_receipts_transition ON public.cloud_campaign_touch_plan_review_receipts;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_review_receipt_valid();
DROP TABLE public.cloud_campaign_touch_plan_handoffs;
DROP TABLE public.cloud_campaign_touch_plan_review_receipts;
DROP TRIGGER cloud_campaign_touch_plan_reviews_transition ON public.cloud_campaign_touch_plan_reviews;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_review_transition_valid();
DROP TRIGGER cloud_campaign_touch_plan_reviews_scope ON public.cloud_campaign_touch_plan_reviews;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_review_scope_valid();
DROP TRIGGER cloud_campaign_touch_plan_review_create_draft ON public.cloud_campaign_touch_plans;
DROP FUNCTION public.aicrm_cloud_campaign_touch_plan_review_create_draft();
DROP TABLE public.cloud_campaign_touch_plan_reviews;
