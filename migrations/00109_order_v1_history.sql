-- +goose Up
ALTER TABLE public.order_list_projections
  ADD COLUMN record_origin TEXT NOT NULL DEFAULT 'native'
  CHECK (record_origin IN ('native', 'v1_history'));

-- Keep the existing (provider, merchant_order_no) uniqueness: a historical
-- import must never shadow a native order or create ambiguous refund lookup.
CREATE TABLE public.order_historical_refunds (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES public.order_list_projections(id),
  source_refund_id BIGINT NOT NULL UNIQUE CHECK (source_refund_id > 0),
  refund_number TEXT NOT NULL UNIQUE CHECK (refund_number <> '' AND char_length(refund_number) <= 200),
  provider_refund_id TEXT NOT NULL DEFAULT '',
  transaction_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status <> '' AND char_length(status) <= 80),
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  order_amount_minor BIGINT NOT NULL CHECK (order_amount_minor > 0 AND amount_minor <= order_amount_minor),
  currency CHAR(3) NOT NULL CHECK (currency = 'CNY'),
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);
CREATE INDEX order_historical_refunds_order_idx
  ON public.order_historical_refunds (order_id, created_at DESC, id DESC);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.order_historical_refunds)
    OR EXISTS (SELECT 1 FROM public.order_list_projections WHERE record_origin = 'v1_history') THEN
    RAISE EXCEPTION 'cannot drop populated V1 order history; restore the pre-import snapshot instead';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.order_historical_refunds;
ALTER TABLE public.order_list_projections DROP COLUMN record_origin;
