# PE01 WeChat Pay Settlement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver one fake-provider-verifiable WeChat Pay flow from immutable Order snapshot through prepay, authoritative settlement, Product entitlement/member creation, refund unknown reconciliation, and full-refund compensation.

**Architecture:** Order owns all financial facts and provider evidence. Product exposes a closed transaction-bound settlement port for entitlement/member mutation. External Effects Runtime remains the digest-only execution controller; Order-owned workers and adapters reload closed commands by digest and never treat runtime `executed` as financial settlement.

**Tech Stack:** Go 1.26.6, PostgreSQL 16, pgx/sqlc, River, oapi-codegen/OpenAPI, repository UoW and event_log.

---

### Task 1: Freeze domain contracts and invariants

**Files:**
- Create: `internal/order/port/settlement.go`
- Create: `internal/product/port/paid_settlement.go`
- Modify: `internal/externaleffects/runtime.go`
- Test: `internal/order/app/settlement_test.go`
- Test: `internal/externaleffects/runtime_test.go`

1. Add failing tests for closed provider/currency/product kinds, positive minor-unit amounts, exact replay mismatch, cumulative refund reservation, and `order_payment_prepay` owner-kind validation.
2. Run focused tests and confirm the new symbols/invariants fail to compile.
3. Add the minimal typed contracts and validators.
4. Run focused tests and confirm they pass.

### Task 2: Add financial and entitlement persistence

**Files:**
- Create: `migrations/00079_pe01_wechat_pay_settlement.sql`
- Create: `internal/order/store/queries/settlement.sql`
- Modify: `internal/product/store/queries/catalog.sql`
- Modify: `internal/product/store/queries/service_period_members.sql`
- Modify: `docs/architecture/table-ownership.yml`
- Generated: `internal/order/store/generated/*`, `internal/product/store/generated/*`

1. Add migration compatibility tests for all CHECK/UNIQUE/FK/receipt-last/down-guard constraints.
2. Define immutable orders/payment attempts/callback receipts/refunds/reconciliation and operation receipts; extend Product lineage without moving ownership.
3. Add SQLC queries with locks, CAS and receipt-last writes; regenerate SQLC.
4. Run migration validation, generation checks, ownership and architecture lint.

### Task 3: Implement atomic settlement application

**Files:**
- Create: `internal/product/app/paid_settlement.go`
- Create: `internal/product/store/paid_settlement_repository.go`
- Create: `internal/order/app/settlement.go`
- Create: `internal/order/store/settlement_repository.go`
- Test: matching `_test.go` files

1. Write fake UoW/store tests for checkout, settlement callback, refund request, full-refund compensation, replay, mismatch, injected failures and concurrent reservation.
2. Implement Order orchestration using only Product's transaction-bound port and Events owner appender.
3. Ensure financial state, entitlement/member, events and final receipt commit or roll back together.
4. Run focused race tests.

### Task 4: Connect EER and provider-shaped workers

**Files:**
- Modify: `internal/externaleffects/app/service.go`
- Modify: `internal/externaleffects/store/queries/runtime.sql`
- Modify: `internal/externaleffects/store/repository.go`
- Create: `internal/order/provider/wechatpay/adapter.go`
- Create: `internal/order/worker/wechatpay.go`
- Test: matching `_test.go` files

1. Add tests proving prepay/refund attempts persist before provider I/O, transport ambiguity becomes unknown, unknown never retries, and terminal outcome can be re-read after a crash.
2. Add the smallest safe EER terminal outcome reader.
3. Implement Order-owned adapters that reload canonical commands by digest and compare payload fingerprints before provider calls.
4. Run focused race tests with a fake provider and assert exact call counts.

### Task 5: Add external protocol and RBAC/self scope

**Files:**
- Create: `internal/order/http/wechatpay.go`
- Modify: `cmd/aicrm/api.go`
- Modify: `api/openapi.yaml`
- Generated: `internal/api/candidate/generated/server.gen.go`, `internal/api/generated/server.gen.go`
- Test: HTTP handler and API composition tests

1. Add failing tests for public checkout/status self scope, signed callbacks, malformed/oversize payloads, duplicate callbacks, admin refund RBAC/CSRF and actor-bound idempotency.
2. Implement exact callback acknowledgement and fail-closed validation without exposing payer identifiers or provider bodies.
3. Wire the real provider adapter behind a typed disabled-by-default configuration; tests inject the fake adapter.
4. Regenerate OpenAPI and run contract checks.

### Task 6: Add PG16 acceptance, migration companion and CI selection

**Files:**
- Create: `acceptance/order/pe01_integration_test.go`
- Create: `acceptance/order/pe01_migration_compatibility.sh`
- Create: `internal/order/migration/pe01.go`
- Modify: `Makefile`
- Modify: `scripts/ci/run_selected_database.sh`
- Modify: `docs/ci/go-acceptance-manifest.tsv`
- Modify only required generated/ledger ownership inputs.

1. Test the full fake-provider flow, 10-way replay/concurrency, callback mismatch rollback, unknown query reconciliation, entitlement/member counts, full-refund compensation and zero unrelated side effects.
2. Add a closed migration companion contract: import opening financial/entitlement facts only from provider evidence, quarantine unresolved/drift rows, never create EER/jobs/events/provider work, and reconcile counts/sums/digests.
3. Add selected database wiring and force the PG URL in the launcher.
4. Run focused race/vet, migration validation, generate/OpenAPI, ownership/source/arch, selected database, diff and gitleaks checks.
5. Commit one clean local checkpoint; do not push.
