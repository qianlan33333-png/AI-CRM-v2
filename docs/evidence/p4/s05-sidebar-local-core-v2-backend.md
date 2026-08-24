# S05 Customer Sidebar Local Core：V2 后端能力账本

## 口径与状态

审计与开发基线是 `origin/main@82974b5b5b00744dcbd5399523868b2c6c86c199`。
本包只交付纯本地后端能力，不包含前端渲染、debounce/toast、浏览器缓存与请求取消、
JSSDK/OAuth、企微发送、Provider 调用、`force_rebind` 或真实缩略图生成。

`LEGACY-S05-022,023,024,026,030,031,032,033` 的旧 `expected_result` 都包含本包未交付的
客户端/UI 语义，且 031、033 有明确 V2 语义差异。因此八行在
`docs/feature-matrix.csv` 中继续保持 `NOT_STARTED/NOT_RUN`，不得标为 `IMPLEMENTED`。
本文件记录的是 **1 个 V2 后端包、8 个旧行关联、9 个本地 operation**，不增加旧 Matrix
完成分子。

## 逐行 V2 后端能力

| Matrix 行 | V2 operation | 已交付的本地后端能力 | 未交付或 V2 差异 | Matrix 状态 |
| --- | --- | --- | --- | --- |
| `LEGACY-S05-022` | `POST /api/sidebar/context-token` | 可选 human session；无会话返回 HTTP 200 `viewer_session_required`；有会话先实际授权 `customers.read`；经 canonical Identity resolve 后签发 15 分钟 HMAC token，绑定 corp/customer/owner/viewer；他人 owner 与未绑定返回完全相同的 `customer_not_bound` | 不执行 JSSDK/OAuth；响应名为 `context_token`，不是伪装旧 `sidebar_owner_token` | 保持 pending |
| `LEGACY-S05-023` | `GET /api/sidebar/v2/workbench` | 以 human session、RBAC 与 context token 聚合 Contact profile、问卷计数、订单计数、周期成员计数、Media 计数 | 只返回后端聚合；不渲染 UI，不伪造 legacy workflow/diagnostics 客户端状态 | 保持 pending |
| `LEGACY-S05-024` | `PUT /api/sidebar/v2/profile` | Contact-owned `customers.extra.sidebar_profile`；owner scope、`expected_updated_at` CAS、幂等 receipt、同事务 `customer.updated` 事件；保留 `extra` 其它键 | 不实现 520ms debounce、toast 或前端自动保存调度 | 保持 pending |
| `LEGACY-S05-026` | `GET /api/sidebar/v2/questionnaires` | 复用 Survey `CustomerSurveyAnswerReader`，只返回 bounded safe-choice answer，不暴露 mobile/textarea 文本 | 不实现展开 UI 与 tab disabled 状态 | 保持 pending |
| `LEGACY-S05-030` | `GET /api/sidebar/v2/orders`；`GET /api/sidebar/v2/periodic-orders` | 普通订单按 canonical customer_id 过滤并移除 payer/phone/identity/detail URL；周期订单复用 canonical service-period member read model | 不打开新窗口、不提供前端重试/toast；周期列表明确返回 `service_product_id/member_ref/version` | 保持 pending |
| `LEGACY-S05-031` | `PUT /api/sidebar/v2/periodic-orders/{service_product_id}/members/{member_ref}/remark` | 先按 canonical key 读取并校验 customer scope，再复用 `serviceperiodmember.Application.UpdateFields` 的 version CAS、UoW、receipt 与事件 | **V2_DIFFERENCE**：旧 `{entitlement_id}` 是 `service_period_entitlements.id`；V2 不猜映射，改用 `service_product_id + member_ref + expected_version`；不实现 debounce/blur/toast | 保持 pending |
| `LEGACY-S05-032` | `GET /api/sidebar/v2/materials` | 复用 Media 本地 image list/facets；只列 enabled metadata，返回去重 quick keywords；不创建 variant | 不实现浏览器 2 分钟缓存、无限滚动状态或取消旧请求 | 保持 pending |
| `LEGACY-S05-033` | `GET /api/sidebar/v2/materials/image/{image_id}/thumbnail` | 只验证本地 image 存在，固定返回 HTTP 202、`X-Thumbnail-Status: pending` 与 local-only safety | **V2_DIFFERENCE**：绝不生成图片，不返回图片/302/304，不声称 ready/error；pending 不是生成完成 | 保持 pending |

## 安全、事务与外部效果边界

- Context token 使用 deployment-local Identity HMAC root 的 domain-separated key，不新增 token
  表；token 绑定 corp、canonical customer、owner staff、admin viewer、role 与 expiry。每次使用
  token 都重读 Contact live profile；Customer 删除、customer/owner 不匹配统一按无效 token
  fail closed，不泄露客户存在性；Contact 依赖不可用独立返回 503。
- context-token 有会话时要求 `customers.read`，无会话时不伪造 Authorization 并返回
  `viewer_session_required`；其余 8 个 operation 都要求 human session、对应 capability 与 owner scope；
  两个 PUT 同时要求 session-bound CSRF 和 `Idempotency-Key`。
- 唯一新增 DDL 是 Contact-owned `00069_sidebar_customer_profile_receipts.sql`。它不创建重复
  Customer/Order/Entitlement/Media 主数据，只为 profile 写入提供 durable receipt 与 fail-closed
  rollback guard。
- 周期备注继续使用已有 `service_period_member_operation_receipts`；本包没有 Product entitlement
  migration，也没有 legacy entitlement ID 映射。
- 所有响应声明 `local_only=true`、`provider_execution_eligible=false`、
  `real_external_call_executed=false`。本包没有 Provider client、发送 job、支付/退款、企微调用或
  thumbnail generator 路径；本地 receipt 只证明本地提交/重放，不证明任何外部效果。

## 后端 DoD 与验收命令

完整后端 DoD：OpenAPI/generated 与 9 个 operation 一致；composition root 只注入各域 public
port/adapter；RBAC/CSRF/token live owner scope 关闭；profile 与周期备注满足 CAS/UoW/receipt；focused 与
race 通过；PostgreSQL 16.14 fresh migration 69、并发 writer、receipt replay 和 rollback guard
通过；`feature-matrix-contract` 通过且八行仍 pending。

```text
go test -count=1 ./internal/contact/app ./internal/contact/store ./internal/sidebar/... ./internal/order/... ./internal/media/... ./internal/product/serviceperiodmember/... ./internal/auth/http ./cmd/aicrm
go test -race -count=1 ./internal/contact/app ./internal/contact/store ./internal/sidebar/... ./internal/order/... ./internal/media/... ./internal/product/serviceperiodmember/... ./internal/auth/http ./cmd/aicrm
P4SIDEBAR_TEST_DATABASE_URL=<isolated-pg16.14-base-dsn> acceptance/sidebar/local_core_pg16.sh
make version-check generate-check feature-matrix-contract migration-validate openapi-p1-contract
```

## 2026-08-24 本地验收回执

- focused 与同集合 `-race -count=1` 均 PASS；覆盖 Contact profile receipt/CAS/replay、
  token 每次使用的 live customer/owner 复核、sales/admin/ops 转属失效、删除/依赖失败分类、
  optional session + `customers.read` 真实授权、他人 owner 与未绑定不可区分、
  Workbench 任一聚合依赖失败即 fail closed、questionnaire safe-choice 投影、Order PII 删减、
  periodic customer scope/canonical key、Media facets/quick keywords、thumbnail 存在时 202 pending/不存在 404、
  两条 PUT CSRF 与 context-token final-route HTTP 语义。
- `generate-check`、`feature-matrix-contract` (`rows=294`)、`migration-validate`、
  `openapi-p1-contract` 全部 PASS。OpenAPI response 是 closed schema；生成器与 main 相同：
  pinned `oapi-codegen v2.6.0` / `sqlc v1.28.0` / Go `1.26.6`。
- 只对允许的 `127.0.0.1:55431/aicrm_test` 先做了只读校验：
  `server_version_num=160014`、`current_database()=aicrm_test`。隔离临时库从空库迁移
  `1..69` 后，PostgreSQL 16.14 race acceptance PASS：两个 writer 精确一个成功、
  一个 CAS conflict，receipt/event 各一、replay 不重写、Customer `extra` 非目标键保留，非空 receipt 以
  `SQLSTATE 55000` 拒绝 00069 down。trap 后只读查询确认临时库计数为 `0`。
- 本包诱发的 arch module registry 与 receipt ownership registry 已补齐。全库
  `arch-import-lint`、`ownership-lint`、`source-policy-lint` 仍分别停在 main 已有的
  `config/http -> auth/http`、Outbound acceptance 写 `event_deliveries`、legacy AI Audience
  adapter 直接 `Exec` 首错；三个首错文件均相对 `origin/main` 为零 diff，本包未修复它们。

以上回执只证明本地 V2 后端能力与本地数据库事实；不证明 Nightly、合并、
部署、UI 或任何 Provider/企微/支付/缩略图外部效果。
