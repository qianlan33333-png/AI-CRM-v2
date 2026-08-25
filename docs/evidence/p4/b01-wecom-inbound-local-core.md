# B01 WeCom inbound local-core evidence

Baseline: EER mainline `dbd6579b`; migration reserved by this package:
`00077` (`00076` remains owned by DMH PR #495).

## Closed local capability

- Verified callback transport is preserved at `/api/wecom/events` and
  `/wecom/external-contact/callback`.
- `wecom_contact_inbox` stores one callback or directory-sync fact per stable
  source key and records only local state, lease/fence, digest, and River job
  linkage.
- Callback insertion and critical-job insertion are one UnitOfWork transaction;
  duplicate callbacks do not enqueue a second job.
- `IdentityContactProcessor` delegates verified external-user facts through the
  Identity port. Identity returns `attributed`, `pending`, or `conflict`; B01
  persists the corresponding local terminal state.
- Directory sync persists every page item and queues its job before cursor
  advancement.

## Verification

- `go test ./internal/wecom/app ./internal/wecom/store ./internal/wecom/worker`
- `go test ./cmd/aicrm ./internal/identity/http`
- `make generate-check`
- `make migration-validate`
- PG16 acceptance target: `p4-b01-wecom-inbound-acceptance`, covering migration
  `77 down/up`, callback idempotency, critical local job, and populated-table
  rollback refusal.

## Effect statement

No Provider client or outbound write exists in this package. Local ACK, queue,
receipt, processing, and reconciliation facts must not be reported as real
WeCom sync, delivery, welcome, tag, or external-effect success.
