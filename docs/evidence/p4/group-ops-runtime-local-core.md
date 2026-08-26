# P4 Group Ops Runtime backend evidence

Baseline: `origin/main=1f6671dd9c80a6f53737cdf3eb4eb6e5e483a41e`.
Business package migrations: `00085`, `00097`, and `00098`.
This is branch-local backend evidence only. It is not a main merge, Nightly result, deployment, Provider call, delivery receipt, or external-effect acceptance.

## Capability boundary

- The `00063` control plane remains the source for plans, operation members, group assets, nodes, webhook descriptors, and content preview. Independent USER OPS is removed; `scope=group_ops` owns the operation-member responsibility.
- `00085` owns immutable runs/executions, sender snapshots, group-directory projections, EER acceptance, dispatch outcomes, and reconciliation.
- `00097` persists WeCom group-message task receipts separately from verified delivery evidence. Provider acceptance and delivery proof remain distinct facts.
- `00098` adds typed material plans, durable HMAC replay receipts, Media-owned temporary-upload preparations/receipts, and Group Ops execution intents.
- Text-only nodes are accepted and queued atomically. Material nodes first persist an intent and Media preparation jobs; only a provider-ready lease covering dispatch plus one hour can become an immutable Group Ops execution.
- Upload preparation and group broadcast are different EER effects. A Media upload receipt never counts as Group Ops Provider acceptance or delivery.
- Provider transport/parse ambiguity becomes `outcome_unknown` and is never automatically replayed. Explicit Provider rejection becomes `final_failed`. Attempted crash recovery performs no Provider call.

## API, protocol, and route evidence

The backend and OpenAPI register the control-plane routes plus:

- run-due preview/acceptance, API-client broadcast acceptance, HMAC webhook acceptance, run/execution reads, and evidence-gated reconciliation;
- typed `material_plan` references for image, mini-program, attachment, and group invite; non-empty legacy `material_reference` fails closed;
- group directory/picker reads and explicit refresh;
- `GET/POST /api/admin/common/operation-members?scope=group_ops`, with no separate USER OPS board.

Broadcast authentication is strict JWT (`purpose=group_broadcast`, `aud=external_integration`, `scope=write`, `capability=group_broadcast_execute`). Webhook authentication uses client/timestamp/event/signature headers, HMAC-SHA256 over the canonical timestamp, event ID, and raw body, a five-minute past/sixty-second future window, and a durable one-time replay reservation.

When WeCom outbound is disabled, intake fails closed before EER acceptance. When enabled, API accepts only local intents; Provider credentials and upload/dispatch clients are bound in the worker component. No test in this package performs a real WeCom call.

## Migration and generated evidence

- Migration waterline is contiguous through `00098`; populated `00098` material/replay/intent facts block rollback.
- The runtime acceptance target now verifies exact `85` and `98`, populated guards, and empty `84/85` plus `96/98` rollback/reapply on PostgreSQL 16.14 CI.
- Local PostgreSQL 16.13 supplementary checks proved fresh `1 -> 98`, populated `00098` rollback rejection, and empty `98 -> 96 -> 98`. The repository's exact-version acceptance correctly rejects 16.13, so it is not reported as PostgreSQL 16.14 evidence.
- OpenAPI Go output, SQLC output, Orval TypeScript output, and `scripts/generated-sources.sha256` are regenerated from source contracts; generated files are not hand-edited.

## Local verification receipts

- focused Group Ops/Media/WeCom/outbound-provider/cmd tests: PASS;
- focused race tests and vet: PASS;
- `make migration-validate`: PASS;
- `make generate-check`: PASS;
- `go -C tools test ./openapi-contract`: PASS after updating the canonical external-effect declarations to the implemented EER/material-intent and reconciliation behavior;
- `make arch-import-lint`: PASS with the pinned Go 1.26.6 toolchain;
- Orval regenerated. The final clean-tree `orval-check`, exact PostgreSQL 16.14 acceptance, PR required CI, and exact-main batch Nightly remain required before merge/batch closure.

## Layered status

`BACKEND_CAPABILITY_BRANCH_COMPLETE_PENDING_FINAL_REVIEW`; `MAIN_NOT_MERGED`; `BATCH_NIGHTLY_NOT_RUN`; `DEPLOYMENT_NOT_EXECUTED`; `REAL_PROVIDER_NOT_EXECUTED`; `REAL_WEBHOOK_NOT_EXECUTED`; `DELIVERY_NOT_PROVEN`.
