-- +goose Up
-- C01 owns the mapping from an immutable 00068 Campaign handoff to the
-- digest-only External Effects Runtime. The immutable handoff itself is never
-- updated and external_contact_id/payload/provider bodies never enter these
-- tables.
CREATE TABLE public.outbound_campaign_dispatches (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  handoff_id BIGINT NOT NULL REFERENCES public.outbound_campaign_handoffs(id) ON DELETE RESTRICT,
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  step_index INTEGER NOT NULL CHECK (step_index BETWEEN 1 AND 100),
  external_effect_id BIGINT UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  recipient_digest TEXT NOT NULL CHECK (recipient_digest ~ '^sha256:[0-9a-f]{64}$'),
  payload_digest TEXT NOT NULL CHECK (payload_digest ~ '^sha256:[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('accepted','queued','attempted','executed','outcome_unknown','reconciled','retryable_failed','final_failed','blocked')),
  block_reason TEXT CHECK (block_reason IN ('external_gate_disabled','identity_unresolved','contact_policy','inactive_customer','provider_preflight_failed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT outbound_campaign_dispatches_shape CHECK (
    (state = 'blocked' AND external_effect_id IS NULL AND block_reason IS NOT NULL)
    OR (state <> 'blocked' AND external_effect_id IS NOT NULL AND block_reason IS NULL)
  ),
  UNIQUE (handoff_id, customer_id, step_index)
);
CREATE INDEX outbound_campaign_dispatches_handoff_idx ON public.outbound_campaign_dispatches(handoff_id, id);

CREATE TABLE public.outbound_campaign_dispatch_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  handoff_id BIGINT NOT NULL REFERENCES public.outbound_campaign_handoffs(id) ON DELETE RESTRICT,
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  result_snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(actor_id, key_digest)
);

CREATE TABLE public.outbound_campaign_provider_attempt_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  external_effect_id BIGINT NOT NULL REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
  completion TEXT NOT NULL CHECK (completion IN ('executed','retryable_failed','final_failed','outcome_unknown','reconciled')),
  provider_receipt_digest TEXT NOT NULL CHECK (provider_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT delivery_proven),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(external_effect_id, attempt_number, completion)
);

-- A dispatch binding is append-only. Runtime terminal state remains owned by
-- external_effects; the local state is a safe operator projection only.
CREATE FUNCTION public.aicrm_outbound_campaign_dispatches_no_delete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'outbound campaign dispatch facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.handoff_id IS DISTINCT FROM OLD.handoff_id
     OR NEW.customer_id IS DISTINCT FROM OLD.customer_id OR NEW.step_index IS DISTINCT FROM OLD.step_index
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id OR NEW.recipient_digest IS DISTINCT FROM OLD.recipient_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'outbound campaign dispatch identity is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER outbound_campaign_dispatches_guard
BEFORE UPDATE OR DELETE ON public.outbound_campaign_dispatches
FOR EACH ROW EXECUTE FUNCTION public.aicrm_outbound_campaign_dispatches_no_delete();

-- +goose Down
LOCK TABLE public.outbound_campaign_provider_attempt_receipts, public.outbound_campaign_dispatch_receipts, public.outbound_campaign_dispatches IN SHARE ROW EXCLUSIVE MODE;
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.outbound_campaign_provider_attempt_receipts)
     OR EXISTS (SELECT 1 FROM public.outbound_campaign_dispatch_receipts)
     OR EXISTS (SELECT 1 FROM public.outbound_campaign_dispatches) THEN
    RAISE EXCEPTION 'cannot roll back populated outbound campaign external dispatch facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
DROP TRIGGER outbound_campaign_dispatches_guard ON public.outbound_campaign_dispatches;
DROP FUNCTION public.aicrm_outbound_campaign_dispatches_no_delete();
DROP TABLE public.outbound_campaign_provider_attempt_receipts;
DROP TABLE public.outbound_campaign_dispatch_receipts;
DROP TABLE public.outbound_campaign_dispatches;
