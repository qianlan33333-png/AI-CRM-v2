-- +goose Up
-- Contact owns the local channel resource; this migration performs no provider call.
UPDATE public.channels
SET code = 'legacy-channel-' || id::text
WHERE code IS NULL OR btrim(code) = '';
UPDATE public.channels
SET config = jsonb_build_object(
  'schema_version', 1, 'channel_type', 'qrcode', 'carrier_type', 'qrcode',
  'channel_code', code, 'channel_name', name, 'status', 'active',
  'scene_value', '', 'qr_url', '', 'owner_staff_id', '', 'customer_channel', '',
  'link_url', '', 'final_url', '', 'welcome_message', '',
  'welcome_image_library_ids', '[]'::jsonb, 'welcome_miniprogram_library_ids', '[]'::jsonb,
  'welcome_attachment_library_ids', '[]'::jsonb, 'welcome_group_invite_library_ids', '[]'::jsonb,
  'auto_accept_friend', false, 'entry_tag_id', '', 'entry_tag_name', '', 'entry_tag_group_name', '',
  'assignment_mode', 'single_owner', 'assignment_strategy', 'ratio',
  'overflow_policy', 'least_loaded', 'assignment_config_json', config
);
ALTER TABLE public.channels
  ALTER COLUMN code SET NOT NULL,
  ADD COLUMN status TEXT NOT NULL DEFAULT 'active',
  ADD COLUMN created_by BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN updated_by BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD CONSTRAINT channels_code CHECK (btrim(code) = code AND code <> '' AND char_length(code) <= 200),
  ADD CONSTRAINT channels_name CHECK (btrim(name) = name AND name <> '' AND char_length(name) <= 200),
  ADD CONSTRAINT channels_status CHECK (status IN ('active', 'inactive', 'archived')),
  ADD CONSTRAINT channels_projection_v1 CHECK (jsonb_typeof(config) = 'object' AND config ? 'schema_version'),
  ADD CONSTRAINT channels_actors CHECK (created_by > 0 AND updated_by > 0),
  ADD CONSTRAINT channels_timestamps CHECK (updated_at >= created_at);
CREATE INDEX channels_status_updated_id_idx ON public.channels (status, updated_at DESC, id DESC);
CREATE INDEX channels_updated_id_idx ON public.channels (updated_at DESC, id DESC);

CREATE TABLE public.channel_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT channel_operation_receipts_operation CHECK (operation IN ('create', 'update')),
  CONSTRAINT channel_operation_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT channel_operation_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT channel_operation_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT channel_operation_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT channel_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_channel_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.channel_operation_receipts WHERE id = NEW.id AND state = 'completed') THEN
    RAISE EXCEPTION 'channel operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER channel_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.channel_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_channel_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_channel_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed channel operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid channel operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER channel_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.channel_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_channel_receipt_transition_valid();

-- +goose Down
DROP TRIGGER channel_operation_receipts_transition ON public.channel_operation_receipts;
DROP TRIGGER channel_operation_receipts_complete_before_commit ON public.channel_operation_receipts;
DROP TABLE public.channel_operation_receipts;
DROP INDEX public.channels_updated_id_idx;
DROP INDEX public.channels_status_updated_id_idx;
ALTER TABLE public.channels
  DROP CONSTRAINT channels_timestamps,
  DROP CONSTRAINT channels_actors,
  DROP CONSTRAINT channels_projection_v1,
  DROP CONSTRAINT channels_status,
  DROP CONSTRAINT channels_name,
  DROP CONSTRAINT channels_code,
  DROP COLUMN updated_at,
  DROP COLUMN updated_by,
  DROP COLUMN created_by,
  DROP COLUMN status;
ALTER TABLE public.channels ALTER COLUMN code DROP NOT NULL;
DROP FUNCTION public.aicrm_channel_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_channel_receipt();
