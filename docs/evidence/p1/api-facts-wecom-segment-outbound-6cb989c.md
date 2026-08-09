# P1-S03 企微、人群与外发旧 API 静态事实

- Issue: #67
- Legacy source: `origin/main@6cb989c071255437d75953dabb943318a74eb8f4`
- P1 input: `docs/evidence/p1/legacy-routes-6cb989c.json`
- P1 input SHA-256: `fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231`
- Input manifest SHA-256: `3bb11a48c8bbc520fb9da5128726594232bca0b4e0f0c7ed1f63bb4b3c2263bd`
- Evidence level: `LOCAL` / static source only
- Disposition: `UNREVIEWED`
- Signoff: `PENDING_HUMAN_SIGNOFF`

## 调查边界

Terra task `/root/p1_s03_wecom_segment_outbound` 使用 `gpt-5.6-terra` / `ultra`
只读调查；Codex Sol 独立复核输入哈希、分区计数、AST 对账和高风险源码锚点。
没有导入或启动旧 FastAPI app，没有访问数据库、网络、浏览器、凭据、真实用户数据，
也没有执行企微、Webhook、部署或迁移。

调查曾有一次潜在范围例外：从 cwd `/Users/qianlan/Downloads/新CRM` 执行
`rg -n "def assert_run_due_guard|assert_run_due_guard" "$LEGACY/aicrm_next" -g '*.py'`，
exit `0`。输出只含允许文件中的目标符号，未输出禁读路径或内容；本证据的结论均由
下列显式文件重新取得，不依赖该次广域遍历。该例外不能被描述为完全符合最小读取面。

本证据冻结源码事实，不证明线上部署 SHA、运行配置、provider 请求、消息送达或
真实数据。`202`、`accepted`、`queued`、`provider_accepted`、`simulated` 和
`record_only` 均不得被写成“已发送”。

## 781 条路由的唯一分区

P1-S02 已冻结 156 条。P1-S03 选择：

1. owner 全量：`ai_audience_ops`、`auth_wecom`、`channel_entry`、
   `ops_enrollment`、`send_content`；
2. `automation_engine` 中 path 以前缀
   `/api/admin/automation-conversion/group-ops`、`/api/automation/group-ops`、
   `/api/admin/channels`、`/api/admin/channel-welcome-materials`、
   `/api/admin/wecom-customer-acquisition-links`、`/admin/channels`、
   `/admin/automation-conversion/group-ops` 开始的记录；
3. `platform_foundation` 中 path 以前缀 `/api/admin/external-effects`、
   `/api/external-effects`、`/api/admin/push-center` 开始，或精确等于
   `/api/admin/wecom/execution-diagnostics` 的记录。

结果固定为：

| 分区 | 数量 | 说明 |
|---|---:|---|
| P1-S02 | 156 | contact/auth/admin，已冻结 |
| P1-S03 | 184 | 本证据 |
| P1-S04 | 441 | 严格补集，后续单独冻结 |
| 总计 | 781 | overlap `0`，uncovered `0` |

P1-S03 按 owner 为：AI Audience 66、automation engine 59、platform foundation
22、User Ops 18、channel entry 7、Auth WeCom 6、Send Content 6。P1-S04 必须使用
该补集，不能沿用早期有交叠的 311 条调查口径。

## 静态源码对账

Sol 使用 Python 标准库 `ast.parse`，只解析以下 15 个 handler 文件，按 path、route
name 和 manifest method 包含关系得到 `184/184`，未匹配 `0`：

```text
aicrm_next/channels/channel_entry/api.py
aicrm_next/channels/auth_wecom/api.py
aicrm_next/automation/automation_engine/channels_api.py
aicrm_next/automation/automation_engine/channel_admin_pages.py
aicrm_next/automation/automation_engine/group_ops/api.py
aicrm_next/automation/automation_engine/group_ops/admin_pages.py
aicrm_next/automation/ops_enrollment/api.py
aicrm_next/automation/ops_enrollment/admin_pages.py
aicrm_next/engagement/send_content/api.py
aicrm_next/extensions/ai/ai_audience_ops/api.py
aicrm_next/extensions/ai/ai_audience_ops/admin_api.py
aicrm_next/extensions/ai/ai_audience_ops/external_api.py
aicrm_next/extensions/ai/ai_audience_ops/admin_pages.py
aicrm_next/platform/platform_foundation/external_effects/api.py
aicrm_next/platform/platform_foundation/push_center/api.py
```

明确禁止的 `automation/automation_engine/group_ops/repo.py` 未作为证据读取。

P1-S01 保存 manifest canonical methods；源码另有 7 条辅助 method bundle 差异：

| 路由族 | 记录数 | manifest | source decorator |
|---|---:|---|---|
| 获客链接 collection/detail/action | 3 | 业务 methods | 加 `OPTIONS` |
| 两个企微 callback | 2 | GET/POST | 加 `OPTIONS/HEAD` |
| Auth WeCom start/callback | 2 | GET | 加 `OPTIONS` |

辅助 method 不生成额外 P1 路由记录，但不能从源码事实中抹除。

## 行为事实

### Callback、Auth 与渠道

- 两个 callback 的同步边界是验签、解密、持久化 inbox 和 ACK。重复 callback 由
  corp/event/change type/external user/user/time/welcome/state 等组成的稳定键去重；
  welcome、tag、identity sync 是后续计划，不是同步 provider 写入。
- inbox 最多尝试 8 次，租约丢失可形成 `unknown/lost_lease`。这是 inbox 状态，不能
  与外部写已经跨 provider 后的 `unknown_after_dispatch` 混写。
- Auth start 生成授权 URL/302；callback 在显式配置 gate 通过后才可能执行 token 与
  user-info adapter。OAuth state 重放、cookie/session 与真实交换均未运行，保持 G1。
- 获客链接 API 是内存 fixture + blocked plan；`wecom_api_called=false`，不是企微
  获客链接真源。标准 channel CRUD 是本地 resource；非 PostgreSQL 的空 contacts 或
  materials 不能解释为真实“没有数据”。
- QR generate 静态存在 contact-way adapter；QR download 与 lesson-card cover 存在外部
  读取路径。是否可达、URL trust、SSRF 与 provider receipt 均未验证。

### Group Ops 与 User Ops

- Group Ops 覆盖 plan/group/node/rule/member/segmentation、sync、run-due、webhook 和
  token broadcast。普通控制面默认 `real_external_call_executed=false`。
- 非 preview run-due 和 token broadcast 构造确定性 External Effect graph，可返回
  `202 queued/accepted` 与 job id；HTTP handler 不直接拥有企微 provider adapter。
- public webhook duplicate 只证明 action bookkeeping 幂等，不证明 provider 未重复；
  sender、recipient、approval、消息 ID 和 failed recipient 仍需真实 receipt。
- group sync 可更新本地群资产，但声明 `no_outbound_send=true`；fake/snapshot 资产不可
  迁移为真实企微事实。
- User Ops batch execute 强制 confirm，但当前公开路径只创建/reuse
  `pending_review/draft` AI-assist review plan，broadcast job 为 0、effect ids 为空、
  sent 为 0。潜在 enqueue helper 不等于该 API 已调用它。
- Send Content 六条 API 只做内容包/素材 normalize、validate、preview/read；未发现直接
  provider send。DND 是本地状态，send record 也不提供真实企微送达保证。

### Segment / AI Audience

- package/version 初始为 draft/paused；publish、activate、refresh 产生 refresh intent、
  membership diff 与 internal event，不直接发送。
- inbound webhook 以 external event id 或 payload hash 去重，持久化 record 与 internal
  outbox；初始结果是 `record_only=true`、`real_external=false`。
- public outbound subscription configuration API 静态返回
  `410 webhook_configuration_retired`；不能迁移旧订阅为已启用配置。
- outbound planner 只能排入 `WEBHOOK_GENERIC_PUSH` 等 External Effect；测试/e2e 是
  allowlist 控制的 simulated/record-only 输入。
- send record 只有 `side_effect_executed` 与 `provider_result_received` 同时为真才可称
  sent；跨边界结果不确定时保持 `unknown_after_dispatch`，simulated 仍是 simulated。
- 旧定义含动态 SQL/外部身份与 OFFSET 行为；v2 必须经冻结 DSL、identity port 与
  keyset 重写，不能逐文件搬运旧架构或把旧成员快照当新结果。

### External Effects 与 Push Center

- effect 状态至少区分 planned、approved、queued、dispatching、succeeded、simulated、
  unknown_after_dispatch、failed、blocked、cancelled、expired。
- run-due 默认 dry-run；非 dry-run、retry、cancel 需要 action token、actor、reason 与
  状态/CAS 约束。`202` 只表示 queue command 接受。
- worker 在 provider 前持久化 attempt。adapter exception 或 provider-result 持久化失败
  进入 `unknown_after_dispatch`，不得自动恢复为可发送或盲目重试。
- unknown retry 需要 actor、reason 和 duplicate-risk confirmation；cancel 只在 provider
  前可安全终止。Push Center 是投影视图，`provider_accepted` 不等于 delivered。

## 差异与禁止声明

| id | 静态事实 | 当前处理 |
|---|---|---|
| S03-D01 | 7 条 manifest/source auxiliary-method 差异 | 两列并存 |
| S03-D02 | 184 条与 S02/S04 必须组成唯一分区 | 永久引用精确谓词 |
| S03-D03 | callback ACK 与后续 effect 不同事务/阶段 | 禁止称同步发送 |
| S03-D04 | fixture、blocked、simulated 与真实 provider 状态不同 | 不迁移为运行事实 |
| S03-D05 | queued/202/provider accepted 不等于 delivered | receipt 单独验收 |
| S03-D06 | unknown_after_dispatch 有重复风险 | 只归档/对账/人工处置 |
| S03-D07 | manifest `none/staging_disabled` 与 gated source path 可有张力 | G1 安全签字 |
| S03-D08 | 一次广域 rg 可能遍历禁读范围 | 已记录；结论零依赖 |

## v2 候选边界与 G1

候选模块边界是 wecom callback/read adapter、identity attribution、segment DSL/materialized
members、outbound durable task/attempt/receipt 和受控 Extension webhook。所有候选均为
`UNREVIEWED/PENDING_HUMAN_SIGNOFF`，不构成 MIGRATE、MERGED、DEPRECATED 或实现状态。

G1 至少确认：callback 幂等与 ACK 时限；OAuth state/cookie；corp/owner scope；QR 与外部
素材 URL trust；群消息 sender/recipient/频率/审批；User Ops review 后何时建真实任务；
Audience SQL/身份/snapshot 语义；Webhook allowlist/HMAC/SSRF；provider accepted 与送达
证据；unknown retry 的 duplicate-risk 人工确认；固定 410 配置面的兼容策略。

## 独立复核结果

```text
input_sha256=fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231
partition_total=781 s02=156 s03=184 s04=441 overlap=0 uncovered=0
static_ast_targets=184 matched=184 unmatched=0 method_deltas=7
```

未执行旧 runtime、DB、网络、真实 provider、浏览器、staging 或 production。相应项目
保持 `PENDING_EXTERNAL_GATE`，不能以本静态报告替代。
