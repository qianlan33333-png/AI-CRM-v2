# P1-S07 剩余前端严格补集静态盘点

- Issue: #75
- Base SHA: `07735c1749b729db45c21f1a4bb766de33516d57`
- Legacy source: `origin/main@6cb989c071255437d75953dabb943318a74eb8f4`
- Route input: `docs/evidence/p1/legacy-routes-6cb989c.json`
- Route input SHA-256: `fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231`
- API complement input: `docs/evidence/p1/api-facts-upper-domains-6cb989c.md`
- Evidence level: `LOCAL` / static source only
- Disposition: `UNREVIEWED`
- Signoff: `PENDING_HUMAN_SIGNOFF`

## 结果

P1-S02 的 156 条与 P1-S03 的 184 条从 781 条 route records 扣除后，strict complement 为
441 条；三分区 overlap=0、uncovered=0。本 Slice 只从该补集提取页面验收候选，不改变 API
分区。

- 页面入口原始集合：47 条 `admin_page`（46 GET + 1 POST）与 1 条 H5。
- 最终追加 183 行 `LEGACY-S07-001..183`：43 个页面入口与 140 个页面内交互。
- `/admin/logout` 是指向 `/logout` 的兼容重定向；退出行为已由 `LEGACY-S05-037` 冻结，
  因此不重复建立独立候选。
- owner-migration GET/POST、问卷列表/UI alias、问卷 new/detail 编辑器、全局/单问卷 push logs
  分别按单一页面行为合并。
- 优惠券、雷达、周期商品、微信支付商品的 new/edit 即使共用模板仍分列，避免静默合并语义。

第二轮按“页面 → 区块 → 用户动作 → API → 静态预期”补齐交互：

| 范围 | 交互行数 |
|---|---:|
| 素材库 / 自动化话术 | 27 |
| Cloud campaign / plan / observability | 22 |
| 优惠券 / HXC | 17 |
| owner migration / 运营闭环 | 8 |
| 问卷 / 内容雷达 | 25 |
| 周期商品及会员网格 | 21 |
| 微信支付商品 / 订单交易退款 | 20 |
| 合计 | 140 |

## 路由与源码口径

- Manifest canonical method pairs：481。
- Source decorator method pairs：503；覆盖 manifest 481/481，missing=0。
- Source-only auxiliary：22，即 11 HEAD + 11 OPTIONS；只作 method-bundle delta，不新增矩阵行。
- 43 个入口候选覆盖排除 `/admin/logout` 后的 47 个页面入口 method pairs；公开 H5 另显式列出
  bootstrap/query 两个客户端 API method pairs。
- 140 个交互候选逐行保存客户端动作与源码范围；其中 157 个非 client-only method/path
  项均与 781 路由权威匹配。直接模板和静态依赖以逐行 `source_evidence` 为准，不另声明
  无可重放清单支撑的全局文件计数。
- `questionnaire_state.html` 只用于错误占位分支，不作为独立候选。

静态解析只覆盖 registry 明示的 36 个 handler：

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

## 必须保留的边界

- 页面 200、列表计数、详情或看板只证明旧静态读模型，不证明 runtime、production 或外部真源。
- AI 计划的 draft/pending review、action token 和审批 UI 不证明 provider 调用、发送或送达。
- 订单、支付宝、微信支付、微信小店、优惠券和周期商品页面不作为付款、退款、核销或权益事实。
- owner-migration 的页面 POST 是唯一 `admin_page` 写方法；本 Slice 不授权真实迁移或生产写入。
- 问卷、雷达和公开会员网格不证明 OAuth、OneID、点击归因、外推 receipt 或分享 token 可用。
- 素材页、群发配置与公开 H5 只冻结页面及客户端请求形状，上传、对象 URL、安全检查和
  外部访问均待 G1。

## G1 待签字

1. 183 行是否覆盖真实菜单、角色、隐藏按钮、移动端及兼容入口。
2. AI 助手、自动化话术、运营闭环的页面动作与真实外部效果边界。
3. 支付、优惠券、订单、周期权益和公开分享的产品/权限/财务语义。
4. 问卷、雷达、素材与 owner migration 的身份、写权限和外部链路。
5. source-only HEAD/OPTIONS 是否需要保留，以及共用模板的 new/edit 拆分。

第一轮调查 scope exceptions=0。第二轮为解析交易页注入的 `listApiUrl/exportApiUrl` 与动态
`download_url`，额外只读 `aicrm_next/extensions/commerce/commerce/api_support.py:282-352`
和 `aicrm_next/extensions/commerce/commerce/admin_exports.py:62-85`；相关结论只用于交易页行证。
未运行旧 app、浏览器、数据库、网络、provider、staging、production 或部署。所有 G1 项均为
`PENDING_EXTERNAL_GATE`。
