-- +goose Up
-- The six fields are the short-lived values returned to
-- WeixinJSBridge.getBrandWCPayRequest. They intentionally exclude payer
-- identity, provider response bodies and merchant signing credentials.
ALTER TABLE public.order_payment_commands
  ADD COLUMN provider_jsapi_contract_version TEXT,
  ADD COLUMN provider_jsapi_app_id TEXT,
  ADD COLUMN provider_jsapi_timestamp BIGINT,
  ADD COLUMN provider_jsapi_nonce_str TEXT,
  ADD COLUMN provider_jsapi_package TEXT,
  ADD COLUMN provider_jsapi_sign_type TEXT,
  ADD COLUMN provider_jsapi_pay_sign TEXT,
  ADD COLUMN provider_jsapi_expires_at TIMESTAMPTZ;

-- A pre-existing prepay_ready row has no recoverable client handoff. Refuse a
-- silent migration instead of presenting it as frontend-callable.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.order_payment_commands WHERE state = 'prepay_ready'
  ) THEN
    RAISE EXCEPTION 'cannot add PE01 JSAPI handoff to an existing prepay_ready command'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE public.order_payment_commands
  DROP CONSTRAINT order_payment_commands_prepay,
  ADD CONSTRAINT order_payment_commands_prepay CHECK (
    state IN ('accepted','queued') AND (
      provider_prepay_digest IS NULL AND provider_jsapi_contract_version IS NULL
      OR provider_prepay_digest IS NOT NULL
        AND provider_jsapi_contract_version = 'wechat-jsapi/v1'
    )
    OR state = 'prepay_ready'
      AND provider_prepay_digest IS NOT NULL
      AND provider_jsapi_contract_version = 'wechat-jsapi/v1'
    OR state IN ('outcome_unknown','reconciled','final_failed')
      AND provider_prepay_digest IS NULL
      AND provider_jsapi_contract_version IS NULL
  ),
  ADD CONSTRAINT order_payment_commands_jsapi_bundle CHECK (
    provider_jsapi_contract_version IS NULL
      AND provider_jsapi_app_id IS NULL
      AND provider_jsapi_timestamp IS NULL
      AND provider_jsapi_nonce_str IS NULL
      AND provider_jsapi_package IS NULL
      AND provider_jsapi_sign_type IS NULL
      AND provider_jsapi_pay_sign IS NULL
      AND provider_jsapi_expires_at IS NULL
    OR
    provider_jsapi_contract_version = 'wechat-jsapi/v1'
      AND provider_prepay_digest IS NOT NULL
      AND provider_jsapi_app_id IS NOT NULL
      AND btrim(provider_jsapi_app_id) = provider_jsapi_app_id
      AND provider_jsapi_app_id <> '' AND char_length(provider_jsapi_app_id) <= 128
      AND provider_jsapi_timestamp IS NOT NULL
      AND provider_jsapi_timestamp > 0
      AND provider_jsapi_nonce_str IS NOT NULL
      AND btrim(provider_jsapi_nonce_str) = provider_jsapi_nonce_str
      AND provider_jsapi_nonce_str <> '' AND char_length(provider_jsapi_nonce_str) <= 128
      AND provider_jsapi_package IS NOT NULL
      AND provider_jsapi_package LIKE 'prepay_id=%'
      AND btrim(provider_jsapi_package) = provider_jsapi_package
      AND char_length(provider_jsapi_package) BETWEEN 12 AND 256
      AND provider_jsapi_sign_type IS NOT NULL
      AND provider_jsapi_sign_type = 'RSA'
      AND provider_jsapi_pay_sign IS NOT NULL
      AND btrim(provider_jsapi_pay_sign) = provider_jsapi_pay_sign
      AND provider_jsapi_pay_sign <> '' AND char_length(provider_jsapi_pay_sign) <= 1024
      AND provider_jsapi_expires_at IS NOT NULL
      AND provider_jsapi_expires_at > to_timestamp(provider_jsapi_timestamp)
      AND provider_jsapi_expires_at <= to_timestamp(provider_jsapi_timestamp) + INTERVAL '24 hours'
  );

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.order_payment_commands
    WHERE provider_jsapi_contract_version = 'wechat-jsapi/v1'
  ) THEN
    RAISE EXCEPTION 'cannot roll back materialized PE01 JSAPI handoffs'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE public.order_payment_commands
  DROP CONSTRAINT order_payment_commands_prepay,
  DROP CONSTRAINT order_payment_commands_jsapi_bundle,
  DROP COLUMN provider_jsapi_expires_at,
  DROP COLUMN provider_jsapi_pay_sign,
  DROP COLUMN provider_jsapi_sign_type,
  DROP COLUMN provider_jsapi_package,
  DROP COLUMN provider_jsapi_nonce_str,
  DROP COLUMN provider_jsapi_timestamp,
  DROP COLUMN provider_jsapi_app_id,
  DROP COLUMN provider_jsapi_contract_version,
  ADD CONSTRAINT order_payment_commands_prepay CHECK (
    state = 'prepay_ready' AND provider_prepay_digest IS NOT NULL
    OR state <> 'prepay_ready'
  );
