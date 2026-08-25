-- +goose Up
-- Commerce External Push stores only Product-local opaque configuration and
-- acceptance facts. It has no Provider endpoint, request body, credential,
-- webhook or automatic retry path.
ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_owner_check,
  ADD CONSTRAINT external_effects_owner_check CHECK (owner IN (
    'campaign','contact','outbound','wecom','survey','audience','order','product'
  )),
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync',
    'survey_webhook','audience_webhook','order_payment_prepay',
    'order_payment_capture','order_refund','product_external_push_test'
  ));

ALTER TABLE public.product_operation_receipts
  DROP CONSTRAINT product_operation_receipts_operation,
  ADD CONSTRAINT product_operation_receipts_operation CHECK (
    operation IN (
      'create','update','lifecycle','enable','disable','copy','delete',
      'external_push_save','external_push_test'
    )
  );

CREATE TABLE public.product_external_push_configurations (
  product_id BIGINT PRIMARY KEY REFERENCES public.products(id) ON DELETE RESTRICT,
  product_kind TEXT NOT NULL CHECK (product_kind IN ('wechat_pay','service_period')),
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  configuration_reference TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT product_external_push_configurations_reference CHECK (
    btrim(configuration_reference) = configuration_reference
    AND char_length(configuration_reference) <= 128
    AND position('://' IN configuration_reference) = 0
    AND (
      (enabled = FALSE AND configuration_reference = '')
      OR (enabled = TRUE AND configuration_reference <> '')
    )
  )
);

CREATE TABLE public.product_external_push_test_bindings (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  product_id BIGINT NOT NULL REFERENCES public.products(id) ON DELETE RESTRICT,
  product_kind TEXT NOT NULL CHECK (product_kind IN ('wechat_pay','service_period')),
  operation_receipt_id BIGINT NOT NULL UNIQUE REFERENCES public.product_operation_receipts(id) ON DELETE RESTRICT,
  external_effect_id BIGINT NOT NULL UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  configuration_digest BYTEA NOT NULL CHECK (octet_length(configuration_digest) = 32),
  state TEXT NOT NULL CHECK (state IN ('accepted','queued')),
  provider_accepted BOOLEAN NOT NULL DEFAULT FALSE CHECK (provider_accepted = FALSE),
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE CHECK (delivery_proven = FALSE),
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (real_external_call_executed = FALSE),
  auto_retry_allowed BOOLEAN NOT NULL DEFAULT FALSE CHECK (auto_retry_allowed = FALSE),
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX product_external_push_test_bindings_product_created_idx
  ON public.product_external_push_test_bindings (product_id, created_at DESC, id DESC);

-- +goose Down
-- Serialize with Product and EER writers. Immutable EER facts are never
-- deleted merely to make an older schema fit.
LOCK TABLE public.product_external_push_test_bindings,
  public.product_external_push_configurations,
  public.product_operation_receipts,
  public.external_effects,
  public.products
IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.product_external_push_test_bindings)
    OR EXISTS (SELECT 1 FROM public.product_external_push_configurations)
    OR EXISTS (
      SELECT 1 FROM public.product_operation_receipts
      WHERE operation IN ('external_push_save','external_push_test')
    ) OR EXISTS (
      SELECT 1 FROM public.external_effects
      WHERE owner = 'product' OR kind = 'product_external_push_test'
    ) THEN
    RAISE EXCEPTION 'cannot roll back populated product external push facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP INDEX public.product_external_push_test_bindings_product_created_idx;
DROP TABLE public.product_external_push_test_bindings;
DROP TABLE public.product_external_push_configurations;
ALTER TABLE public.product_operation_receipts
  DROP CONSTRAINT product_operation_receipts_operation,
  ADD CONSTRAINT product_operation_receipts_operation CHECK (
    operation IN ('create','update','lifecycle','enable','disable','copy','delete')
  );
ALTER TABLE public.external_effects
  DROP CONSTRAINT external_effects_kind_check,
  ADD CONSTRAINT external_effects_kind_check CHECK (kind IN (
    'campaign_dispatch','campaign_group_announcement','contact_touch',
    'outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync',
    'survey_webhook','audience_webhook','order_payment_prepay',
    'order_payment_capture','order_refund'
  )),
  DROP CONSTRAINT external_effects_owner_check,
  ADD CONSTRAINT external_effects_owner_check CHECK (owner IN (
    'campaign','contact','outbound','wecom','survey','audience','order'
  ));
