-- +goose Up
-- Historical facts only. Do not mutate Product classification, current members,
-- grants, paid settlements, event_log or any external-effect queue.
CREATE TABLE public.product_service_period_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_definition_id BIGINT NOT NULL UNIQUE CHECK (source_definition_id > 0),
  product_id BIGINT NOT NULL REFERENCES public.products(id),
  membership_config_id TEXT NOT NULL,
  membership_config_name TEXT NOT NULL,
  duration_days INTEGER NOT NULL,
  deleted BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);
CREATE TABLE public.product_service_period_entitlement_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_entitlement_id BIGINT NOT NULL UNIQUE CHECK (source_entitlement_id > 0),
  definition_id BIGINT NOT NULL REFERENCES public.product_service_period_history(id),
  customer_id BIGINT REFERENCES public.customers(id),
  membership_config_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status <> ''),
  start_at TIMESTAMPTZ NOT NULL,
  end_at TIMESTAMPTZ NOT NULL,
  last_order_id BIGINT REFERENCES public.order_list_projections(id),
  last_out_trade_no TEXT NOT NULL,
  renewal_count INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);
CREATE INDEX product_service_period_entitlement_history_page_idx ON public.product_service_period_entitlement_history(definition_id, id);
CREATE TABLE public.product_service_period_event_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_event_id BIGINT NOT NULL UNIQUE CHECK (source_event_id > 0),
  definition_id BIGINT NOT NULL REFERENCES public.product_service_period_history(id),
  entitlement_id BIGINT REFERENCES public.product_service_period_entitlement_history(id),
  customer_id BIGINT REFERENCES public.customers(id),
  order_id BIGINT REFERENCES public.order_list_projections(id),
  event_id TEXT NOT NULL CHECK (event_id <> ''),
  event_type TEXT NOT NULL CHECK (event_type <> ''),
  duration_days INTEGER NOT NULL,
  out_trade_no TEXT NOT NULL,
  before_start_at TIMESTAMPTZ,
  before_end_at TIMESTAMPTZ,
  after_start_at TIMESTAMPTZ,
  after_end_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX product_service_period_event_history_page_idx ON public.product_service_period_event_history(definition_id, id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.product_service_period_history)
    OR EXISTS (SELECT 1 FROM public.product_service_period_entitlement_history)
    OR EXISTS (SELECT 1 FROM public.product_service_period_event_history) THEN
    RAISE EXCEPTION 'cannot drop populated service-period history; restore a pre-import snapshot instead';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.product_service_period_event_history;
DROP TABLE public.product_service_period_entitlement_history;
DROP TABLE public.product_service_period_history;
