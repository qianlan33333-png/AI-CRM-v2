-- +goose Up
-- Group Ops records only local administrative facts. It has no provider,
-- runtime, group-send, webhook dispatch, or external-effect state.
CREATE TABLE public.group_ops_plans (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'paused', 'archived')),
  revision BIGINT NOT NULL CHECK (revision > 0),
  created_by BIGINT NOT NULL CHECK (created_by > 0),
  updated_by BIGINT NOT NULL CHECK (updated_by > 0),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT group_ops_plans_name CHECK (btrim(name) = name AND name <> '' AND char_length(name) <= 128),
  CONSTRAINT group_ops_plans_timestamps CHECK (updated_at >= created_at)
);
CREATE INDEX group_ops_plans_updated_idx ON public.group_ops_plans (updated_at DESC, id DESC);

CREATE TABLE public.group_ops_plan_members (
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_plans(id) ON DELETE CASCADE,
  staff_id BIGINT NOT NULL CHECK (staff_id > 0),
  PRIMARY KEY (plan_id, staff_id)
);

CREATE TABLE public.group_ops_plan_group_assets (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_plans(id) ON DELETE CASCADE,
  asset_reference TEXT NOT NULL,
  CONSTRAINT group_ops_plan_group_assets_reference CHECK (
    asset_reference ~ '^[A-Za-z0-9._:-]{1,128}$'
    AND position('://' IN asset_reference) = 0
  ),
  UNIQUE (plan_id, asset_reference)
);
CREATE INDEX group_ops_plan_group_assets_plan_idx ON public.group_ops_plan_group_assets (plan_id, asset_reference, id);

CREATE TABLE public.group_ops_plan_nodes (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_plans(id) ON DELETE CASCADE,
  position INTEGER NOT NULL CHECK (position > 0),
  kind TEXT NOT NULL CHECK (kind IN ('message', 'delay')),
  message_text TEXT NOT NULL DEFAULT '',
  delay_minutes INTEGER NOT NULL DEFAULT 0,
  CONSTRAINT group_ops_plan_nodes_content CHECK (
    (kind = 'message' AND btrim(message_text) = message_text AND message_text <> '' AND char_length(message_text) <= 1000 AND delay_minutes = 0)
    OR (kind = 'delay' AND message_text = '' AND delay_minutes BETWEEN 1 AND 10080)
  )
);
CREATE INDEX group_ops_plan_nodes_plan_idx ON public.group_ops_plan_nodes (plan_id, position, id);

CREATE TABLE public.group_ops_plan_webhook_descriptors (
  plan_id BIGINT PRIMARY KEY REFERENCES public.group_ops_plans(id) ON DELETE CASCADE,
  reference TEXT NOT NULL DEFAULT '',
  CONSTRAINT group_ops_plan_webhook_descriptors_reference CHECK (
    reference = '' OR (
      reference ~ '^[A-Za-z0-9._:-]{1,128}$'
      AND position('://' IN reference) = 0
      AND lower(reference) NOT LIKE '%secret%'
      AND lower(reference) NOT LIKE '%token%'
      AND lower(reference) NOT LIKE '%password%'
      AND lower(reference) NOT LIKE '%api_key%'
    )
  )
);

CREATE TABLE public.group_ops_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN (
    'plan_create', 'plan_update', 'plan_activate', 'plan_pause', 'plan_archive',
    'member_add', 'member_remove', 'group_asset_add', 'group_asset_remove',
    'node_add', 'node_update', 'node_remove', 'webhook_descriptor_put'
  )),
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress' CHECK (state IN ('in_progress', 'completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT group_ops_operation_receipts_actor CHECK (actor_scope ~ '^admin:[1-9][0-9]*$'),
  CONSTRAINT group_ops_operation_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT group_ops_operation_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT group_ops_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_group_ops_receipt_complete_before_commit()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.group_ops_operation_receipts WHERE id = NEW.id AND state = 'completed'
  ) THEN
    RAISE EXCEPTION 'group ops receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER group_ops_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.group_ops_operation_receipts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_receipt_complete_before_commit();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_group_ops_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'group ops receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed group ops receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.operation <> OLD.operation OR NEW.actor_scope <> OLD.actor_scope
    OR NEW.key_digest <> OLD.key_digest OR NEW.payload_digest <> OLD.payload_digest
    OR NEW.created_at <> OLD.created_at OR NEW.state <> 'completed'
    OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid group ops receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER group_ops_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.group_ops_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_receipt_transition_valid();

-- +goose Down
LOCK TABLE public.group_ops_plans, public.group_ops_operation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.group_ops_plans) OR EXISTS (SELECT 1 FROM public.group_ops_operation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back group ops facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER group_ops_operation_receipts_transition ON public.group_ops_operation_receipts;
DROP FUNCTION public.aicrm_group_ops_receipt_transition_valid();
DROP TRIGGER group_ops_operation_receipts_complete_before_commit ON public.group_ops_operation_receipts;
DROP FUNCTION public.aicrm_group_ops_receipt_complete_before_commit();
DROP TABLE public.group_ops_operation_receipts;
DROP TABLE public.group_ops_plan_webhook_descriptors;
DROP TABLE public.group_ops_plan_nodes;
DROP TABLE public.group_ops_plan_group_assets;
DROP TABLE public.group_ops_plan_members;
DROP TABLE public.group_ops_plans;
