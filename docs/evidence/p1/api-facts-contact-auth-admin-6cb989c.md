# P1-S02 contact/auth/admin 旧 API 静态事实

- Issue: #65
- Legacy source: `origin/main@6cb989c071255437d75953dabb943318a74eb8f4`
- P1 input: `docs/evidence/p1/legacy-routes-6cb989c.json`
- P1 input SHA-256: `fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231`
- Input manifest SHA-256: `3bb11a48c8bbc520fb9da5128726594232bca0b4e0f0c7ed1f63bb4b3c2263bd`
- Evidence level: `LOCAL` / static source only
- Disposition: `UNREVIEWED`
- Signoff: `PENDING_HUMAN_SIGNOFF`

## 调查边界

Terra task `/root/p1_s02_contact_auth_admin` 使用 `gpt-5.6-terra` / `ultra`
只读调查；Codex Sol 独立复核基线、范围计数和高风险源码行。没有导入旧 FastAPI
app、启动服务、访问数据库/网络/凭据、执行外部调用或部署。

本证据冻结“源码存在什么”，不证明部署 SHA、运行配置、真实数据、provider 结果或
线上实际流量。源码中的 `retired`、404、410、fixture、blocked 也不自动构成产品的
`DEPRECATED/MERGED` 决策。

## 可重放范围

从 P1-S01 的 781 条记录选择：

- `capability_owner` 为 `customer_read_model`、`customer_tags`、
  `identity_contact`、`sidebar_write`、`admin_auth`、`admin_config`、
  `admin_jobs`；或
- `route_name=oauth_token`。

该谓词精确得到 156 条：

| manifest owner | 数量 | 主要 registry groups |
|---|---:|---|
| customer_read_model | 32 | customer_read_model, customer_admin_pages |
| customer_tags | 14 | customer_tags_read/write/general/admin_pages |
| identity_contact | 10 | identity, identity_admin_pages, sidebar_jssdk |
| sidebar_write | 8 | sidebar_write |
| admin_auth | 4 | admin_auth |
| admin_config | 69 | admin_config, admin_config_api_key |
| admin_jobs | 18 | admin_jobs |
| platform_foundation | 1 | auth_platform (`oauth_token`) |

静态 `ast.parse` 只解析下列 13 个 handler 文件，按 path、route name 和 method
包含关系完成 156/156 decorator 对账，未匹配为 0：

```text
aicrm_next/crm/customer_read_model/admin_pages.py
aicrm_next/crm/customer_read_model/api.py
aicrm_next/crm/customer_tags/admin_pages.py
aicrm_next/crm/customer_tags/api.py
aicrm_next/crm/identity_contact/admin_pages.py
aicrm_next/crm/identity_contact/api.py
aicrm_next/crm/identity_contact/sidebar_jssdk.py
aicrm_next/crm/sidebar_write/api.py
aicrm_next/platform/admin_auth/api.py
aicrm_next/platform/admin_config/api.py
aicrm_next/platform/admin_config/direct_api_key_api.py
aicrm_next/platform/admin_jobs/routes.py
aicrm_next/platform/platform_foundation/auth_platform/api.py
```

## Method bundle 差异

P1-S01 保存 manifest canonical methods；源码 decorator 另有 19 条辅助方法差异，
不能把两列覆盖成“完全一致”：

| 范围 | 数量 | manifest | source decorator | 静态行为 |
|---|---:|---|---|---|
| customer tag writes | 8 | business methods | 加 `OPTIONS` | `204` |
| sidebar writes | 8 | POST/PUT | 加 `OPTIONS` | `204` |
| sidebar JSSDK | 1 | GET | GET/HEAD/OPTIONS | HEAD `204`; OPTIONS diagnostics |
| sidebar OAuth | 2 | GET | GET/OPTIONS | blocked diagnostics |

例如 tags 的 source decorator 位于
`aicrm_next/crm/customer_tags/api.py:107-153`；OAuth 位于
`aicrm_next/crm/identity_contact/sidebar_jssdk.py:186-280`。

## 行为事实

### Customer read model（32）

- `/api/customers` 使用 owner、tag、status、binding、mobile、keyword、`limit`、
  `offset`；响应携带 total、watermark、filters 和 fallback/source status。
- detail、timeline、recent messages 仍以 external_userid/unionid 为主要 URL 或查询键；
  找不到返回 404。v2 必须经 identity/OneID 收敛，不能继承这些外部 ID 主键。
- sidebar v2 timeline/materials 仍用 OFFSET；thumbnail 可能返回 302、304、二进制、
  404 或 429，并可能只调度后台缩略图任务。
- production read model 失败时，若开关允许会走 live-source fallback；两者失败为 503。
  非生产路径可使用 in-memory fixture，空集合不能解释成真实“没有客户”。
- profile questionnaire helper 直接构造 sidebar SQL repository，是需在 v2 消除的
  API-to-store 边界例外，不是推荐架构。

### Customer tags（14）

- catalog 读返回 tags/groups/count；production read unavailable 可用 HTTP 200 + 空集合
  与错误字段表达，不能把 200 当作数据成功。
- writes 传播 `Idempotency-Key`、actor、dry_run、trace；输入、not found、production
  unavailable 分别落 400/404/503，响应固定不声称已真实外调。
- sync 失败可返回 502 `wecom_tag_sync_failed`。mark/unmark 只计划 effect/job，
  返回 queued/blocked 与 `real_external_call_executed=false`。
- manifest 为 `staging_disabled`，源码又存在受 production-ready gate 控制的 live branch；
  二者是待签字张力，不是已启用证明。

### Identity 与 sidebar OAuth/JSSDK（10）

- resolve 接受 external_userid/mobile/openid/unionid；not found 为 404，
  multiple/conflict/pending 为 409，不能挑一个猜测。
- binding routes 校验 token/cookie/request state；缺失/无效为 401，owner/customer
  mismatch 为 403，production projection unavailable 为 503。
- context token 以服务端 viewer context 为准；corp mismatch 为 403。JSSDK 输入错误
  为 400、配置错误为 502。
- OAuth start 在未启用/缺配置时为 503，成功只 302 到 provider URL；callback 源码
  包含 access-token/user-info client 调用。是否可达依赖未验证配置与凭据，不能写成
  production 外调已发生。

### Sidebar write（8）

- wrapper 必填 external_userid，以受信 token/cookie 覆盖 body 自报 owner/actor；冲突
  依次可能为 400/403/404/409/503。
- 只有 BindMobile、UpdateSidebarProfile、PlanMaterialSend 在 production-ready allowlist；
  其余五个 command 被生产 gate 拒绝为 503。来源：
  `aicrm_next/crm/sidebar_write/application.py:54-58`。
- 命令只创建本地写入、审计、internal event 或 blocked/approval-required effect plan；
  不得把 plan/queued 解释成企微已执行。

### Admin auth/config（73）

- login/logout 为浏览器 integration：可能 HTML 或 302；logout revoke session 并清 cookie；
  OPTIONS 只返回 diagnostics。
- API-client/direct-key create/rotate/disable 需要权限、action token 与 confirm；secret 只在
  创建/轮换时返回并使用 `Cache-Control: no-store`，不得进入日志或本证据。
- config release publish 要 checksum，rollback 只允许 active published release；公开
  projection 必须遮罩 sensitive values。
- routing、signup-tags、class-term-tags 共 7 个 API 在源码固定 404 `... is retired`
  （`platform/admin_config/api.py:1363-1395`）。产品仍需决定是否保留兼容响应、迁移
  或删除，当前 disposition 保持 UNREVIEWED。
- MCP tool 与 signup-conversion 的 wrapper 未显式调用通用 action-token/confirm helper，
  而 manifest 标记 csrf=true；v2 必须显式冻结权限，不推断 middleware 已补齐。

### Admin jobs（18）

- dashboards/lists 多为 `limit/offset`；broadcast list 最大 200，detail events 最多 100
  并返回截断标记。v2 必须改成 ADR-006 keyset cursor。
- archive sync 的 confirm=false 是 preview；confirm 后默认 in-process 开关关闭时返回
  error payload，而 handler 不一定转换成非 2xx。
- approve/cancel 要 reason 与正 expected_version，但 endpoint 调 application 时未传递
  expected_version；不能据请求校验宣称 CAS 已执行。
- order identity repair 路由仍物理注册，但固定 410/retired；来源
  `aicrm_next/platform/admin_jobs/routes.py:242-252`。
- Feishu hourly route 直接调用 sender orchestration；底层在配置、allowlist、SSRF gate
  通过后存在真实 HTTPS sender path。manifest 却标 `external_effects=none`，必须作为
  discrepancy，不得自动归为无外部效果。

### Client credentials token（1）

- `/oauth/token` 只接受 client_credentials；生产非 HTTPS 为 400，Basic/form client
  authentication 失败为 401/403，成功响应 access token、TTL、scope 与 no-store。
- service 静态校验 enabled、secret、source IP CIDR、audience、scope 后签 JWT；没有
  fixture fallback 证据。

## 必须保留的差异

| id | 事实 | 当前处理 |
|---|---|---|
| S02-D01 | 19 条 manifest/source auxiliary-method 差异 | 两列并存 |
| S02-D02 | Feishu hourly manifest effects=none，源码有 gated HTTPS path | G1 安全签字 |
| S02-D03 | tags/OAuth manifest staging-disabled，源码有 gated live branch | G1 运行边界签字 |
| S02-D04 | order repair 物理注册但 runtime owner retired、响应 410 | G1 产品签字 |
| S02-D05 | 旧列表/timeline 广泛使用 OFFSET | v2 OpenAPI 改 keyset，不冒充等价 |
| S02-D06 | fixture/degraded 200 可携带空集合 | 保留 source status/error 字段 |

## v2 候选边界与 G1

候选聚合是 contact query、tag catalog/command、identity resolution、sidebar owner
session、admin config release、admin jobs query/manual transition、OAuth client token。
候选不构成 `MIGRATE/MERGED/DEPRECATED` 或实现状态。

G1 至少确认：identity key precedence/conflict；sidebar OAuth/JSSDK；tag live adapter；
五个 production-blocked sidebar command；secret issuance 与 config publish 权限；Feishu
delivery receipt；broadcast CAS；固定 404/410 的兼容策略；OFFSET 到 keyset 的可见行为差异。

## 独立复核命令

```sh
shasum -a 256 docs/evidence/p1/legacy-routes-6cb989c.json

jq '[.routes[] | select(
  (.capability_owner == "customer_read_model") or
  (.capability_owner == "customer_tags") or
  (.capability_owner == "identity_contact") or
  (.capability_owner == "sidebar_write") or
  (.capability_owner == "admin_auth") or
  (.capability_owner == "admin_config") or
  (.capability_owner == "admin_jobs") or
  (.route_name == "oauth_token"))] | length' \
  docs/evidence/p1/legacy-routes-6cb989c.json
# 156
```

未执行 runtime、DB、网络、真实 provider、浏览器、staging 或 production 验证；这些
均保持 `PENDING_EXTERNAL_GATE` 或后续 G1。
