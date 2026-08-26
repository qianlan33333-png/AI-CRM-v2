-- +goose Up
-- Sidebar owns the browser-scoped temporary-media preparation receipt. It is
-- intentionally separate from Group Ops: the resulting media_id is consumed
-- only by the caller's JSSDK callback, which has no server delivery receipt.
CREATE TABLE public.sidebar_image_temporary_media_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  image_id BIGINT NOT NULL REFERENCES public.media_images(id) ON DELETE RESTRICT,
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  state TEXT NOT NULL CHECK (state IN ('pending', 'ready', 'outcome_unknown', 'final_failed')),
  media_id TEXT,
  media_expires_at TIMESTAMPTZ,
  provider_call_dispatched BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (actor_id, customer_id, key_digest),
  CHECK (
    (state = 'ready' AND media_id IS NOT NULL AND media_expires_at IS NOT NULL AND provider_call_dispatched)
    OR (state <> 'ready' AND media_id IS NULL AND media_expires_at IS NULL)
  ),
  CHECK (media_id IS NULL OR (length(media_id) BETWEEN 1 AND 2048 AND media_id !~ '[[:space:]]')),
  CHECK (updated_at >= created_at)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_sidebar_image_temporary_media_receipt_guard()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'sidebar image temporary-media receipts cannot be deleted' USING ERRCODE = '55000';
  END IF;
  IF ROW(NEW.actor_id, NEW.customer_id, NEW.image_id, NEW.key_digest, NEW.created_at)
       IS DISTINCT FROM ROW(OLD.actor_id, OLD.customer_id, OLD.image_id, OLD.key_digest, OLD.created_at) THEN
    RAISE EXCEPTION 'sidebar image temporary-media receipt command facts are immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state <> 'pending' OR NEW.state NOT IN ('ready', 'outcome_unknown', 'final_failed') THEN
    RAISE EXCEPTION 'invalid sidebar image temporary-media receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER sidebar_image_temporary_media_receipt_guard
BEFORE UPDATE OR DELETE ON public.sidebar_image_temporary_media_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_sidebar_image_temporary_media_receipt_guard();

-- +goose Down
LOCK TABLE public.sidebar_image_temporary_media_receipts IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.sidebar_image_temporary_media_receipts LIMIT 1) THEN
    RAISE EXCEPTION 'cannot roll back populated sidebar image temporary-media receipts' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER sidebar_image_temporary_media_receipt_guard ON public.sidebar_image_temporary_media_receipts;
DROP FUNCTION public.aicrm_sidebar_image_temporary_media_receipt_guard();
DROP TABLE public.sidebar_image_temporary_media_receipts;
