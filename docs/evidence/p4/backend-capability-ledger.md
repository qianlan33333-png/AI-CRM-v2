# P4 后端能力冻结账本

## 口径

本账本只记录 P4 后端四个业务包，不把前端、部署或真实外部效果计入后端完成度。
审计基线是 `origin/main` 的
`71b9b4f43276ae55f0a0ea926152f72fbe0fc6b3`（2026-08-24）。

这四个业务包是 V2 原生后端能力。它们不与旧 Feature Matrix 行做虚假的
1:1 匹配，也不重复计入已经由旧 Campaign API 关闭的 Matrix 行。
`tools/openapi-contract` 中的 `nativePackageOperations` 是机器可执行合同：
这些操作必须保持无 `x-legacy-mapping-ids`、`external-effect: none`，并严格匹配
已批准的 RBAC、CSRF、数据来源和数据分类。

## 冻结范围

| 顺序 | 业务包 | Migration | V2 operationId | 主线实现证据 | 本次本地验收 |
| --- | --- | --- | --- | --- | --- |
| 1 | Contact Touch Policy | `00065_customer_contact_policies.sql` | `getCustomerContactPolicy`、`putCustomerContactPolicy`、`deleteCustomerContactPolicy` | PR #410；`cf362a6bcc567662f9a729cce1aa945ae678e771` | PASS |
| 2 | Campaign Initiation Snapshots | `00066_campaign_initiation_snapshots.sql` | `listCloudCampaignTouchPlans`、`createCloudCampaignTouchPlan`、`getCloudCampaignTouchPlan` | PR #411；`75ceaab2e34ee90f8efa07222c5c8d292b065281` | PASS |
| 3 | Campaign Review/Handoff | `00067_campaign_touch_plan_review_handoff.sql` | `listCloudCampaignTouchPlanRecipients`、`getCloudCampaignTouchPlanRecipient`、`getCloudCampaignTouchPlanReview`、`mutateCloudCampaignTouchPlanReview` | PR #412；`369aa2e743d7de2c5048f84bc3b9326b67edb5b0` | PASS |
| 4 | Outbound Accept/Reconcile | `00068_outbound_campaign_handoff_acceptance.sql` | `getOutboundCampaignHandoffSummary`、`acceptOutboundCampaignHandoff`、`reconcileOutboundCampaignHandoff` | PR #413；`2c96c71a1257efbde67de7267173d79261b3ffd9` | PASS |

冻结分母是 **4 个后端业务包、13 个 V2 操作**。审计基线上已实现
**4/4、13/13**。所有写操作要求 human session、`operations.manage`、CSRF
和幂等键；所有读操作要求 `operations.read`。四个包均只产生本地事实、审计
事件或 held task link，不创建发送任务，不调用 Provider，也不证明送达。

## Matrix 与 USER OPS 关系

- `docs/feature-matrix.csv` 仍是 294 行旧系统行为账本；本次
  `feature-matrix-contract` 通过，未改写其不可变旧事实。
- `LEGACY-S06-019` 与 `LEGACY-S06-036` 至 `LEGACY-S06-042` 已按
  `P4-UO-CANCEL-2026-08-23` 标记为 `DEPRECATED`。独立 USER OPS 不再开发。
- 00065–00068 是旧 USER OPS 之后的 V2 后端业务流，不把 OneID、本地触达策略、
  不可变触达快照、审核交接或 Outbound held reconciliation 硬写成旧路由的
  1:1 实现。
- 旧 Campaign Matrix 行仍以其原 operation/action/result 证据独立计数；本账本
  不重复回填这些行。

## 验收收据

以下命令均在上述审计基线、隔离的 loopback PostgreSQL 16.14
`aicrm_test` 派生临时库上于 2026-08-24 执行：

| 验收 | 结果 | 证明边界 |
| --- | --- | --- |
| `make p4-contact-policy-acceptance` | PASS | 00065 store race、事实/receipt 阻止 down、空库 64→65、恢复到 68 |
| `go test -race -count=1 -timeout=300s ./acceptance/campaign -args -database-url <isolated-00068-dsn>` | PASS | 00066 快照与并发、00067 审核/批准/拒绝/不可变交接、迁移保护 |
| `make p4-outbound-campaign-handoff-acceptance` | PASS | 00001→00068、67/68 空库 down/up、事实保护、接纳/重放/对账 |
| `go test -race -count=1 ./internal/contact/app ./internal/contact/http ./internal/campaign ./internal/campaign/app ./internal/outbound/app ./internal/outbound/http` | PASS | 应用与 HTTP 正常、边界、RBAC 上下文、幂等和并发合同 |
| `go test -race -count=1 ./cmd/aicrm` | PASS | 组合根路由、CSRF 与依赖接线 |
| `make generate-check openapi-p1-contract feature-matrix-contract` | PASS | generated 无漂移、13 个 V2 原生操作边界、旧 Matrix 294 行合同 |

历史实现 PR #410–#413 的 `ci / api-codegen`、`ci / database`、
`ci / shared-regression`、`ci / secret-diff` 与 `ci / merge-gate` 均通过后合入
main。该历史 required CI 只证明各实现 PR 的合并门，不替代当前精确 main 的
完整 Nightly。

## 分层状态

| 层级 | 状态 | 说明 |
| --- | --- | --- |
| 后端业务能力 | `MAIN_IMPLEMENTED_LOCAL_ACCEPTANCE_PASS` | 4/4 包、13/13 操作已在审计基线并通过上述验收 |
| 旧 Feature Matrix | `CONTRACT_PASS_294_ROWS` | 无新增硬匹配；8 行 USER OPS 已弃用 |
| V2 后端能力账本 | `FROZEN_4_PACKAGES_13_OPERATIONS` | 本文件与 OpenAPI 原生操作合同共同承载 |
| 当前 main 完整 Nightly | `NOT_EXECUTED_FOR_FREEZE_SHA` | 账本合入后对精确 main SHA 单独运行 |
| 前端集中接入 | `NOT_IN_CURRENT_DOD` | 后端冻结后单独启动，不在本账本计数 |
| 部署 | `NOT_EXECUTED` | 不由本地验收或 main 合并推导 |
| Provider/企微/支付退款真实效果 | `NOT_EXECUTED_REQUIRES_EXPLICIT_AUTHORIZATION` | 本阶段不执行；receipt/reconciliation 需在授权后另验 |
