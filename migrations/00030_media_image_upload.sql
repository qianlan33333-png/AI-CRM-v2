-- +goose Up
CREATE TABLE public.media_images (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  file_name TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  width INTEGER NOT NULL,
  height INTEGER NOT NULL,
  checksum BYTEA NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  created_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_images_name CHECK (char_length(name) <= 200),
  CONSTRAINT media_images_file_name CHECK (file_name <> '' AND char_length(file_name) <= 255 AND file_name !~ '[\\/[:cntrl:]]'),
  CONSTRAINT media_images_mime_type CHECK (mime_type IN ('image/png', 'image/jpeg', 'image/gif')),
  CONSTRAINT media_images_file_size CHECK (file_size > 0 AND file_size <= 10485760),
  CONSTRAINT media_images_dimensions CHECK (width > 0 AND height > 0 AND width <= 10000 AND height <= 10000 AND width::bigint * height::bigint <= 40000000),
  CONSTRAINT media_images_checksum CHECK (octet_length(checksum) = 32),
  CONSTRAINT media_images_description CHECK (char_length(description) <= 10000),
  CONSTRAINT media_images_tags CHECK (char_length(tags) <= 10000),
  CONSTRAINT media_images_category CHECK (char_length(category) <= 200),
  CONSTRAINT media_images_created_by CHECK (created_by > 0)
);

CREATE TABLE public.media_image_blobs (
  image_id BIGINT PRIMARY KEY REFERENCES public.media_images(id) ON DELETE CASCADE,
  content BYTEA NOT NULL,
  checksum BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_image_blobs_content CHECK (octet_length(content) > 0 AND octet_length(content) <= 10485760),
  CONSTRAINT media_image_blobs_checksum CHECK (octet_length(checksum) = 32)
);

CREATE TABLE public.media_image_upload_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT media_image_upload_receipts_operation CHECK (operation = 'upload'),
  CONSTRAINT media_image_upload_receipts_actor CHECK (actor_scope ~ '^admin:[1-9][0-9]*$' AND char_length(actor_scope) <= 200),
  CONSTRAINT media_image_upload_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT media_image_upload_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT media_image_upload_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT media_image_upload_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_media_image_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.media_image_upload_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'media image upload receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER media_image_upload_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.media_image_upload_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_media_image_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_image_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed media image upload receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid media image upload receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_image_upload_receipts_transition
BEFORE UPDATE OR DELETE ON public.media_image_upload_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_image_receipt_transition_valid();

-- +goose Down
DROP TRIGGER media_image_upload_receipts_transition ON public.media_image_upload_receipts;
DROP TRIGGER media_image_upload_receipts_complete_before_commit ON public.media_image_upload_receipts;
DROP TABLE public.media_image_upload_receipts;
DROP TABLE public.media_image_blobs;
DROP TABLE public.media_images;
DROP FUNCTION public.aicrm_media_image_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_media_image_receipt();
