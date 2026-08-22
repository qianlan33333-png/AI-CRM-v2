-- +goose Up
-- Attachment Library stores only tenant-local, private PDF facts. It creates
-- no provider media id, public URL, share token, remote fetch, or scanner job.
CREATE TABLE public.media_attachments (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  file_name TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  checksum BYTEA NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version BIGINT NOT NULL DEFAULT 1,
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_attachments_name CHECK (
    name <> '' AND btrim(name) = name AND char_length(name) <= 200
  ),
  CONSTRAINT media_attachments_file_name CHECK (
    file_name <> '' AND btrim(file_name) = file_name AND char_length(file_name) <= 255
    AND file_name !~ '[\\/[:cntrl:]]'
  ),
  CONSTRAINT media_attachments_mime_type CHECK (mime_type = 'application/pdf'),
  CONSTRAINT media_attachments_file_size CHECK (file_size > 0 AND file_size <= 10485760),
  CONSTRAINT media_attachments_checksum CHECK (octet_length(checksum) = 32),
  CONSTRAINT media_attachments_description CHECK (
    btrim(description) = description AND char_length(description) <= 10000
  ),
  CONSTRAINT media_attachments_tags CHECK (
    jsonb_typeof(tags) = 'array' AND jsonb_array_length(tags) <= 50
  ),
  CONSTRAINT media_attachments_version CHECK (version > 0),
  CONSTRAINT media_attachments_actors CHECK (created_by > 0 AND updated_by > 0),
  CONSTRAINT media_attachments_timestamps CHECK (updated_at >= created_at)
);

CREATE TABLE public.media_attachment_blobs (
  attachment_id BIGINT PRIMARY KEY REFERENCES public.media_attachments(id) ON DELETE CASCADE,
  content BYTEA NOT NULL,
  checksum BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_attachment_blobs_content CHECK (
    octet_length(content) > 0 AND octet_length(content) <= 10485760
  ),
  CONSTRAINT media_attachment_blobs_checksum CHECK (octet_length(checksum) = 32)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_attachment_blob_required()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  attachment_key BIGINT;
BEGIN
  IF TG_TABLE_NAME = 'media_attachments' THEN
    attachment_key := NEW.id;
  ELSIF TG_OP = 'DELETE' THEN
    attachment_key := OLD.attachment_id;
  ELSE
    attachment_key := NEW.attachment_id;
  END IF;
  IF EXISTS (SELECT 1 FROM public.media_attachments WHERE id = attachment_key)
     AND NOT EXISTS (SELECT 1 FROM public.media_attachment_blobs WHERE attachment_id = attachment_key) THEN
    RAISE EXCEPTION 'media attachment must retain exactly one local blob' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- A deferred check permits the canonical transaction to insert metadata and
-- its blob together, while rejecting any committed blobless attachment.
CREATE CONSTRAINT TRIGGER media_attachments_blob_required
AFTER INSERT ON public.media_attachments DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_attachment_blob_required();

CREATE CONSTRAINT TRIGGER media_attachment_blobs_continuity
AFTER DELETE ON public.media_attachment_blobs DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_attachment_blob_required();

CREATE TABLE public.media_attachment_mutation_receipts (
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
  CONSTRAINT media_attachment_receipts_operation CHECK (operation IN ('upload', 'update', 'delete')),
  CONSTRAINT media_attachment_receipts_actor CHECK (
    actor_scope ~ '^admin:[1-9][0-9]*$' AND char_length(actor_scope) <= 200
  ),
  CONSTRAINT media_attachment_receipts_business CHECK (
    (operation = 'upload' AND business_key = 'upload')
    OR (operation IN ('update', 'delete') AND business_key ~ '^[1-9][0-9]*$')
  ),
  CONSTRAINT media_attachment_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT media_attachment_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT media_attachment_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT media_attachment_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, business_key, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_media_attachment_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.media_attachment_mutation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'media attachment receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER media_attachment_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.media_attachment_mutation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_media_attachment_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_attachment_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed media attachment receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.business_key IS DISTINCT FROM OLD.business_key
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid media attachment receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_attachment_receipts_transition
BEFORE UPDATE OR DELETE ON public.media_attachment_mutation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_attachment_receipt_transition_valid();

CREATE INDEX media_attachments_enabled_updated_id_idx
  ON public.media_attachments (enabled, updated_at DESC, id DESC);
CREATE INDEX media_attachments_name_id_idx
  ON public.media_attachments (name ASC, id ASC);
CREATE INDEX radar_links_cover_image_id_idx
  ON public.radar_links (cover_image_id) WHERE cover_image_id IS NOT NULL;
CREATE INDEX radar_links_attachment_id_idx
  ON public.radar_links (attachment_id) WHERE attachment_id IS NOT NULL;

-- +goose Down
LOCK TABLE public.media_attachments, public.media_attachment_blobs, public.media_attachment_mutation_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.media_attachments)
     OR EXISTS (SELECT 1 FROM public.media_attachment_blobs)
     OR EXISTS (SELECT 1 FROM public.media_attachment_mutation_receipts) THEN
    RAISE EXCEPTION 'cannot roll back media attachment facts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP INDEX public.radar_links_attachment_id_idx;
DROP INDEX public.radar_links_cover_image_id_idx;
DROP INDEX public.media_attachments_name_id_idx;
DROP INDEX public.media_attachments_enabled_updated_id_idx;
DROP TRIGGER media_attachment_receipts_transition ON public.media_attachment_mutation_receipts;
DROP FUNCTION public.aicrm_media_attachment_receipt_transition_valid();
DROP TRIGGER media_attachment_receipts_complete_before_commit ON public.media_attachment_mutation_receipts;
DROP FUNCTION public.aicrm_reject_incomplete_media_attachment_receipt();
DROP TABLE public.media_attachment_mutation_receipts;
DROP TRIGGER media_attachment_blobs_continuity ON public.media_attachment_blobs;
DROP TRIGGER media_attachments_blob_required ON public.media_attachments;
DROP FUNCTION public.aicrm_media_attachment_blob_required();
DROP TABLE public.media_attachment_blobs;
DROP TABLE public.media_attachments;
