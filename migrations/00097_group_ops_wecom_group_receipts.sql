-- +goose Up
-- A WeCom group-message task acceptance is not a delivery receipt. Keep the
-- exact Provider identifiers in the owner-only evidence ledger, while the
-- Group Ops execution and EER boundaries retain digests only.
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
    msgid ~ '^[^[:space:]]{1,1024}$'
    AND sender_userid ~ '^[^[:space:]]{1,128}$'
    AND chat_id ~ '^[^[:space:]]{1,1024}$'
    AND userid ~ '^[^[:space:]]{1,128}$'
  ),
  CONSTRAINT group_ops_wecom_group_message_receipts_delivery CHECK (
    (send_status IS NULL AND delivery_evidence_digest IS NULL)
    OR (send_status = 1 AND delivery_evidence_digest IS NOT NULL)
  ),
  CONSTRAINT group_ops_wecom_group_message_receipts_timestamps CHECK (updated_at >= created_at),
  UNIQUE (msgid, sender_userid, chat_id, userid)
);

-- +goose Down
LOCK TABLE public.group_ops_wecom_group_message_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.group_ops_wecom_group_message_receipts) THEN
    RAISE EXCEPTION 'cannot roll back populated group ops WeCom group receipts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.group_ops_wecom_group_message_receipts;
