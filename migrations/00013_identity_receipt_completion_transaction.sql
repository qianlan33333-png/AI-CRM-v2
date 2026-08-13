-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_reject_incomplete_identity_receipt()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
  receipt_state TEXT;
BEGIN
  SELECT state INTO receipt_state
  FROM public.identity_operation_receipts
  WHERE id = NEW.id;

  IF NOT FOUND OR receipt_state <> 'completed' THEN
    RAISE EXCEPTION 'identity operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.aicrm_reject_incomplete_identity_receipt()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
  IF NEW.state <> 'completed' THEN
    RAISE EXCEPTION 'identity operation receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd
