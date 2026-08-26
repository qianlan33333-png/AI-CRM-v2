# P4 Commerce WeChat Pay direct safe export

`P4-COMMERCE-WXPAY-DIRECT-EXPORT-2026-08-26` adds a V2-safe backend export for
`POST /api/admin/wechat-pay/order-exports`.

- The request is fixed to `resource=orders`, `format=csv`, and
  `provider=wechat` and requires human session, `order.write`, CSRF, actor, and
  Idempotency-Key.
- All seven requested filters are applied to the local read model:
  `mobile`, `identity`, `transaction_id`, `product_code`, `status`,
  `created_from`, and `created_to`.
- The first three sensitive filters are query-only. They are excluded from the
  durable command/receipt snapshot, event payload, and CSV.
- CSV output is formula-safe and restricted to `local_id`, `provider`,
  `product_code`, `amount_minor`, `currency`, `status`, and `created_at`.
- The response is an immediate private/no-store CSV download. The legacy job
  detail/download routes remain fail-closed with `410 Gone`.

Focused application and HTTP tests cover exact filter mapping, provider
restriction, CSRF and idempotency, safe column order, formula escaping, and
non-persistence of sensitive filter values. OpenAPI and Orval generation are
deterministic.

This is a deliberately safer V2 capability, not strict 1:1 completion of
`LEGACY-S07-177`, whose legacy behavior exported payer identity and financial
references. It makes no deployment, Provider call, payment, refund, or real
external-effect claim.
