# Web Admin OpenAPI Adapters Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace every Admin production Mock/seed path with a tested adapter over the current Go OpenAPI client, while leaving only genuinely absent or semantically incompatible actions blocked.

**Architecture:** `web/src/api/admin.ts` is the sole generated-operation to Kimi page-DTO boundary. Controllers consume Admin page models only; generated responses are unwrapped through the shared transport. Capability status is derived from the implemented adapter inventory and is test-checked.

**Tech Stack:** TypeScript 6, Orval 7.21 generated fetch client, esbuild, jsdom.

---

### Task 1: Admin read adapter foundation and customer/survey/channel reads

**Files:**
- Create: `web/src/api/admin.ts`, `web/src/api/admin.test.ts`
- Modify: `web/src/shared/api/client.ts`, `web/src/admin/controller.ts`, `web/src/api/capabilities.ts`

1. Write URL/method/response-mapping failures against generated customer, survey, and channel operations.
2. Implement page DTO mappers with cursor/page metadata; no seed fallback in production.
3. Bind the customer, questionnaire, and channel read pages to adapter results and render API errors/empty states.
4. Run focused adapter tests, `typecheck`, and `transport:contract`.
5. Commit `feat(web): 接通 Admin 客户问卷渠道读取`.

### Task 2: Commerce, media, tags, and member-grid reads

**Files:**
- Modify: `web/src/api/admin.ts`, `web/src/api/admin.test.ts`, Admin controllers/sections and `capabilities.ts`

1. Add generated-operation mappings for orders/refunds, products, service-period/member-grid, coupons, image/attachment/miniprogram, and tags.
2. Replace each listed page's production seed load with adapter DTOs and explicit error/empty states.
3. Add one mapping contract per domain and run focused tests.
4. Commit `feat(web): 接通 Admin 商业素材标签读取`.

### Task 3: Admin mutations

**Files:**
- Modify: `web/src/api/admin.ts`, controller/section handlers, `web/src/api/admin.test.ts`

1. Add only OpenAPI-defined mutations, passing shared CSRF/idempotency headers and preserving confirmations.
2. Show returned receipts/jobs/effects instead of invented success text; map 401/403/409/422 failures.
3. Cover customer, survey, channel, commerce, media, and tag mutation paths with focused contracts.
4. Commit `feat(web): 接通 Admin 核心写入动作`.

### Task 4: Operations, radar, configuration, and full-screen closure

**Files:**
- Modify: `web/src/api/admin.ts`, Admin sections/controllers, `web/src/api/capabilities.ts`, adapter/capability tests

1. Map audience, Group Ops, Radar, and configuration operations where DTO semantics match.
2. Keep AI/funnel/cycles blocked only with a tested missing/equivalence reason and no request path.
3. Inventory all 39 Admin screens and enforce capability entries against handlers.
4. Run Orval check/contract, typecheck, smoke, 66 E2E, adapter tests, build, and audit.
5. Rebase current `origin/main`, regenerate, push one branch, create the Chinese PR, and wait for `ci / merge-gate` before merging.
