-- +goose Up
-- Group Ops owns only immutable execution intent and safe Provider outcome
-- projections. Provider credentials, request bodies, responses, and delivery
-- claims never enter these tables.
ALTER TABLE public.external_effects DROP CONSTRAINT external_effects_owner_check;
ALTER TABLE public.external_effects ADD CONSTRAINT external_effects_owner_check
  CHECK (owner IN ('campaign','contact','outbound','wecom','survey','audience','order','group_ops'));
ALTER TABLE public.external_effects DROP CONSTRAINT external_effects_kind_check;
ALTER TABLE public.external_effects ADD CONSTRAINT external_effects_kind_check
  CHECK (kind IN ('campaign_dispatch','campaign_group_announcement','contact_touch','outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync','survey_webhook','audience_webhook','order_payment_prepay','order_payment_capture','order_refund','group_ops_broadcast'));

ALTER TABLE public.group_ops_plan_nodes
  ADD COLUMN material_reference TEXT NOT NULL DEFAULT '';
ALTER TABLE public.group_ops_plan_nodes
  ADD CONSTRAINT group_ops_plan_nodes_material_reference CHECK (
    material_reference = '' OR (
      material_reference ~ '^[A-Za-z0-9._:-]{1,128}$'
      AND position('://' IN material_reference) = 0
    )
  );

CREATE TABLE public.group_ops_directory_groups (
  chat_reference TEXT PRIMARY KEY,
  owner_staff_id BIGINT NOT NULL CHECK (owner_staff_id > 0),
  display_name TEXT NOT NULL,
  member_count INTEGER NOT NULL CHECK (member_count >= 0),
  source_digest TEXT NOT NULL CHECK (source_digest ~ '^sha256:[0-9a-f]{64}$'),
  refreshed_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT group_ops_directory_groups_reference CHECK (
    chat_reference ~ '^[A-Za-z0-9._:-]{1,128}$'
    AND position('://' IN chat_reference) = 0
  ),
  CONSTRAINT group_ops_directory_groups_name CHECK (
    btrim(display_name) = display_name AND display_name <> '' AND char_length(display_name) <= 128
  )
);
CREATE INDEX group_ops_directory_groups_owner_idx
  ON public.group_ops_directory_groups(owner_staff_id, refreshed_at DESC, chat_reference);

CREATE TABLE public.group_ops_directory_refresh_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  refresh_kind TEXT NOT NULL CHECK (refresh_kind IN ('members','groups')),
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  owner_staff_id BIGINT CHECK (owner_staff_id IS NULL OR owner_staff_id > 0),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  snapshot_digest TEXT NOT NULL CHECK (snapshot_digest ~ '^sha256:[0-9a-f]{64}$'),
  item_count INTEGER NOT NULL CHECK (item_count >= 0),
  provider_read_executed BOOLEAN NOT NULL DEFAULT FALSE,
  refreshed_at TIMESTAMPTZ NOT NULL,
  UNIQUE(refresh_kind, actor_id, key_digest)
);

CREATE TABLE public.group_ops_runs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_plans(id) ON DELETE RESTRICT,
  trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('run_due','broadcast','webhook')),
  source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest) = 32),
  plan_revision BIGINT NOT NULL CHECK (plan_revision > 0),
  scheduled_for TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ NOT NULL,
  accepted_by TEXT NOT NULL CHECK (accepted_by ~ '^(admin:[1-9][0-9]*|service:[A-Za-z0-9._:-]{1,128}|webhook:[A-Za-z0-9._:-]{1,128})$'),
  UNIQUE(plan_id, trigger_kind, source_key_digest)
);
CREATE INDEX group_ops_runs_plan_idx ON public.group_ops_runs(plan_id, accepted_at DESC, id DESC);

CREATE TABLE public.group_ops_executions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES public.group_ops_runs(id) ON DELETE RESTRICT,
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_plans(id) ON DELETE RESTRICT,
  node_id BIGINT NOT NULL REFERENCES public.group_ops_plan_nodes(id) ON DELETE RESTRICT,
  plan_revision BIGINT NOT NULL CHECK (plan_revision > 0),
  node_position INTEGER NOT NULL CHECK (node_position > 0),
  target_reference TEXT NOT NULL,
  target_digest TEXT NOT NULL CHECK (target_digest ~ '^sha256:[0-9a-f]{64}$'),
  content_snapshot JSONB NOT NULL,
  content_digest TEXT NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
  material_snapshot JSONB NOT NULL,
  material_digest TEXT NOT NULL CHECK (material_digest ~ '^sha256:[0-9a-f]{64}$'),
  execution_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(execution_key_digest) = 32),
  external_effect_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  state TEXT NOT NULL DEFAULT 'accepted' CHECK (state IN ('accepted','provider_accepted','delivery_proven','outcome_unknown','reconciled','final_failed')),
  provider_accepted BOOLEAN NOT NULL DEFAULT FALSE,
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE,
  provider_receipt_digest TEXT CHECK (provider_receipt_digest IS NULL OR provider_receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  reconciliation_evidence_digest TEXT CHECK (reconciliation_evidence_digest IS NULL OR reconciliation_evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT group_ops_executions_target_reference CHECK (
    target_reference ~ '^[A-Za-z0-9._:-]{1,128}$'
    AND position('://' IN target_reference) = 0
  ),
  CONSTRAINT group_ops_executions_snapshots CHECK (
    jsonb_typeof(content_snapshot) = 'object' AND jsonb_typeof(material_snapshot) = 'object'
  ),
  CONSTRAINT group_ops_executions_outcome CHECK (
    (state = 'accepted' AND NOT provider_accepted AND NOT delivery_proven AND provider_receipt_digest IS NULL AND reconciliation_evidence_digest IS NULL AND attempt_count = 0)
    OR (state = 'provider_accepted' AND provider_accepted AND NOT delivery_proven AND provider_receipt_digest IS NOT NULL AND reconciliation_evidence_digest IS NULL AND attempt_count > 0)
    OR (state = 'delivery_proven' AND provider_accepted AND delivery_proven AND provider_receipt_digest IS NOT NULL AND reconciliation_evidence_digest IS NULL AND attempt_count > 0)
    OR (state = 'outcome_unknown' AND NOT delivery_proven AND provider_receipt_digest IS NOT NULL AND reconciliation_evidence_digest IS NULL AND attempt_count > 0)
    OR (state = 'reconciled' AND (NOT delivery_proven OR provider_accepted) AND provider_receipt_digest IS NOT NULL AND reconciliation_evidence_digest IS NOT NULL AND attempt_count > 0)
    OR (state = 'final_failed' AND NOT delivery_proven AND provider_receipt_digest IS NOT NULL AND reconciliation_evidence_digest IS NULL AND attempt_count > 0)
  ),
  CONSTRAINT group_ops_executions_timestamps CHECK (updated_at >= created_at),
  UNIQUE(run_id, node_id, target_reference)
);
CREATE INDEX group_ops_executions_plan_idx
  ON public.group_ops_executions(plan_id, created_at DESC, id DESC);
CREATE INDEX group_ops_executions_state_idx
  ON public.group_ops_executions(state, updated_at DESC, id DESC);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_group_ops_runtime_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'group ops runtime facts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF TG_TABLE_NAME = 'group_ops_runs' THEN
    RAISE EXCEPTION 'group ops run facts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.run_id IS DISTINCT FROM OLD.run_id
     OR NEW.plan_id IS DISTINCT FROM OLD.plan_id OR NEW.node_id IS DISTINCT FROM OLD.node_id
     OR NEW.plan_revision IS DISTINCT FROM OLD.plan_revision
     OR NEW.node_position IS DISTINCT FROM OLD.node_position
     OR NEW.target_reference IS DISTINCT FROM OLD.target_reference
     OR NEW.target_digest IS DISTINCT FROM OLD.target_digest
     OR NEW.content_snapshot IS DISTINCT FROM OLD.content_snapshot
     OR NEW.content_digest IS DISTINCT FROM OLD.content_digest
     OR NEW.material_snapshot IS DISTINCT FROM OLD.material_snapshot
     OR NEW.material_digest IS DISTINCT FROM OLD.material_digest
     OR NEW.execution_key_digest IS DISTINCT FROM OLD.execution_key_digest
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'group ops execution snapshots are immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state IN ('delivery_proven','reconciled','final_failed')
     OR (OLD.state = 'accepted' AND NEW.state NOT IN ('provider_accepted','delivery_proven','outcome_unknown','final_failed'))
     OR (OLD.state = 'provider_accepted' AND NEW.state NOT IN ('delivery_proven','outcome_unknown','final_failed'))
     OR (OLD.state = 'outcome_unknown' AND NEW.state <> 'reconciled') THEN
    RAISE EXCEPTION 'invalid group ops execution transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER group_ops_runs_guard
BEFORE UPDATE OR DELETE ON public.group_ops_runs
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_runtime_guard();
CREATE TRIGGER group_ops_executions_guard
BEFORE UPDATE OR DELETE ON public.group_ops_executions
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_runtime_guard();

-- +goose Down
LOCK TABLE public.group_ops_directory_groups, public.group_ops_directory_refresh_receipts,
  public.group_ops_runs, public.group_ops_executions IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.group_ops_directory_groups)
     OR EXISTS (SELECT 1 FROM public.group_ops_directory_refresh_receipts)
     OR EXISTS (SELECT 1 FROM public.group_ops_runs)
     OR EXISTS (SELECT 1 FROM public.group_ops_executions)
     OR EXISTS (SELECT 1 FROM public.external_effects WHERE owner = 'group_ops' OR kind = 'group_ops_broadcast') THEN
    RAISE EXCEPTION 'cannot roll back populated group ops runtime facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER group_ops_executions_guard ON public.group_ops_executions;
DROP TRIGGER group_ops_runs_guard ON public.group_ops_runs;
DROP FUNCTION public.aicrm_group_ops_runtime_guard();
DROP TABLE public.group_ops_executions;
DROP TABLE public.group_ops_runs;
DROP TABLE public.group_ops_directory_refresh_receipts;
DROP TABLE public.group_ops_directory_groups;
ALTER TABLE public.group_ops_plan_nodes DROP CONSTRAINT group_ops_plan_nodes_material_reference;
ALTER TABLE public.group_ops_plan_nodes DROP COLUMN material_reference;
ALTER TABLE public.external_effects DROP CONSTRAINT external_effects_kind_check;
ALTER TABLE public.external_effects ADD CONSTRAINT external_effects_kind_check
  CHECK (kind IN ('campaign_dispatch','campaign_group_announcement','contact_touch','outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync','survey_webhook','audience_webhook','order_payment_prepay','order_payment_capture','order_refund'));
ALTER TABLE public.external_effects DROP CONSTRAINT external_effects_owner_check;
ALTER TABLE public.external_effects ADD CONSTRAINT external_effects_owner_check
  CHECK (owner IN ('campaign','contact','outbound','wecom','survey','audience','order'));
