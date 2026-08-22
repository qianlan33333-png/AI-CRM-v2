-- +goose Up
-- Product lifecycle is a CRM-local fact. It does not imply provider
-- configuration, public purchase availability, payment, or refund effects.
ALTER TABLE public.products
  ADD COLUMN local_lifecycle TEXT NOT NULL DEFAULT 'draft';

UPDATE public.products
SET local_lifecycle = CASE
  WHEN legacy_admin_projection->>'status' IN ('active', 'enabled')
    AND legacy_admin_projection->>'enabled' = 'true' THEN 'enabled'
  WHEN legacy_admin_projection->>'status' IN ('disabled', 'inactive')
    AND legacy_admin_projection->>'enabled' <> 'true' THEN 'disabled'
  ELSE 'draft'
END;

ALTER TABLE public.products
  ADD CONSTRAINT products_local_lifecycle
  CHECK (local_lifecycle IN ('draft', 'disabled', 'enabled'));

ALTER TABLE public.product_operation_receipts
  DROP CONSTRAINT product_operation_receipts_operation,
  ADD CONSTRAINT product_operation_receipts_operation CHECK (
    operation IN ('create', 'update', 'lifecycle', 'enable', 'disable', 'copy', 'delete')
  );

-- +goose Down
-- Keep completed lifecycle receipts and non-draft local states safe on
-- rollback. An operator must explicitly decide how those facts are handled.
LOCK TABLE public.product_operation_receipts, public.products
  IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM public.product_operation_receipts
    WHERE operation IN ('lifecycle', 'enable', 'disable', 'copy', 'delete')
  ) OR EXISTS (
    SELECT 1
    FROM public.products
    WHERE local_lifecycle <> 'draft'
  ) THEN
    RAISE EXCEPTION 'cannot roll back product local lifecycle facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
ALTER TABLE public.product_operation_receipts
  DROP CONSTRAINT product_operation_receipts_operation,
  ADD CONSTRAINT product_operation_receipts_operation CHECK (
    operation IN ('create', 'update')
  );
ALTER TABLE public.products DROP CONSTRAINT products_local_lifecycle;
ALTER TABLE public.products DROP COLUMN local_lifecycle;
