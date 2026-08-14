-- +goose Up
CREATE TABLE public.products (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  product_code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  price_minor BIGINT NOT NULL,
  currency CHAR(3) NOT NULL,
  stock_quantity INTEGER NOT NULL,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  legacy_admin_projection JSONB NOT NULL DEFAULT '{"schema_version":1}',
  CONSTRAINT products_product_code CHECK (btrim(product_code) = product_code AND product_code <> '' AND char_length(product_code) <= 200),
  CONSTRAINT products_name CHECK (btrim(name) = name AND name <> '' AND char_length(name) <= 200),
  CONSTRAINT products_description CHECK (char_length(description) <= 10000),
  CONSTRAINT products_price_minor CHECK (price_minor >= 0),
  CONSTRAINT products_currency CHECK (currency ~ '^[A-Z]{3}$'),
  CONSTRAINT products_stock_quantity CHECK (stock_quantity >= 0),
  CONSTRAINT products_created_by CHECK (created_by > 0),
  CONSTRAINT products_legacy_admin_projection CHECK (jsonb_typeof(legacy_admin_projection) = 'object' AND legacy_admin_projection ? 'schema_version')
);

CREATE TABLE public.product_images (
  product_id BIGINT NOT NULL REFERENCES public.products(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  image_url TEXT NOT NULL,
  PRIMARY KEY (product_id, position),
  CONSTRAINT product_images_position CHECK (position >= 0),
  CONSTRAINT product_images_url CHECK (btrim(image_url) = image_url AND image_url <> '' AND char_length(image_url) <= 2048)
);

CREATE TABLE public.product_catalog_counters (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE,
  total_products BIGINT NOT NULL DEFAULT 0,
  CONSTRAINT product_catalog_counters_singleton CHECK (singleton),
  CONSTRAINT product_catalog_counters_total CHECK (total_products >= 0)
);

INSERT INTO public.product_catalog_counters (singleton, total_products) VALUES (TRUE, 0);

CREATE TABLE public.product_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT product_operation_receipts_operation CHECK (operation = 'create'),
  CONSTRAINT product_operation_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT product_operation_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT product_operation_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT product_operation_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT product_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_product_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.product_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'product operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER product_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.product_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_product_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_product_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed product operation receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid product operation receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER product_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.product_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_product_receipt_transition_valid();

-- +goose Down
DROP TRIGGER product_operation_receipts_transition ON public.product_operation_receipts;
DROP TRIGGER product_operation_receipts_complete_before_commit ON public.product_operation_receipts;
DROP TABLE public.product_operation_receipts;
DROP TABLE public.product_catalog_counters;
DROP TABLE public.product_images;
DROP TABLE public.products;
DROP FUNCTION public.aicrm_product_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_product_receipt();
