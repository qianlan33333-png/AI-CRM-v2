-- +goose Up
-- Freeze the exact reviewed content before an outbound effect is accepted.
-- Existing bindings were created from the immutable handoff step, so that
-- step is the only valid backfill source.
ALTER TABLE public.outbound_campaign_dispatches
  ADD COLUMN content_snapshot TEXT;

UPDATE public.outbound_campaign_dispatches AS dispatch
SET content_snapshot = step.content
FROM public.outbound_campaign_handoff_steps AS step
WHERE step.handoff_id = dispatch.handoff_id
  AND step.step_index = dispatch.step_index;

ALTER TABLE public.outbound_campaign_dispatches
  ALTER COLUMN content_snapshot SET NOT NULL,
  ADD CONSTRAINT outbound_campaign_dispatches_content_snapshot_check CHECK (content_snapshot <> '');

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_outbound_campaign_dispatches_no_delete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'outbound campaign dispatch facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.handoff_id IS DISTINCT FROM OLD.handoff_id
     OR NEW.customer_id IS DISTINCT FROM OLD.customer_id OR NEW.step_index IS DISTINCT FROM OLD.step_index
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id OR NEW.recipient_digest IS DISTINCT FROM OLD.recipient_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.content_snapshot IS DISTINCT FROM OLD.content_snapshot
     OR NEW.sender_userid_snapshot IS DISTINCT FROM OLD.sender_userid_snapshot
     OR NEW.external_userid_snapshot IS DISTINCT FROM OLD.external_userid_snapshot
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'outbound campaign dispatch identity is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
LOCK TABLE public.outbound_campaign_dispatches IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM public.outbound_campaign_dispatches AS dispatch
    JOIN public.outbound_campaign_handoff_steps AS step
      ON step.handoff_id = dispatch.handoff_id AND step.step_index = dispatch.step_index
    WHERE dispatch.content_snapshot IS DISTINCT FROM step.content
  ) THEN
    RAISE EXCEPTION 'cannot remove reviewed outbound campaign content snapshots' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE public.outbound_campaign_dispatches
  DROP CONSTRAINT outbound_campaign_dispatches_content_snapshot_check;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_outbound_campaign_dispatches_no_delete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'outbound campaign dispatch facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.handoff_id IS DISTINCT FROM OLD.handoff_id
     OR NEW.customer_id IS DISTINCT FROM OLD.customer_id OR NEW.step_index IS DISTINCT FROM OLD.step_index
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id OR NEW.recipient_digest IS DISTINCT FROM OLD.recipient_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.sender_userid_snapshot IS DISTINCT FROM OLD.sender_userid_snapshot
     OR NEW.external_userid_snapshot IS DISTINCT FROM OLD.external_userid_snapshot
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'outbound campaign dispatch identity is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE public.outbound_campaign_dispatches DROP COLUMN content_snapshot;
