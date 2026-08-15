-- +goose Up
CREATE TABLE public.media_group_invites (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  join_url TEXT NOT NULL,
  cover_image_id BIGINT REFERENCES public.media_images(id) ON DELETE RESTRICT,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  archived_at TIMESTAMPTZ,
  CONSTRAINT media_group_invites_name CHECK (name <> ''),
  CONSTRAINT media_group_invites_title CHECK (octet_length(title) BETWEEN 1 AND 128),
  CONSTRAINT media_group_invites_description CHECK (octet_length(description) <= 512),
  CONSTRAINT media_group_invites_join_url CHECK (
    octet_length(join_url) <= 2048 AND join_url ~ '^https://work[.]weixin[.]qq[.]com/gm/[^?#]+$'
  ),
  CONSTRAINT media_group_invites_cover CHECK (cover_image_id IS NULL OR cover_image_id > 0),
  CONSTRAINT media_group_invites_actors CHECK (created_by > 0 AND updated_by > 0),
  CONSTRAINT media_group_invites_version CHECK (version > 0),
  CONSTRAINT media_group_invites_archive CHECK (archived_at IS NULL OR enabled = FALSE)
);

CREATE INDEX media_group_invites_active_id_idx
ON public.media_group_invites (id DESC) WHERE archived_at IS NULL;

CREATE INDEX media_group_invites_active_enabled_id_idx
ON public.media_group_invites (enabled, id DESC) WHERE archived_at IS NULL;

CREATE TABLE public.media_group_invite_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  business_key TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT media_group_invite_receipts_operation CHECK (operation IN ('create', 'update', 'archive')),
  CONSTRAINT media_group_invite_receipts_actor CHECK (actor_scope ~ '^admin:[1-9][0-9]*$' AND char_length(actor_scope) <= 200),
  CONSTRAINT media_group_invite_receipts_business CHECK (business_key = 'create' OR business_key ~ '^[1-9][0-9]*$'),
  CONSTRAINT media_group_invite_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT media_group_invite_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT media_group_invite_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT media_group_invite_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, business_key, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_media_group_invite_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.media_group_invite_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'media group invite receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER media_group_invite_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.media_group_invite_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_media_group_invite_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_group_invite_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed media group invite receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.business_key IS DISTINCT FROM OLD.business_key
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid media group invite receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_group_invite_receipts_transition
BEFORE UPDATE OR DELETE ON public.media_group_invite_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_group_invite_receipt_transition_valid();

-- +goose Down
DROP TRIGGER media_group_invite_receipts_transition ON public.media_group_invite_operation_receipts;
DROP TRIGGER media_group_invite_receipts_complete_before_commit ON public.media_group_invite_operation_receipts;
DROP TABLE public.media_group_invite_operation_receipts;
DROP TABLE public.media_group_invites;
DROP FUNCTION public.aicrm_media_group_invite_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_media_group_invite_receipt();
