-- +goose Up
-- Product references are historical facts and must not outlive the product.
-- Fail closed before adding the constraints if a pre-existing orphan is
-- discovered; no fact is guessed, repaired, or deleted by this migration.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM public.coupon_targets target
    LEFT JOIN public.products product ON product.id = target.product_id
    WHERE product.id IS NULL
  ) OR EXISTS (
    SELECT 1
    FROM public.order_list_projections order_projection
    LEFT JOIN public.products product ON product.id = order_projection.product_id
    WHERE order_projection.product_id IS NOT NULL AND product.id IS NULL
  ) THEN
    RAISE EXCEPTION 'cannot add product reference constraints while orphan facts exist'
      USING ERRCODE = '23503';
  END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE public.coupon_targets
  ADD CONSTRAINT coupon_targets_product_fk
  FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE RESTRICT;

ALTER TABLE public.order_list_projections
  ADD CONSTRAINT order_list_projections_product_fk
  FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE RESTRICT;

-- +goose Down
-- Remove only the guards. Historical rows remain untouched.
ALTER TABLE public.order_list_projections
  DROP CONSTRAINT order_list_projections_product_fk;

ALTER TABLE public.coupon_targets
  DROP CONSTRAINT coupon_targets_product_fk;
