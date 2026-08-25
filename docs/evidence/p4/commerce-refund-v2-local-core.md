# Commerce Refund V2 local closure

This package closes `LEGACY-S07-182` and `LEGACY-S07-183` as V2 backend
capabilities on `origin/main@f8ee5106eb8b61ae9541936dc52cc31870a56d77`.
All evidence below is local. It is not deployment, real WeChat Pay/WeChat Shop
execution, provider acceptance, refund success, or delivery evidence.

## Route and ownership boundary

- `LEGACY-API-0524` remains `POST
  /api/admin/wechat-pay/orders/{order_id}/refunds`, but the handler now resolves
  at most one PE01 order and delegates to `RequestRefundV2`. It never invokes
  the old Board refund writer, and the `202` response is explicitly a PE01 V2
  refund projection rather than a legacy-equivalent response.
- Resolution uses the exact reference across PE01 merchant order number,
  provider transaction reference, or local ID with `LIMIT 2`; zero matches is
  not found and two matches is conflict. `transaction_id_confirmation` must
  match the canonical provider transaction reference and participates in the
  command/payload idempotency digest.
- `LEGACY-API-0464` remains `POST /api/admin/refunds` but accepts only
  `provider=wechat_shop`. Its application, store, provider port, callback
  verifier, worker, request evidence, callback evidence, and query evidence are
  independent from `WeChatPayProvider` and PE01 EER kinds.
- Public callback evidence is exposed at `POST
  /api/public/wechat-shop/callbacks/refund`; `POST
  /api/admin/wechat-shop/refunds/{refund_id}/reconcile` is the only provider
  query path after `outcome_unknown`. Both production adapters are disabled.

The canonical route mappings are recorded in `docs/api-mapping.jsonl`; the
generated route ledger and OpenAPI contract bind `LEGACY-API-0464` to
`createLegacyRefundIntent` and `LEGACY-API-0524` to
`createLegacyWechatRefundIntent`.

## Fact and effect boundary

- WeChat Pay has one source of truth: `order_financial_refunds`. The bridge does
  not write `order_refunds` or `order_external_effects`.
- WeChat Shop owns `order_wechat_shop_refunds` plus separate attempt, callback,
  and query evidence tables. It reads historical `order_refunds` only when
  bounding the refundable amount and never writes it.
- `provider_accepted` stores a provider acceptance evidence digest but leaves
  `delivery_proven=false`. Only an independently verified exact callback or
  exact confirmed manual query transitions the refund to `succeeded` and makes
  `delivery_proven=true`.
- A network error or invalid result becomes `outcome_unknown`. A repeated River
  execution observes that state and does not call the provider again. Manual
  reconcile records query evidence; absent, mismatched, or ambiguous evidence
  fails closed.
- The production composition root uses `DisabledWeChatShopRefund` and
  `DisabledWeChatShopCallbackVerifier`. No test or runtime path calls a real
  provider, manufactures a provider receipt, or declares an external refund.

## Migration and verification

`00086_commerce_refund_v2.sql` is self-contained and does not alter the PE01
EER kind constraint. The audited baseline has migrations `00079`, `00080`, then
the owner-reserved `00086`; `00081`--`00085` are intentionally absent from this
lane and are serial-integration dependencies, so empty rollback is
`86 -> 80 -> 86`.

The PostgreSQL 16.14 acceptance target
`p4-commerce-refund-v2-acceptance` verifies:

- exact apply to 86 and empty `86 -> 80 -> 86`;
- PE01 compatibility through the single canonical table with zero legacy
  refund rows;
- fake WeChat Shop provider acceptance, no automatic second call, verified
  callback delivery proof, and zero legacy refund/effect rows;
- the populated `55000` down guard with
  `cannot roll back materialized WeChat Shop refund facts`.

Focused application tests cover ambiguous PE01 references, confirmation-bound
idempotency, disabled-provider no-enqueue, fake provider acceptance/callback,
and unknown/manual reconcile. HTTP tests cover typed request mapping, snake-case
V2 response fields, and rejection of `provider=wechat` on the WeChat Shop
route. OpenAPI generation, the owner-approved Commerce Refund operation
registry, Matrix rows, the generated route/capability ledgers, and selected
database manifest entry are checked separately.

Deployment, shadow traffic, real provider calls, real callbacks, refund
completion, and financial reconciliation remain `NOT_EXECUTED`.
