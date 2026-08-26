-- name: CreatePE01Order :one
INSERT INTO public.order_list_projections (
  provider, provider_label, merchant_order_no, platform_transaction_no,
  customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
  product_id, product_code, product_name_snapshot, amount_minor, currency,
  status, status_label, detail_url, created_at, updated_at,
  pe01_contract_version, product_version, product_kind, payment_identity_digest
) VALUES (
  'wechat', '微信支付', sqlc.arg(merchant_order_no), '',
  sqlc.arg(customer_id), '', '', '', '',
  sqlc.arg(product_id), sqlc.arg(product_code), sqlc.arg(product_name),
  sqlc.arg(amount_minor), 'CNY', 'awaiting_prepay', '待创建预支付',
  '/api/h5/wechat-pay/orders/' || sqlc.arg(merchant_order_no),
  sqlc.arg(created_at), sqlc.arg(created_at), 'pe01/v1',
  sqlc.arg(product_version), sqlc.arg(product_kind), sqlc.arg(payment_identity_digest)
)
RETURNING id, merchant_order_no, customer_id, product_id, product_kind,
  amount_minor, currency, status, created_at, version;

-- name: LockPE01OrderByMerchantNo :one
SELECT id, merchant_order_no, platform_transaction_no, customer_id, product_id,
  product_version, product_kind, amount_minor, currency, status,
  payment_identity_digest, settled_amount_minor, refunded_amount_minor,
  settlement_receipt_digest, paid_at, fully_refunded_at, version, created_at, updated_at
FROM public.order_list_projections
WHERE provider = 'wechat' AND merchant_order_no = sqlc.arg(merchant_order_no)
  AND pe01_contract_version = 'pe01/v1'
FOR UPDATE;

-- name: LockPE01OrderByID :one
SELECT id, merchant_order_no, platform_transaction_no, customer_id, product_id,
  product_version, product_kind, amount_minor, currency, status,
  payment_identity_digest, settled_amount_minor, refunded_amount_minor,
  settlement_receipt_digest, paid_at, fully_refunded_at, version, created_at, updated_at
FROM public.order_list_projections
WHERE id = sqlc.arg(order_id) AND pe01_contract_version = 'pe01/v1'
FOR UPDATE;

-- name: GetPE01OrderSelfScoped :one
SELECT id, merchant_order_no, customer_id, product_id, product_version, product_kind,
  amount_minor, currency, status, created_at, version
FROM public.order_list_projections
WHERE provider = 'wechat' AND merchant_order_no = sqlc.arg(merchant_order_no)
  AND pe01_contract_version = 'pe01/v1'
  AND payment_identity_digest = sqlc.arg(payment_identity_digest);

-- name: ReadPE01PrepayProviderMaterial :one
SELECT merchant_order_no, customer_id, product_name_snapshot, amount_minor,
  currency, payment_identity_digest
FROM public.order_list_projections AS projection
JOIN public.order_payment_commands AS command ON command.order_id = projection.id
WHERE projection.provider = 'wechat' AND projection.merchant_order_no = sqlc.arg(merchant_order_no)
  AND projection.pe01_contract_version = 'pe01/v1'
  AND projection.status = 'awaiting_prepay' AND command.state = 'accepted';

-- name: ReadPE01RefundProviderMaterial :one
SELECT projection.merchant_order_no, projection.amount_minor AS original_amount_minor,
  projection.currency, refund.out_refund_no, refund.amount_minor AS refund_amount_minor,
  refund.reason
FROM public.order_financial_refunds AS refund
JOIN public.order_list_projections AS projection ON projection.id = refund.order_id
WHERE refund.out_refund_no = sqlc.arg(out_refund_no)
  AND refund.state = 'accepted'
  AND projection.provider = 'wechat' AND projection.pe01_contract_version = 'pe01/v1'
  AND projection.status IN ('paid','partially_refunded');

-- name: CreatePE01PaymentCommand :one
INSERT INTO public.order_payment_commands (
  order_id, source_ref_digest, target_ref_digest, payload_digest,
  policy_version_digest, created_at, updated_at
) VALUES (
  sqlc.arg(order_id), sqlc.arg(source_ref_digest), sqlc.arg(target_ref_digest),
  sqlc.arg(payload_digest), sqlc.arg(policy_version_digest),
  sqlc.arg(created_at), sqlc.arg(created_at)
)
RETURNING id, order_id, external_effect_id, source_ref_digest, target_ref_digest,
  payload_digest, policy_version_digest, state, provider_prepay_digest,
  version, created_at, updated_at,
  provider_jsapi_contract_version, provider_jsapi_app_id, provider_jsapi_timestamp,
  provider_jsapi_nonce_str, provider_jsapi_package, provider_jsapi_sign_type,
  provider_jsapi_pay_sign, provider_jsapi_expires_at;

-- name: GetPE01PaymentCommandByOrder :one
SELECT id, order_id, external_effect_id, source_ref_digest, target_ref_digest,
  payload_digest, policy_version_digest, state, provider_prepay_digest,
  version, created_at, updated_at,
  provider_jsapi_contract_version, provider_jsapi_app_id, provider_jsapi_timestamp,
  provider_jsapi_nonce_str, provider_jsapi_package, provider_jsapi_sign_type,
  provider_jsapi_pay_sign, provider_jsapi_expires_at
FROM public.order_payment_commands
WHERE order_id = sqlc.arg(order_id);

-- name: LockPE01PaymentCommandBySourceDigest :one
SELECT id, order_id, external_effect_id, source_ref_digest, target_ref_digest,
  payload_digest, policy_version_digest, state, provider_prepay_digest,
  version, created_at, updated_at,
  provider_jsapi_contract_version, provider_jsapi_app_id, provider_jsapi_timestamp,
  provider_jsapi_nonce_str, provider_jsapi_package, provider_jsapi_sign_type,
  provider_jsapi_pay_sign, provider_jsapi_expires_at
FROM public.order_payment_commands
WHERE source_ref_digest = sqlc.arg(source_ref_digest)
FOR UPDATE;

-- name: LockPE01PaymentCommandByID :one
SELECT id, order_id, external_effect_id, source_ref_digest, target_ref_digest,
  payload_digest, policy_version_digest, state, provider_prepay_digest,
  version, created_at, updated_at,
  provider_jsapi_contract_version, provider_jsapi_app_id, provider_jsapi_timestamp,
  provider_jsapi_nonce_str, provider_jsapi_package, provider_jsapi_sign_type,
  provider_jsapi_pay_sign, provider_jsapi_expires_at
FROM public.order_payment_commands
WHERE id = sqlc.arg(command_id)
FOR UPDATE;

-- name: BindPE01PaymentEffect :one
UPDATE public.order_payment_commands
SET external_effect_id = sqlc.arg(external_effect_id), state = 'queued',
    version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(command_id) AND state = 'accepted'
  AND external_effect_id IS NULL AND version = sqlc.arg(expected_version)
RETURNING id, order_id, external_effect_id, source_ref_digest, target_ref_digest,
  payload_digest, policy_version_digest, state, provider_prepay_digest,
  version, created_at, updated_at,
  provider_jsapi_contract_version, provider_jsapi_app_id, provider_jsapi_timestamp,
  provider_jsapi_nonce_str, provider_jsapi_package, provider_jsapi_sign_type,
  provider_jsapi_pay_sign, provider_jsapi_expires_at;

-- name: RecordPE01JSAPIHandoff :one
UPDATE public.order_payment_commands
SET provider_prepay_digest = sqlc.arg(provider_prepay_digest),
    provider_jsapi_contract_version = 'wechat-jsapi/v1',
    provider_jsapi_app_id = sqlc.arg(provider_jsapi_app_id),
    provider_jsapi_timestamp = sqlc.arg(provider_jsapi_timestamp),
    provider_jsapi_nonce_str = sqlc.arg(provider_jsapi_nonce_str),
    provider_jsapi_package = sqlc.arg(provider_jsapi_package),
    provider_jsapi_sign_type = sqlc.arg(provider_jsapi_sign_type),
    provider_jsapi_pay_sign = sqlc.arg(provider_jsapi_pay_sign),
    provider_jsapi_expires_at = sqlc.arg(provider_jsapi_expires_at),
    version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(command_id) AND version = sqlc.arg(expected_version)
  AND state = 'accepted' AND provider_prepay_digest IS NULL
  AND provider_jsapi_contract_version IS NULL
RETURNING id, order_id, external_effect_id, source_ref_digest, target_ref_digest,
  payload_digest, policy_version_digest, state, provider_prepay_digest,
  version, created_at, updated_at,
  provider_jsapi_contract_version, provider_jsapi_app_id, provider_jsapi_timestamp,
  provider_jsapi_nonce_str, provider_jsapi_package, provider_jsapi_sign_type,
  provider_jsapi_pay_sign, provider_jsapi_expires_at;

-- name: CompletePE01Prepay :one
UPDATE public.order_payment_commands
SET state = sqlc.arg(state),
    provider_prepay_digest = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_prepay_digest ELSE NULL END,
    provider_jsapi_contract_version = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_contract_version ELSE NULL END,
    provider_jsapi_app_id = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_app_id ELSE NULL END,
    provider_jsapi_timestamp = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_timestamp ELSE NULL END,
    provider_jsapi_nonce_str = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_nonce_str ELSE NULL END,
    provider_jsapi_package = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_package ELSE NULL END,
    provider_jsapi_sign_type = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_sign_type ELSE NULL END,
    provider_jsapi_pay_sign = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_pay_sign ELSE NULL END,
    provider_jsapi_expires_at = CASE WHEN sqlc.arg(state)::text = 'prepay_ready' THEN provider_jsapi_expires_at ELSE NULL END,
    version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(command_id) AND version = sqlc.arg(expected_version)
  AND state IN ('queued','outcome_unknown')
  AND (sqlc.arg(state)::text <> 'prepay_ready'
    OR provider_prepay_digest = sqlc.arg(provider_prepay_digest)
      AND provider_jsapi_contract_version = 'wechat-jsapi/v1')
RETURNING id, order_id, external_effect_id, source_ref_digest, target_ref_digest,
  payload_digest, policy_version_digest, state, provider_prepay_digest,
  version, created_at, updated_at,
  provider_jsapi_contract_version, provider_jsapi_app_id, provider_jsapi_timestamp,
  provider_jsapi_nonce_str, provider_jsapi_package, provider_jsapi_sign_type,
  provider_jsapi_pay_sign, provider_jsapi_expires_at;

-- name: MarkPE01OrderAwaitingPayment :one
UPDATE public.order_list_projections
SET status = 'awaiting_payment', status_label = '等待付款',
    version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(order_id) AND status = 'awaiting_prepay'
  AND version = sqlc.arg(expected_version) AND pe01_contract_version = 'pe01/v1'
RETURNING id, merchant_order_no, customer_id, product_id, product_kind,
  amount_minor, currency, status, created_at, version;

-- name: ApplyPE01PaymentSettlement :one
UPDATE public.order_list_projections
SET platform_transaction_no = sqlc.arg(provider_transaction_ref),
    status = 'paid', status_label = '已支付', settled_amount_minor = amount_minor,
    settlement_receipt_digest = sqlc.arg(settlement_receipt_digest),
    paid_at = sqlc.arg(paid_at), version = version + 1, updated_at = sqlc.arg(paid_at)
WHERE id = sqlc.arg(order_id) AND status = 'awaiting_payment'
  AND settled_amount_minor = 0 AND refunded_amount_minor = 0
  AND amount_minor = sqlc.arg(amount_minor) AND currency = sqlc.arg(currency)
  AND version = sqlc.arg(expected_version) AND pe01_contract_version = 'pe01/v1'
RETURNING id, merchant_order_no, customer_id, product_id, product_kind,
  amount_minor, currency, status, created_at, version;

-- name: ReservePE01CallbackReceipt :one
INSERT INTO public.order_provider_callback_receipts (
  callback_kind, provider_event_digest, payload_digest, order_id, refund_id, received_at
) VALUES (
  sqlc.arg(callback_kind), sqlc.arg(provider_event_digest), sqlc.arg(payload_digest),
  sqlc.arg(order_id), sqlc.narg(refund_id), sqlc.arg(received_at)
)
ON CONFLICT (provider_event_digest) DO NOTHING
RETURNING id, callback_kind, provider_event_digest, payload_digest, order_id,
  refund_id, outcome, result_digest, state, received_at, completed_at;

-- name: GetPE01CallbackReceipt :one
SELECT id, callback_kind, provider_event_digest, payload_digest, order_id,
  refund_id, outcome, result_digest, state, received_at, completed_at
FROM public.order_provider_callback_receipts
WHERE provider_event_digest = sqlc.arg(provider_event_digest);

-- name: CompletePE01CallbackReceipt :one
UPDATE public.order_provider_callback_receipts
SET state = 'completed', outcome = sqlc.arg(outcome),
    result_digest = sqlc.arg(result_digest), completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(receipt_id) AND state = 'reserved'
RETURNING id, callback_kind, provider_event_digest, payload_digest, order_id,
  refund_id, outcome, result_digest, state, received_at, completed_at;

-- name: CountPE01ReservedRefundAmount :one
SELECT COALESCE(sum(amount_minor), 0)::bigint
FROM public.order_financial_refunds
WHERE order_id = sqlc.arg(order_id) AND state <> 'final_failed';

-- name: CreatePE01Refund :one
INSERT INTO public.order_financial_refunds (
  order_id, out_refund_no, amount_minor, currency, reason,
  source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  created_at, updated_at
) VALUES (
  sqlc.arg(order_id), sqlc.arg(out_refund_no), sqlc.arg(amount_minor), 'CNY',
  sqlc.arg(reason), sqlc.arg(source_ref_digest), sqlc.arg(target_ref_digest),
  sqlc.arg(payload_digest), sqlc.arg(policy_version_digest),
  sqlc.arg(created_at), sqlc.arg(created_at)
)
RETURNING id, order_id, external_effect_id, out_refund_no, amount_minor, currency,
  reason, source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  provider_refund_digest, settlement_receipt_digest, state, version,
  created_at, updated_at, settled_at;

-- name: LockPE01RefundByOutRefundNo :one
SELECT id, order_id, external_effect_id, out_refund_no, amount_minor, currency,
  reason, source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  provider_refund_digest, settlement_receipt_digest, state, version,
  created_at, updated_at, settled_at
FROM public.order_financial_refunds
WHERE out_refund_no = sqlc.arg(out_refund_no)
FOR UPDATE;

-- name: LockPE01RefundBySourceDigest :one
SELECT id, order_id, external_effect_id, out_refund_no, amount_minor, currency,
  reason, source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  provider_refund_digest, settlement_receipt_digest, state, version,
  created_at, updated_at, settled_at
FROM public.order_financial_refunds
WHERE source_ref_digest = sqlc.arg(source_ref_digest)
FOR UPDATE;

-- name: LockPE01RefundByID :one
SELECT id, order_id, external_effect_id, out_refund_no, amount_minor, currency,
  reason, source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  provider_refund_digest, settlement_receipt_digest, state, version,
  created_at, updated_at, settled_at
FROM public.order_financial_refunds
WHERE id = sqlc.arg(refund_id)
FOR UPDATE;

-- name: BindPE01RefundEffect :one
UPDATE public.order_financial_refunds
SET external_effect_id = sqlc.arg(external_effect_id), state = 'queued',
    version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(refund_id) AND state = 'accepted'
  AND external_effect_id IS NULL AND version = sqlc.arg(expected_version)
RETURNING id, order_id, external_effect_id, out_refund_no, amount_minor, currency,
  reason, source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  provider_refund_digest, settlement_receipt_digest, state, version,
  created_at, updated_at, settled_at;

-- name: MarkPE01RefundEffectResult :one
UPDATE public.order_financial_refunds
SET state = sqlc.arg(state), version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(refund_id) AND state IN ('queued','outcome_unknown')
  AND version = sqlc.arg(expected_version)
RETURNING id, order_id, external_effect_id, out_refund_no, amount_minor, currency,
  reason, source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  provider_refund_digest, settlement_receipt_digest, state, version,
  created_at, updated_at, settled_at;

-- name: ApplyPE01RefundSettlement :one
UPDATE public.order_financial_refunds
SET state = 'succeeded', provider_refund_digest = sqlc.arg(provider_refund_digest),
    settlement_receipt_digest = sqlc.arg(settlement_receipt_digest),
    settled_at = sqlc.arg(settled_at), updated_at = sqlc.arg(settled_at),
    version = version + 1
WHERE id = sqlc.arg(refund_id) AND state IN ('executed','outcome_unknown','reconciled')
  AND amount_minor = sqlc.arg(amount_minor) AND currency = sqlc.arg(currency)
  AND version = sqlc.arg(expected_version)
RETURNING id, order_id, external_effect_id, out_refund_no, amount_minor, currency,
  reason, source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
  provider_refund_digest, settlement_receipt_digest, state, version,
  created_at, updated_at, settled_at;

-- name: AddPE01SettledRefundToOrder :one
UPDATE public.order_list_projections
SET refunded_amount_minor = refunded_amount_minor + sqlc.arg(amount_minor),
    status = CASE
      WHEN refunded_amount_minor + sqlc.arg(amount_minor) = settled_amount_minor THEN 'refunded'
      ELSE 'partially_refunded'
    END,
    status_label = CASE
      WHEN refunded_amount_minor + sqlc.arg(amount_minor) = settled_amount_minor THEN '已退款'
      ELSE '部分退款'
    END,
    fully_refunded_at = CASE
      WHEN refunded_amount_minor + sqlc.arg(amount_minor) = settled_amount_minor THEN sqlc.arg(settled_at)::timestamptz
      ELSE NULL
    END,
    version = version + 1, updated_at = sqlc.arg(settled_at)
WHERE id = sqlc.arg(order_id) AND status IN ('paid','partially_refunded')
  AND refunded_amount_minor + sqlc.arg(amount_minor) <= settled_amount_minor
  AND version = sqlc.arg(expected_version) AND pe01_contract_version = 'pe01/v1'
RETURNING id, merchant_order_no, customer_id, product_id, product_kind,
  amount_minor, currency, status, created_at, version;

-- name: InsertPE01FinancialReconciliation :one
INSERT INTO public.order_financial_reconciliations (
  external_effect_id, evidence_digest, result_digest, outcome, recorded_at
) VALUES (
  sqlc.arg(external_effect_id), sqlc.arg(evidence_digest),
  sqlc.arg(result_digest), sqlc.arg(outcome), sqlc.arg(recorded_at)
)
ON CONFLICT (external_effect_id, evidence_digest) DO NOTHING
RETURNING id, external_effect_id, evidence_digest, result_digest, outcome, recorded_at;
