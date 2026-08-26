-- +goose Up
-- A WeCom group-message task acceptance is not a delivery receipt. Keep the
-- exact Provider identifiers in the owner-only evidence ledger, while the
-- Group Ops execution and EER boundaries retain digests only.
ALTER TABLE public.group_ops_executions
  ADD COLUMN sender_userid_snapshot TEXT;
ALTER TABLE public.group_ops_executions
  ADD CONSTRAINT group_ops_executions_sender_snapshot CHECK (
    sender_userid_snapshot IS NULL
    OR (sender_userid_snapshot ~ '^[^[:space:]]{1,128}$')
  );

-- 00085's guard predates sender snapshots. Recreate it with the same closed
-- transition rules plus immutable sender evidence; existing rows remain NULL
-- and fail closed in the dispatch reader.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_group_ops_runtime_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN RAISE EXCEPTION 'group ops runtime facts cannot be deleted' USING ERRCODE = '55000'; END IF;
  IF TG_TABLE_NAME = 'group_ops_runs' THEN RAISE EXCEPTION 'group ops run facts are immutable' USING ERRCODE = '55000'; END IF;
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.plan_id IS DISTINCT FROM OLD.plan_id OR NEW.node_id IS DISTINCT FROM OLD.node_id
     OR NEW.plan_revision IS DISTINCT FROM OLD.plan_revision OR NEW.node_position IS DISTINCT FROM OLD.node_position OR NEW.target_reference IS DISTINCT FROM OLD.target_reference
     OR NEW.target_digest IS DISTINCT FROM OLD.target_digest OR NEW.content_snapshot IS DISTINCT FROM OLD.content_snapshot OR NEW.content_digest IS DISTINCT FROM OLD.content_digest
     OR NEW.material_snapshot IS DISTINCT FROM OLD.material_snapshot OR NEW.material_digest IS DISTINCT FROM OLD.material_digest OR NEW.execution_key_digest IS DISTINCT FROM OLD.execution_key_digest
     OR NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id OR NEW.sender_userid_snapshot IS DISTINCT FROM OLD.sender_userid_snapshot OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'group ops execution snapshots are immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state IN ('delivery_proven','reconciled','final_failed') OR (OLD.provider_accepted AND NOT NEW.provider_accepted) OR NEW.attempt_count < OLD.attempt_count
     OR (OLD.state = 'accepted' AND NEW.state NOT IN ('provider_accepted','delivery_proven','outcome_unknown','final_failed'))
     OR (OLD.state = 'provider_accepted' AND NEW.state NOT IN ('delivery_proven','outcome_unknown','final_failed'))
     OR (OLD.state = 'outcome_unknown' AND NEW.state <> 'reconciled') THEN
    RAISE EXCEPTION 'invalid group ops execution transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TABLE public.group_ops_wecom_group_message_receipts (
  external_effect_id BIGINT PRIMARY KEY REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  execution_id BIGINT NOT NULL UNIQUE REFERENCES public.group_ops_executions(id) ON DELETE RESTRICT,
  msgid TEXT NOT NULL,
  sender_userid TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  userid TEXT NOT NULL,
  send_status INTEGER,
  task_evidence_digest TEXT NOT NULL CHECK (task_evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  delivery_evidence_digest TEXT CHECK (delivery_evidence_digest IS NULL OR delivery_evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT group_ops_wecom_group_message_receipts_identifiers CHECK (
    msgid ~ '^[^[:space:]]+$'
    AND char_length(msgid) BETWEEN 1 AND 1024
    AND sender_userid ~ '^[^[:space:]]{1,128}$'
    AND chat_id ~ '^[^[:space:]]+$'
    AND char_length(chat_id) BETWEEN 1 AND 1024
    AND userid ~ '^[^[:space:]]{1,128}$'
  ),
  CONSTRAINT group_ops_wecom_group_message_receipts_delivery CHECK (
    (send_status IS NULL AND delivery_evidence_digest IS NULL)
    OR (send_status = 1 AND delivery_evidence_digest IS NOT NULL)
  ),
  CONSTRAINT group_ops_wecom_group_message_receipts_timestamps CHECK (updated_at >= created_at),
  UNIQUE (msgid, sender_userid, chat_id, userid)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_group_ops_wecom_group_message_receipt_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'group ops WeCom group message receipts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF NEW.external_effect_id IS DISTINCT FROM OLD.external_effect_id
     OR NEW.execution_id IS DISTINCT FROM OLD.execution_id
     OR NEW.msgid IS DISTINCT FROM OLD.msgid
     OR NEW.sender_userid IS DISTINCT FROM OLD.sender_userid
     OR NEW.chat_id IS DISTINCT FROM OLD.chat_id
     OR NEW.userid IS DISTINCT FROM OLD.userid
     OR NEW.task_evidence_digest IS DISTINCT FROM OLD.task_evidence_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.updated_at < OLD.updated_at THEN
    RAISE EXCEPTION 'group ops WeCom group message task receipt is immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.send_status IS NULL THEN
    IF NEW.send_status IS DISTINCT FROM 1 OR NEW.delivery_evidence_digest IS NULL THEN
      RAISE EXCEPTION 'group ops WeCom group message delivery must be set once' USING ERRCODE = '55000';
    END IF;
  ELSIF NEW.send_status IS DISTINCT FROM OLD.send_status
        OR NEW.delivery_evidence_digest IS DISTINCT FROM OLD.delivery_evidence_digest THEN
    RAISE EXCEPTION 'group ops WeCom group message delivery is immutable' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_group_ops_wecom_group_message_receipt_binding()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM public.group_ops_executions execution
    JOIN public.external_effects effect ON effect.id = execution.external_effect_id
    WHERE execution.id = NEW.execution_id
      AND execution.external_effect_id = NEW.external_effect_id
      AND effect.owner = 'group_ops'
      AND effect.kind = 'group_ops_broadcast'
  ) THEN
    RAISE EXCEPTION 'group ops WeCom group message receipt must bind its exact group ops broadcast effect' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER group_ops_wecom_group_message_receipts_guard
BEFORE UPDATE OR DELETE ON public.group_ops_wecom_group_message_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_wecom_group_message_receipt_guard();
CREATE TRIGGER group_ops_wecom_group_message_receipts_binding
BEFORE INSERT OR UPDATE ON public.group_ops_wecom_group_message_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_group_ops_wecom_group_message_receipt_binding();

-- +goose Down
LOCK TABLE public.group_ops_wecom_group_message_receipts, public.group_ops_executions IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.group_ops_wecom_group_message_receipts)
     OR EXISTS (SELECT 1 FROM public.group_ops_executions WHERE sender_userid_snapshot IS NOT NULL) THEN
    RAISE EXCEPTION 'cannot roll back populated group ops WeCom group receipts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER group_ops_wecom_group_message_receipts_binding ON public.group_ops_wecom_group_message_receipts;
DROP TRIGGER group_ops_wecom_group_message_receipts_guard ON public.group_ops_wecom_group_message_receipts;
DROP FUNCTION public.aicrm_group_ops_wecom_group_message_receipt_binding();
DROP FUNCTION public.aicrm_group_ops_wecom_group_message_receipt_guard();
DROP TABLE public.group_ops_wecom_group_message_receipts;
-- Restore 00085's guard before dropping the new column: both runtime triggers
-- remain installed and must never retain a function body that references a
-- column removed by this rollback.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_group_ops_runtime_guard()
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
     OR (OLD.provider_accepted AND NOT NEW.provider_accepted)
     OR NEW.attempt_count < OLD.attempt_count
     OR (OLD.state = 'accepted' AND NEW.state NOT IN ('provider_accepted','delivery_proven','outcome_unknown','final_failed'))
     OR (OLD.state = 'provider_accepted' AND NEW.state NOT IN ('delivery_proven','outcome_unknown','final_failed'))
     OR (OLD.state = 'outcome_unknown' AND NEW.state <> 'reconciled') THEN
    RAISE EXCEPTION 'invalid group ops execution transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
ALTER TABLE public.group_ops_executions DROP CONSTRAINT group_ops_executions_sender_snapshot;
ALTER TABLE public.group_ops_executions DROP COLUMN sender_userid_snapshot;
