-- +goose Up
CREATE TABLE public.order_list_projections (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  provider TEXT NOT NULL,
  provider_label TEXT NOT NULL,
  merchant_order_no TEXT NOT NULL,
  platform_transaction_no TEXT NOT NULL DEFAULT '',
  customer_id BIGINT,
  payer_name_snapshot TEXT NOT NULL DEFAULT '',
  mobile_snapshot TEXT NOT NULL DEFAULT '',
  identity_kind TEXT NOT NULL DEFAULT '',
  identity_value TEXT NOT NULL DEFAULT '',
  product_id BIGINT,
  product_code TEXT NOT NULL,
  product_name_snapshot TEXT NOT NULL DEFAULT '',
  amount_minor BIGINT NOT NULL,
  currency CHAR(3) NOT NULL DEFAULT 'CNY',
  status TEXT NOT NULL,
  status_label TEXT NOT NULL,
  detail_url TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT order_list_projections_provider CHECK (provider IN ('wechat', 'alipay', 'wechat_shop')),
  CONSTRAINT order_list_projections_provider_label CHECK (btrim(provider_label) = provider_label AND provider_label <> '' AND char_length(provider_label) <= 80),
  CONSTRAINT order_list_projections_merchant_order_no CHECK (btrim(merchant_order_no) = merchant_order_no AND merchant_order_no <> '' AND char_length(merchant_order_no) <= 200),
  CONSTRAINT order_list_projections_platform_transaction_no CHECK (btrim(platform_transaction_no) = platform_transaction_no AND char_length(platform_transaction_no) <= 200),
  CONSTRAINT order_list_projections_customer_id CHECK (customer_id IS NULL OR customer_id > 0),
  CONSTRAINT order_list_projections_payer_name CHECK (btrim(payer_name_snapshot) = payer_name_snapshot AND char_length(payer_name_snapshot) <= 200),
  CONSTRAINT order_list_projections_mobile CHECK (btrim(mobile_snapshot) = mobile_snapshot AND char_length(mobile_snapshot) <= 80),
  CONSTRAINT order_list_projections_identity CHECK (
    (identity_kind = '' AND identity_value = '') OR
    (identity_kind IN ('userid', 'external_userid', 'unionid') AND btrim(identity_value) = identity_value AND identity_value <> '' AND char_length(identity_value) <= 200)
  ),
  CONSTRAINT order_list_projections_product_id CHECK (product_id IS NULL OR product_id > 0),
  CONSTRAINT order_list_projections_product_code CHECK (btrim(product_code) = product_code AND product_code <> '' AND char_length(product_code) <= 200),
  CONSTRAINT order_list_projections_product_name CHECK (btrim(product_name_snapshot) = product_name_snapshot AND char_length(product_name_snapshot) <= 200),
  CONSTRAINT order_list_projections_amount CHECK (amount_minor >= 0),
  CONSTRAINT order_list_projections_currency CHECK (currency ~ '^[A-Z]{3}$'),
  CONSTRAINT order_list_projections_status CHECK (btrim(status) = status AND status <> '' AND char_length(status) <= 80),
  CONSTRAINT order_list_projections_status_label CHECK (btrim(status_label) = status_label AND status_label <> '' AND char_length(status_label) <= 80),
  CONSTRAINT order_list_projections_detail_url CHECK (detail_url ~ '^/[^[:space:]]*$' AND char_length(detail_url) <= 2048),
  CONSTRAINT order_list_projections_time CHECK (created_at <= updated_at),
  UNIQUE (provider, merchant_order_no)
);

CREATE INDEX order_list_projections_created_idx
  ON public.order_list_projections (created_at DESC, id DESC);
CREATE INDEX order_list_projections_provider_created_idx
  ON public.order_list_projections (provider, created_at DESC, id DESC);
CREATE INDEX order_list_projections_status_created_idx
  ON public.order_list_projections (status, created_at DESC, id DESC);
CREATE INDEX order_list_projections_provider_status_created_idx
  ON public.order_list_projections (provider, status, created_at DESC, id DESC);
CREATE INDEX order_list_projections_product_created_idx
  ON public.order_list_projections (product_code, created_at DESC, id DESC);
CREATE INDEX order_list_projections_merchant_order_trgm_idx
  ON public.order_list_projections USING GIN (merchant_order_no gin_trgm_ops);
CREATE INDEX order_list_projections_mobile_trgm_idx
  ON public.order_list_projections USING GIN (mobile_snapshot gin_trgm_ops);

CREATE TABLE public.order_list_projection_counters (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE,
  total_orders BIGINT NOT NULL DEFAULT 0,
  CONSTRAINT order_list_projection_counters_singleton CHECK (singleton),
  CONSTRAINT order_list_projection_counters_total CHECK (total_orders >= 0)
);

INSERT INTO public.order_list_projection_counters (singleton, total_orders) VALUES (TRUE, 0);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_order_list_projection_count_insert()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE inserted_count BIGINT;
BEGIN
  SELECT count(*) INTO inserted_count FROM inserted_rows;
  UPDATE public.order_list_projection_counters SET total_orders = total_orders + inserted_count WHERE singleton = TRUE;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_order_list_projection_count_delete()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE deleted_count BIGINT;
BEGIN
  SELECT count(*) INTO deleted_count FROM deleted_rows;
  UPDATE public.order_list_projection_counters SET total_orders = total_orders - deleted_count WHERE singleton = TRUE;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER order_list_projection_count_insert
AFTER INSERT ON public.order_list_projections
REFERENCING NEW TABLE AS inserted_rows
FOR EACH STATEMENT EXECUTE FUNCTION public.aicrm_order_list_projection_count_insert();

CREATE TRIGGER order_list_projection_count_delete
AFTER DELETE ON public.order_list_projections
REFERENCING OLD TABLE AS deleted_rows
FOR EACH STATEMENT EXECUTE FUNCTION public.aicrm_order_list_projection_count_delete();

-- +goose Down
DROP TRIGGER order_list_projection_count_delete ON public.order_list_projections;
DROP TRIGGER order_list_projection_count_insert ON public.order_list_projections;
DROP FUNCTION public.aicrm_order_list_projection_count_delete();
DROP FUNCTION public.aicrm_order_list_projection_count_insert();
DROP TABLE public.order_list_projection_counters;
DROP TABLE public.order_list_projections;
