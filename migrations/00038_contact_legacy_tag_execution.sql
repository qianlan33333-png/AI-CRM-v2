-- +goose Up
-- Contact records local acceptance for the frozen legacy tag routes. These
-- tables contain no WeCom credentials or provider results: a queued River job
-- is deliberately not evidence that a provider operation was attempted.
CREATE TABLE public.legacy_tag_sync_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL,
  idempotency_key TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  kind TEXT NOT NULL,
  trace_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'reserved',
  event_id BIGINT,
  river_job_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at TIMESTAMPTZ,
  CONSTRAINT legacy_tag_sync_receipts_actor CHECK (actor_id > 0),
  CONSTRAINT legacy_tag_sync_receipts_key CHECK (btrim(idempotency_key) = idempotency_key AND char_length(idempotency_key) BETWEEN 1 AND 200),
  CONSTRAINT legacy_tag_sync_receipts_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT legacy_tag_sync_receipts_kind CHECK (kind IN ('manual', 'due')),
  CONSTRAINT legacy_tag_sync_receipts_trace CHECK (btrim(trace_id) = trace_id AND char_length(trace_id) <= 200),
  CONSTRAINT legacy_tag_sync_receipts_state CHECK (state IN ('reserved', 'queued')),
  CONSTRAINT legacy_tag_sync_receipts_acceptance CHECK (
    (state = 'reserved' AND event_id IS NULL AND river_job_id IS NULL AND accepted_at IS NULL) OR
    (state = 'queued' AND event_id IS NOT NULL AND event_id > 0 AND river_job_id IS NOT NULL AND river_job_id > 0 AND accepted_at IS NOT NULL)
  ),
  UNIQUE (actor_id, key_digest)
);

CREATE TABLE public.legacy_tag_live_mutation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL,
  idempotency_key TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  operation TEXT NOT NULL,
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'reserved',
  event_id BIGINT,
  river_job_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at TIMESTAMPTZ,
  CONSTRAINT legacy_tag_live_mutation_receipts_actor CHECK (actor_id > 0),
  CONSTRAINT legacy_tag_live_mutation_receipts_key CHECK (btrim(idempotency_key) = idempotency_key AND char_length(idempotency_key) BETWEEN 1 AND 200),
  CONSTRAINT legacy_tag_live_mutation_receipts_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT legacy_tag_live_mutation_receipts_operation CHECK (operation IN ('mark', 'unmark')),
  CONSTRAINT legacy_tag_live_mutation_receipts_payload CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT legacy_tag_live_mutation_receipts_trace CHECK (btrim(trace_id) = trace_id AND char_length(trace_id) <= 200),
  CONSTRAINT legacy_tag_live_mutation_receipts_state CHECK (state IN ('reserved', 'queued')),
  CONSTRAINT legacy_tag_live_mutation_receipts_acceptance CHECK (
    (state = 'reserved' AND event_id IS NULL AND river_job_id IS NULL AND accepted_at IS NULL) OR
    (state = 'queued' AND event_id IS NOT NULL AND event_id > 0 AND river_job_id IS NOT NULL AND river_job_id > 0 AND accepted_at IS NOT NULL)
  ),
  UNIQUE (actor_id, key_digest)
);

-- A singleton local projection makes the execution gate fail closed. It is
-- intentionally not a provider health check and contains no external facts.
CREATE TABLE public.legacy_tag_execution_status (
  singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT legacy_tag_execution_status_payload CHECK (jsonb_typeof(payload) = 'object')
);
INSERT INTO public.legacy_tag_execution_status (singleton, payload)
VALUES (true, jsonb_build_object(
  'mode', 'provider_execution_unavailable',
  'accepted', true,
  'queued', true,
  'attempted', false,
  'executed', false,
  'outcome_unknown', false,
  'reconciled', false,
  'real_external_call_executed', false,
  'sync_executed', false
));

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_legacy_tag_sync_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.legacy_tag_sync_receipts WHERE id = NEW.id AND state = 'queued') THEN
    RAISE EXCEPTION 'legacy tag sync receipt must queue in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER legacy_tag_sync_receipts_queue_before_commit
AFTER INSERT OR UPDATE ON public.legacy_tag_sync_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_legacy_tag_sync_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_legacy_tag_live_mutation_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.legacy_tag_live_mutation_receipts WHERE id = NEW.id AND state = 'queued') THEN
    RAISE EXCEPTION 'legacy tag live mutation receipt must queue in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER legacy_tag_live_mutation_receipts_queue_before_commit
AFTER INSERT OR UPDATE ON public.legacy_tag_live_mutation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_legacy_tag_live_mutation_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_legacy_tag_sync_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'queued' THEN
    RAISE EXCEPTION 'queued legacy tag execution receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'queued' OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.kind IS DISTINCT FROM OLD.kind
     OR NEW.trace_id IS DISTINCT FROM OLD.trace_id
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid legacy tag execution receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_legacy_tag_live_mutation_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'queued' THEN
    RAISE EXCEPTION 'queued legacy tag execution receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'queued' OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.payload IS DISTINCT FROM OLD.payload
     OR NEW.trace_id IS DISTINCT FROM OLD.trace_id
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid legacy tag execution receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER legacy_tag_sync_receipts_transition
BEFORE UPDATE OR DELETE ON public.legacy_tag_sync_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_legacy_tag_sync_receipt_transition_valid();
CREATE TRIGGER legacy_tag_live_mutation_receipts_transition
BEFORE UPDATE OR DELETE ON public.legacy_tag_live_mutation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_legacy_tag_live_mutation_receipt_transition_valid();

-- +goose Down
DROP TRIGGER legacy_tag_live_mutation_receipts_transition ON public.legacy_tag_live_mutation_receipts;
DROP TRIGGER legacy_tag_sync_receipts_transition ON public.legacy_tag_sync_receipts;
DROP FUNCTION IF EXISTS public.aicrm_legacy_tag_live_mutation_receipt_transition_valid();
DROP FUNCTION IF EXISTS public.aicrm_legacy_tag_sync_receipt_transition_valid();
DROP FUNCTION IF EXISTS public.aicrm_legacy_tag_execution_receipt_transition_valid();
DROP TRIGGER legacy_tag_live_mutation_receipts_queue_before_commit ON public.legacy_tag_live_mutation_receipts;
DROP FUNCTION public.aicrm_reject_incomplete_legacy_tag_live_mutation_receipt();
DROP TRIGGER legacy_tag_sync_receipts_queue_before_commit ON public.legacy_tag_sync_receipts;
DROP FUNCTION public.aicrm_reject_incomplete_legacy_tag_sync_receipt();
DROP TABLE public.legacy_tag_execution_status;
DROP TABLE public.legacy_tag_live_mutation_receipts;
DROP TABLE public.legacy_tag_sync_receipts;
