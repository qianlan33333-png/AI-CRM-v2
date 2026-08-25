# PE01 data-migration companion

PE01 creates new V2 financial and entitlement facts only. It does not import
legacy provider outcomes, replay callbacks, create EER attempts, or infer paid
or refunded state from a local/pending receipt.

| Legacy mapping | Frozen disposition | PE01 target action |
| --- | --- | --- |
| `LEGACY-T14-297` `wechat_pay_order_events` | `ARCHIVE_ONLY` | No target row, callback, event, job, or external effect. |
| `LEGACY-T14-300` `wechat_pay_orders` | `ARCHIVE_ONLY` | No `order_list_projections` or entitlement materialization. |
| `LEGACY-T14-303` `wechat_pay_refunds` | `ARCHIVE_ONLY` | No refund, compensation, callback receipt, or reconciliation materialization. |

The V2 seam starts with a new actor-bound checkout and immutable Product
snapshot. Authoritative paid/refunded facts require a verified WeChat callback
or a confirmed active-query reconciliation. The new routes are therefore a V2
contract, not evidence that legacy H5 route shapes or historical payment data
were migrated. Deployment and real Provider/payment/refund effects remain
`NOT_EXECUTED`.
