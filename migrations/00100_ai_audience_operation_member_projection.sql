-- AI Audience owns its operation-member read projection. A successful
-- provider read replaces the whole snapshot in the same transaction as its
-- Audience receipt and redacted event; failed reads never mutate this table.

-- +goose Up
ALTER TABLE public.ai_audience_local_configuration_receipts
  DROP CONSTRAINT ai_audience_local_configuration_receipts_operation;
ALTER TABLE public.ai_audience_local_configuration_receipts
  ADD CONSTRAINT ai_audience_local_configuration_receipts_operation CHECK (
    operation IN (
      'automation_binding_put', 'automation_binding_delete', 'senders_put',
      'configuration_version_put', 'configuration_materialize', 'operation_members_sync'
    )
  );

CREATE TABLE public.ai_audience_operation_member_projection (
  sender_userid TEXT PRIMARY KEY,
  display_name  TEXT NOT NULL,
  synced_at     TIMESTAMPTZ NOT NULL,
  CONSTRAINT ai_audience_operation_member_projection_sender CHECK (
    sender_userid = btrim(sender_userid)
    AND sender_userid <> ''
    AND char_length(sender_userid) <= 128
  ),
  CONSTRAINT ai_audience_operation_member_projection_display_name CHECK (
    display_name = btrim(display_name)
    AND display_name <> ''
    AND char_length(display_name) <= 128
  )
);

-- +goose Down
LOCK TABLE public.ai_audience_operation_member_projection,
           public.ai_audience_local_configuration_receipts
  IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.ai_audience_operation_member_projection)
     OR EXISTS (
       SELECT 1
       FROM public.ai_audience_local_configuration_receipts
       WHERE operation = 'operation_members_sync'
     ) THEN
    RAISE EXCEPTION 'cannot roll back populated AI Audience operation-member projection facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

DROP TABLE public.ai_audience_operation_member_projection;
ALTER TABLE public.ai_audience_local_configuration_receipts
  DROP CONSTRAINT ai_audience_local_configuration_receipts_operation;
ALTER TABLE public.ai_audience_local_configuration_receipts
  ADD CONSTRAINT ai_audience_local_configuration_receipts_operation CHECK (
    operation IN ('automation_binding_put', 'automation_binding_delete', 'senders_put', 'configuration_version_put', 'configuration_materialize')
  );
