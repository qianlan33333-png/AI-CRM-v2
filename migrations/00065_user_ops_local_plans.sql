-- +goose Up
-- Local-only User Ops facts. customer_id is a canonical OneID value checked
-- by a transaction-bound composition reader; contact keeps customers ownership.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_user_ops_content_snapshot_valid(snapshot JSONB, stored_digest BYTEA)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE SET search_path = pg_catalog AS $$
DECLARE
  key TEXT;
  item JSONB;
  maximum INTEGER;
  digest_text TEXT;
BEGIN
  IF jsonb_typeof(snapshot) <> 'object' OR stored_digest IS NULL OR octet_length(stored_digest) <> 32
    OR snapshot - 'text' - 'image_library_ids' - 'miniprogram_library_ids' - 'attachment_library_ids' - 'content_digest' <> '{}'::jsonb
    OR NOT snapshot ?& ARRAY['text', 'image_library_ids', 'miniprogram_library_ids', 'attachment_library_ids', 'content_digest']
    OR jsonb_typeof(snapshot -> 'text') <> 'string' OR char_length(snapshot ->> 'text') > 4000
    OR jsonb_typeof(snapshot -> 'content_digest') <> 'string' THEN
    RETURN false;
  END IF;
  digest_text := snapshot ->> 'content_digest';
  IF digest_text !~ '^[0-9a-f]{64}$' OR decode(digest_text, 'hex') <> stored_digest THEN
    RETURN false;
  END IF;
  FOR key, maximum IN SELECT * FROM (VALUES ('image_library_ids', 3), ('miniprogram_library_ids', 1), ('attachment_library_ids', 9)) AS limits(key, maximum) LOOP
    IF jsonb_typeof(snapshot -> key) <> 'array' OR jsonb_array_length(snapshot -> key) > maximum THEN
      RETURN false;
    END IF;
    FOR item IN SELECT value FROM jsonb_array_elements(snapshot -> key) LOOP
      IF jsonb_typeof(item) <> 'number' OR item::text !~ '^[1-9][0-9]{0,18}$' OR (item #>> '{}')::numeric > 9223372036854775807 THEN
        RETURN false;
      END IF;
    END LOOP;
  END LOOP;
  RETURN jsonb_array_length(snapshot -> 'image_library_ids') + jsonb_array_length(snapshot -> 'miniprogram_library_ids') + jsonb_array_length(snapshot -> 'attachment_library_ids') <= 9;
END;
$$;
-- +goose StatementEnd
CREATE TABLE public.user_ops_dnd (
  customer_id BIGINT PRIMARY KEY CHECK (customer_id > 0),
  reason TEXT NOT NULL CHECK (btrim(reason) = reason AND reason <> '' AND char_length(reason) <= 500),
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);

CREATE TABLE public.user_ops_local_plans (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  state TEXT NOT NULL CHECK (state IN ('draft', 'pending_review')),
  -- This is a closed local snapshot, not a generic provider payload. Keeping
  -- the shape in SQL prevents a later caller from smuggling URLs or external
  -- identifiers into a reviewable local plan.
  content_snapshot JSONB NOT NULL,
  content_digest BYTEA NOT NULL CHECK (aicrm_user_ops_content_snapshot_valid(content_snapshot, content_digest)),
  target_digest BYTEA NOT NULL CHECK (octet_length(target_digest) = 32),
  target_count INTEGER NOT NULL CHECK (target_count BETWEEN 1 AND 1000),
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_by BIGINT NOT NULL CHECK (created_by > 0),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);

CREATE TABLE public.user_ops_local_plan_targets (
  plan_id BIGINT NOT NULL REFERENCES public.user_ops_local_plans(id) ON DELETE RESTRICT,
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  PRIMARY KEY (plan_id, customer_id)
);

CREATE TABLE public.user_ops_send_records (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  plan_id BIGINT NOT NULL REFERENCES public.user_ops_local_plans(id) ON DELETE RESTRICT,
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  technical_status TEXT NOT NULL CHECK (technical_status IN ('draft', 'pending_review', 'not_dispatched')),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
  UNIQUE (plan_id, customer_id)
);
CREATE INDEX user_ops_send_records_plan_page_idx ON public.user_ops_send_records (plan_id, id DESC);
CREATE INDEX user_ops_local_plan_targets_customer_idx ON public.user_ops_local_plan_targets (customer_id, plan_id);
CREATE INDEX user_ops_local_plans_state_updated_idx ON public.user_ops_local_plans (state, updated_at DESC, id DESC);
CREATE INDEX user_ops_dnd_updated_idx ON public.user_ops_dnd (updated_at DESC, customer_id DESC);

CREATE TABLE public.user_ops_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN ('dnd_set', 'dnd_clear', 'local_plan_create')),
  actor_scope TEXT NOT NULL CHECK (actor_scope ~ '^user_ops:actor:[1-9][0-9]*$'),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved', 'completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT user_ops_receipts_completion CHECK (
    (state = 'reserved' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_user_ops_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.user_ops_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'user ops receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER user_ops_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.user_ops_operation_receipts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION public.aicrm_user_ops_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_user_ops_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed user ops receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.operation IS DISTINCT FROM OLD.operation OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope
    OR NEW.key_digest IS DISTINCT FROM OLD.key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
    OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.state <> 'completed'
    OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid user ops receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER user_ops_receipts_transition
BEFORE UPDATE OR DELETE ON public.user_ops_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_user_ops_receipt_transition_valid();

-- +goose Down
LOCK TABLE public.user_ops_dnd, public.user_ops_local_plans, public.user_ops_local_plan_targets,
  public.user_ops_send_records, public.user_ops_operation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.user_ops_dnd) OR EXISTS (SELECT 1 FROM public.user_ops_local_plans)
    OR EXISTS (SELECT 1 FROM public.user_ops_local_plan_targets) OR EXISTS (SELECT 1 FROM public.user_ops_send_records)
    OR EXISTS (SELECT 1 FROM public.user_ops_operation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back user ops facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER user_ops_receipts_transition ON public.user_ops_operation_receipts;
DROP FUNCTION public.aicrm_user_ops_receipt_transition_valid();
DROP TRIGGER user_ops_receipts_complete_before_commit ON public.user_ops_operation_receipts;
DROP FUNCTION public.aicrm_user_ops_receipt_complete_before_commit();
DROP TABLE public.user_ops_operation_receipts;
DROP TABLE public.user_ops_send_records;
DROP TABLE public.user_ops_local_plan_targets;
DROP TABLE public.user_ops_local_plans;
DROP TABLE public.user_ops_dnd;
DROP FUNCTION public.aicrm_user_ops_content_snapshot_valid(JSONB, BYTEA);
