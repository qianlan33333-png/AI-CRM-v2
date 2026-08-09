# P1-S04 上层域与剩余旧 API 静态事实

- Issue: #69
- Legacy source: `origin/main@6cb989c071255437d75953dabb943318a74eb8f4`
- P1 input: `docs/evidence/p1/legacy-routes-6cb989c.json`
- P1 input SHA-256: `fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231`
- Input manifest SHA-256: `3bb11a48c8bbc520fb9da5128726594232bca0b4e0f0c7ed1f63bb4b3c2263bd`
- Evidence level: `LOCAL` / static source only
- Disposition: `UNREVIEWED`
- Signoff: `PENDING_HUMAN_SIGNOFF`

## 调查边界

Terra task `/root/p1_s04_upper_domains` 使用 `gpt-5.6-terra` / `ultra` 只读调查；
Codex Sol 独立复核输入哈希、分区、AST 对账和高风险源码锚点。没有导入或启动旧
FastAPI app，没有访问数据库、网络、凭据、真实用户数据，也没有执行支付、OAuth、
MCP、Webhook、迁移或部署。

本证据冻结源码事实，不证明部署 SHA、运行配置、真实支付、消息送达、OAuth 交换或
线上可达性。`200/202`、`accepted`、`queued`、`draft`、`preview`、`simulated`、
`blocked`、`retired` 与 fixture 状态均不得写成生产执行成功。

## 781 条路由的严格补集

P1-S04 不是按宽泛域名重新选择，而是 P1-S01 781 条记录扣除 P1-S02 的 156 条和
P1-S03 的 184 条后的严格补集。早期 311 条调查口径已废弃，不能当完整范围。

可重放谓词为：S02 选择 owner `customer_read_model/customer_tags/identity_contact/
sidebar_write/admin_auth/admin_config/admin_jobs`，或 `route_name=oauth_token`。S03 选择
owner `ai_audience_ops/auth_wecom/channel_entry/ops_enrollment/send_content`；另选择
automation-engine 中 path 等于或位于 `/api/admin/automation-conversion/group-ops`、
`/api/automation/group-ops`、`/api/admin/channels`、`/api/admin/channel-welcome-materials`、
`/api/admin/wecom-customer-acquisition-links`、`/admin/channels`、
`/admin/automation-conversion/group-ops` 下的记录；以及 platform-foundation 中 path 等于
或位于 `/api/admin/external-effects`、`/api/external-effects`、
`/api/admin/push-center` 下，或精确等于 `/api/admin/wecom/execution-diagnostics` 的记录。

| 分区 | 数量 | overlap | uncovered |
|---|---:|---:|---:|
| P1-S02 contact/auth/admin | 156 | 0 | 0 |
| P1-S03 wecom/segment/outbound | 184 | 0 | 0 |
| P1-S04 strict complement | 441 | 0 | 0 |
| 总计 | 781 | 0 | 0 |

441 条按 manifest owner 为：

| owner | 数量 | owner | 数量 | owner | 数量 |
|---|---:|---|---:|---|---:|
| admin_shell | 2 | ai_assist | 9 | automation_agents | 13 |
| automation_engine | 17 | class_user_management | 1 | cloud_orchestrator | 37 |
| commerce | 106 | common_operation_members | 2 | data_health | 3 |
| delivery_lineage | 5 | hxc_dashboard | 16 | integration_gateway | 2 |
| media_library | 35 | message_archive | 11 | operation_cycles | 19 |
| owner_migration | 9 | platform_foundation | 23 | public_product | 16 |
| questionnaire | 42 | radar_links | 36 | service_period | 37 |

manifest 分类中 424 条 `external_effects=none`、13 条 `staging_disabled`、4 条
`real_requires_approval`；379 条要求认证，62 条为 public/callback/integration 面；109
条标为 financial、87 条 sensitive。`external_effects=none` 只是 manifest 声明，不能
覆盖源码中受 gate 控制的 adapter 或仅创建 execution intent 的行为。

## 静态源码对账

只以 `ast.parse` 解析 `router_registry.py` 明示的以下 36 个 handler，不 import app：

```text
aicrm_next/app/admin_console/routes.py
aicrm_next/extensions/ai/ai_assist/api.py
aicrm_next/extensions/ai/automation_agents/admin_api.py
aicrm_next/extensions/ai/automation_agents/admin_pages.py
aicrm_next/extensions/ai/automation_agents/api.py
aicrm_next/automation/automation_engine/api.py
aicrm_next/extensions/hxc/class_user_management/api.py
aicrm_next/extensions/growth/cloud_orchestrator/api.py
aicrm_next/extensions/commerce/commerce/api.py
aicrm_next/extensions/commerce/commerce/external_orders.py
aicrm_next/extensions/commerce/commerce/coupons/admin_api.py
aicrm_next/extensions/commerce/commerce/coupons/admin_pages.py
aicrm_next/extensions/commerce/commerce/coupons/public_api.py
aicrm_next/extensions/commerce/commerce/coupons/sidebar_api.py
aicrm_next/common_operation_members.py
aicrm_next/insights/data_health/api.py
aicrm_next/insights/delivery_lineage/api.py
aicrm_next/extensions/hxc/hxc_dashboard/api.py
aicrm_next/channels/integration_gateway/api.py
aicrm_next/engagement/media_library/admin_pages.py
aicrm_next/engagement/media_library/api.py
aicrm_next/extensions/archive/message_archive/api.py
aicrm_next/extensions/hxc/operation_cycles/admin_pages.py
aicrm_next/extensions/hxc/operation_cycles/api.py
aicrm_next/crm/owner_migration/api.py
aicrm_next/platform/platform_foundation/api.py
aicrm_next/platform/platform_foundation/execution_runtime/api.py
aicrm_next/platform/platform_foundation/internal_events/api.py
aicrm_next/platform/platform_foundation/webhook_inbox/api.py
aicrm_next/platform/platform_foundation/verification_files.py
aicrm_next/extensions/commerce/public_product/api.py
aicrm_next/extensions/forms/questionnaire/admin_pages.py
aicrm_next/extensions/forms/questionnaire/api.py
aicrm_next/extensions/radar/radar_links/admin_pages.py
aicrm_next/extensions/radar/radar_links/api.py
aicrm_next/extensions/commerce/service_period/api.py
```

结果是 441 个 route decorator calls；manifest 展开为 481 个 canonical method-path，
源码展开为 503 个 unique method-path，覆盖 `481/481`、缺失 0。额外 22 个辅助对为
11 个 HEAD 和 11 个 OPTIONS，不新增 P1 route record：

- HEAD：commerce 八个 unknown/wildcard、public product 三个 wildcard；
- OPTIONS：class export 1、cloud audit/observability 2、HXC unknown 1、commerce
  wildcard 4、message send/broadcast/archive-sync 3。

registry container owner 与 manifest logical owner 不能互相覆盖：
`common_operation_members` 两条放在 platform-foundation registry spec，但 manifest owner
是 common_operation_members；`POST /api/admin/ai-assist/review-plans` 位于
cloud-orchestrator handler/container，而 manifest owner 是 ai_assist。

## 行为事实

### AI、Automation 与 orchestration

- AI-assist external/admin/direct-send API 生成建议、review plan 或本地记录；review plan
  初态为 `pending_review/draft`，`broadcast_job_created=false`、
  `real_external_call_executed=false`。来源：
  `extensions/growth/cloud_orchestrator/review_plans.py:15-65`。
- review-plan decorator 实际在 cloud-orchestrator handler；逻辑 owner/container 的差异
  进入 G1，不能因文件位置擅自改 owner。
- automation-agents 提供配置、版本、运行记录和 audience webhook；provider、prompt、
  enrollment 与执行结果仍需各自契约，静态存在不证明可用 AI provider。
- automation-engine 本补集中的旧 customer-automation webhook 多条固定返回 410，且
  `real_external_call_executed=false`；源码 retired 响应是事实，但产品 disposition 仍为
  UNREVIEWED。来源：`automation/automation_engine/api.py:43-101,240-259`。
- cloud-orchestrator 与 operation-cycle 的 command/run 接口可返回 plan、intent、
  accepted 或 draft；它们不等于外部动作已执行。actor、reason、confirm、idempotency、
  expected version 与 receipt 必须在 v2 重新冻结。

### Questionnaire、OAuth 与身份

- questionnaire 覆盖 CRUD、H5、提交、OAuth、external push logs。提交成功只证明本地
  command/record；identity 仍须通过 v2 `identity.Ingest`，不得沿用外部 ID 猜测归因。
- H5 OAuth OPTIONS 暴露 `real_blocked/fake` gate；start 只有非 blocked payload 才允许
  跳转，callback eligibility 委托导入的 query/service。实际 token/userinfo adapter 未在
  本调查展开；state、cookie、return URL 与 openid/unionid scope 均保持 G1。
- external push、问卷 tag 与 automation enrollment 必须保持 outbox/至少一次和幂等
  分层；HTTP 成功不能替代 provider receipt。

### Commerce、公开页与服务期

- commerce 106 条包含订单、支付、优惠券、企微商店和外部订单；public-product 与
  service-period 另有 53 条。旧系统大量以 order number、openid/mobile/unionid 串联，
  v2 不得把这些字段提升为 customers 主键。
- public product 的写 wildcard 固定返回 410 blocked；读/HEAD wildcard 只有识别为旧
  payment action 时返回 410，其余可返回 list/product/404。HEAD 是源码辅助 method，
  blocked compatibility surface 不等于产品已批准删除。
- WeChat Pay OAuth、JSAPI order create/status 和 service-period payment 只有在配置、
  HTTPS、身份、审批与 provider gate 通过后才可能真实外调；本调查没有运行。订单创建、
  provider accepted、支付完成、权益生效是不同状态和证据。
- coupons 的 availability/claim、sidebar read 与 admin CRUD 也不能把 fixture/preview 当
  库存或核销事实；财务/PII 字段需要 owner scope 和审计签字。

### Media、Radar 与 archive

- media library 覆盖图片、附件、小程序和群邀请素材的 admin CRUD/read；文件 URL、
  下载/缩略图和外部对象读取需要受控 URL、大小、类型、SSRF 与审计门。
- radar link/content/click 与 H5 OAuth 分层；click record 不等于客户意图或 conversion，
  OAuth start/callback 仍是 `staging_disabled` 的外部门。
- message archive 的 send、broadcast、archive-sync 三条路由无条件委托 imported
  `blocked_messages_side_effect`，并含 OPTIONS；本范围未验证 helper 的精确 body/status。
  读/搜索/archive 状态也不证明会话存档许可或数据完整性。

### Platform、Ops、Gateway 与诊断

- execution-runtime 只读 lane/timeline；internal-events 与 webhook-inbox 的 run、dispatch
  在 handler 默认 dry-run。retry/skip 与非 dry-run command 委托 action-token、actor、
  reason、version/CAS parser/service 校验；哪些字段逐项必填仍由 G1/契约测试冻结。
  `202` 只是 execution intent/queue command 接受，响应固定不证明外部调用。
- webhook-inbox dispatch/run-due 虽标 `staging_disabled`，仍必须先通过静态/权限/幂等
  门；重复 callback、重复 job 与 provider 重复效果是三个不同问题。
- health、data-health、delivery-lineage、owner-migration 与 HXC dashboard 是观测/迁移
  控制面；静态检查、计数或 reconciliation 不能自动执行修复。
- common operation-member sync 标 `real_requires_approval`；读取成员与真实同步分开。
- `/mcp` GET/POST 标 `staging_disabled`，旧客户端兼容性只能由原客户端 harness 验证，
  不能据 handler 存在宣称 MCP 可用。

## 差异、禁止声明与 G1

| id | 静态事实 | 当前处理 |
|---|---|---|
| S04-D01 | 441/481/503 三种计数语义不同 | 分列保存，禁止混写 |
| S04-D02 | 22 个 source-only HEAD/OPTIONS | 保留辅助 method，不加 route record |
| S04-D03 | 两类 registry container/logical owner 差异 | G1 owner 签字 |
| S04-D04 | manifest none 与 gated adapter/intent 有张力 | 源码与 manifest 并列 |
| S04-D05 | fixed 404/410 与 blocked-helper 路由仍物理注册 | 不自动标 DEPRECATED |
| S04-D06 | queued/accepted/draft/preview 不等于外部成功 | receipt/verification 分层 |
| S04-D07 | 财务、OAuth、MCP、PII 均未运行 | PENDING_EXTERNAL_GATE |

G1 至少确认：review-plan owner；AI provider/prompt/version；automation enrollment 与 action
幂等；问卷 OAuth/identity/tag；支付与权益状态机；优惠券核销；media URL/SSRF；radar
click 语义；archive 许可；queue manual action/CAS；operation-member sync；旧 MCP 客户端；
所有固定 404/410 的兼容策略。所有候选保持
`UNREVIEWED/PENDING_HUMAN_SIGNOFF`，不构成 MIGRATE、MERGED 或 DEPRECATED。

## 独立复核结果

```text
input_sha256=fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231
partition_total=781 s02=156 s03=184 s04=441 overlap=0 uncovered=0
handler_files=36 route_decorators=441 manifest_method_paths=481
source_method_paths=503 matched=481 missing=0 auxiliary=22
```

未执行旧 runtime、DB、网络、支付、OAuth、MCP、浏览器、staging 或 production。相应
项目保持 `PENDING_EXTERNAL_GATE`，不能以本静态报告替代。
