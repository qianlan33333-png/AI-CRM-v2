-- +goose Up
CREATE TABLE public.coupon_v1_history_definitions (
  coupon_id BIGINT PRIMARY KEY REFERENCES public.coupons(id),
  source_coupon_id BIGINT NOT NULL UNIQUE CHECK (source_coupon_id > 0),
  original_status TEXT NOT NULL
);

CREATE TABLE public.coupon_v1_history_claims (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_claim_id BIGINT NOT NULL UNIQUE CHECK (source_claim_id > 0),
  source_coupon_id BIGINT NOT NULL CHECK (source_coupon_id > 0),
  coupon_id BIGINT NOT NULL REFERENCES public.coupon_v1_history_definitions(coupon_id),
  customer_id BIGINT REFERENCES public.customers(id),
  claim_no TEXT NOT NULL,
  status TEXT NOT NULL,
  discount_amount_total BIGINT NOT NULL CHECK (discount_amount_total >= 0),
  currency TEXT NOT NULL CHECK (currency = 'CNY'),
  valid_from TIMESTAMPTZ NOT NULL,
  valid_until TIMESTAMPTZ NOT NULL,
  claimed_at TIMESTAMPTZ NOT NULL,
  reserved_at TIMESTAMPTZ,
  consumed_at TIMESTAMPTZ,
  expired_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX coupon_v1_history_claims_coupon ON public.coupon_v1_history_claims(coupon_id,id);

CREATE TABLE public.coupon_v1_history_redemptions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_redemption_id BIGINT NOT NULL UNIQUE CHECK (source_redemption_id > 0),
  source_claim_id BIGINT NOT NULL CHECK (source_claim_id > 0),
  source_order_id BIGINT NOT NULL CHECK (source_order_id > 0),
  claim_history_id BIGINT NOT NULL REFERENCES public.coupon_v1_history_claims(id),
  order_id BIGINT REFERENCES public.order_list_projections(id),
  out_trade_no TEXT NOT NULL,
  status TEXT NOT NULL,
  original_amount_total BIGINT NOT NULL CHECK (original_amount_total >= 0),
  discount_amount_total BIGINT NOT NULL CHECK (discount_amount_total >= 0),
  payable_amount_total BIGINT NOT NULL CHECK (payable_amount_total >= 0),
  currency TEXT NOT NULL CHECK (currency = 'CNY'),
  reserved_until TIMESTAMPTZ NOT NULL,
  release_reason TEXT NOT NULL,
  reserved_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  released_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX coupon_v1_history_redemptions_claim ON public.coupon_v1_history_redemptions(claim_history_id,id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.coupon_v1_history_definitions)
    OR EXISTS (SELECT 1 FROM public.coupon_v1_history_claims)
    OR EXISTS (SELECT 1 FROM public.coupon_v1_history_redemptions) THEN
    RAISE EXCEPTION 'coupon history is populated; restore a separate snapshot instead';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.coupon_v1_history_redemptions;
DROP TABLE public.coupon_v1_history_claims;
DROP TABLE public.coupon_v1_history_definitions;
