# AI-CRM v2 Canonical Architecture

## 1. 文档地位

本文是 AI-CRM v2 的 canonical architecture。发生冲突时，执行优先级为：

1. 用户最新书面指令；
2. 已接受的 ADR 与本文；
3. [`AI-CRM-v2-重构详细设计.md`](../spec/AI-CRM-v2-重构详细设计.md)；
4. [`AI-CRM-v2-执行方案.md`](../spec/AI-CRM-v2-执行方案.md)；
5. 单个 Slice 任务卡。

ADR 只能显式消解冲突，不能静默删减需求或验收标准。尚未由真实环境验证的能力必须标记为 synthetic、local 或 `PENDING_EXTERNAL_GATE`，不能写成生产验证通过。

## 2. 系统目标与约束

AI-CRM v2 是单企业私有部署的 CRM 重构，目标是在保持线上 `aicrm_next` 既有业务行为的前提下，建立可机械验证的模块边界、统一身份图谱、可靠异步任务和可回放的行为契约。

### 2.1 功能约束

- 现有页面的每个按钮和能力都必须在功能矩阵中有对应项；UI 可以变化，行为不能静默缺失。
- 客户、标签、阶段、时间线、身份、企微同步、人群、自动化、外发、问卷、AI、网关、统计和运维能力均在模块化单体内实现。
- 支付、商品和客户专属小程序等定制件不进入核心仓库，只能通过 Extension API 接入。
- OpenAPI 是 HTTP 接口的唯一契约；服务端 stub 与前端 client 均由锁定生成器产生。

### 2.2 非功能约束

| 维度 | 基线 |
|---|---|
| 部署 | 单企业、单机、单库；一个二进制支持 `--role=api|worker|all` |
| 容量 | 10–20 万企微客户；后台并发用户不超过 15；API 峰值 50 QPS |
| 峰值输入 | 企微回调按 100 次/秒设计 |
| 批处理 | 单次外发最多 20 万任务；吞吐受企微限速而非横向扩展驱动 |
| 性能 | S 档 2C4G 上，20 万模拟客户的客户列表任意受支持筛选 P95 小于 200ms |
| 数据 | PostgreSQL 16 是唯一有状态组件；时间线按月分区、append-only |
| 查询 | sqlc；列表使用 keyset cursor；禁止深 OFFSET、ORM 和手拼 SQL |
| 运维 | 低运维复杂度；不引入 Redis、额外消息队列、微服务或 Kubernetes |
| 安全 | Secret 不入库、不回显；PII 不进入结构化日志；身份归因 fail-closed |

未在需求中给出可量化目标的可用性、RPO 和 RTO 不在本文臆造；部署前必须在 P6 外部门中冻结。当前授权不包含部署、真实数据迁移、真实企微调用或生产切换。

## 3. 高层架构

```text
Browser
   |
nginx (TLS)
   |
aicrm single binary
   +-- role=api: HTTP API, embedded React, WeCom callbacks
   +-- role=worker: River workers and periodic jobs
   +-- role=all: api + worker, intended for S tier
   |
PostgreSQL 16
   +-- domain tables
   +-- event_log transactional outbox
   +-- River job tables
```

生产建议在同一主机运行独立 `api` 与 `worker` 进程并使用独立 pgx 连接池；S 档允许 `all`。`api` 不注册 worker，`worker` 不开放业务 HTTP 端口。单一二进制和单库不等于允许模块绕过所有权边界。

固定工具链：Go 1.26.5、Node 24.18.0 LTS、npm 11.12.1、PostgreSQL 16.14。初始依赖锁定为 River 0.24.0、pgx 5.9.2、chi 5.2.3、oapi-codegen 2.6、sqlc 1.28、Goose 3.25、React 18.3.1、antd 5.27.6、Orval 7.21.0、Vite 7.3.6。oapi-codegen 2.6 生成的 Go server 额外固定其官方示例兼容 runtime 1.2.0；这是生成物运行依赖，不是生成器升级。pgx 5.9.2 是 [GO-2026-4772](https://pkg.go.dev/vuln/GO-2026-4772) 与 [GO-2026-5004](https://pkg.go.dev/vuln/GO-2026-5004) 的共同最低修复版本，替代有可达安全路径的 5.7.5。Orval 7.21.0 是 [GHSA-h526-wf6g-67jv](https://github.com/advisories/GHSA-h526-wf6g-67jv) 的 7.x 修复版本；锁文件强制 `js-yaml` 4.3.1 与 `lodash` 4.18.1，关闭生成器依赖链中已知的 high/critical 漏洞。Vite 7.3.6 是 [GHSA-v2wj-q39q-566r](https://github.com/advisories/GHSA-v2wj-q39q-566r)、[GHSA-p9ff-h696-f583](https://github.com/advisories/GHSA-p9ff-h696-f583) 与 [GHSA-fx2h-pf6j-xcff](https://github.com/advisories/GHSA-fx2h-pf6j-xcff) 的 7.x 安全修复版本，替代未进入可运行基线的 7.2.2。版本变更必须显式评审，不能由业务 Slice 顺手升级。

## 4. 模块与依赖方向

```text
cmd/aicrm                 composition root; 唯一装配具体实现的位置
api/openapi.yaml          唯一 HTTP 契约
internal/<domain>/port    域的跨模块输入/输出契约
internal/<domain>/app     用例编排
internal/<domain>/store   该域拥有表的持久化实现
internal/<domain>/http    HTTP adapter
internal/<domain>/worker  River adapter
internal/platform         pgx/River/HTTP/logging 等共享基础设施
web                       React 前端，构建后 go:embed
migrations                Codex 冻结并拥有的 Goose 迁移
```

域模块为：`contact`、`identity`、`segment`、`automation`、`outbound`、`wecom`、
`ai`、`survey`、`gateway`、`config`、`events`、`auth`、`stats`、`ops`。
`ops` 不拥有业务表；它只通过 platform queue/log/health port 和各域公开健康
port 提供有界运维视图及已授权的 River 重试/取消操作，不得直写其他域表。

依赖规则：

1. 跨域 Go import 只能指向 `internal/<domain>/port`，或通过已冻结的领域事件交互。
2. 其他域不得 import 对方的 `app`、`store`、`http`、`worker` 或生成物。
3. `cmd/aicrm` 是唯一可以同时引用多个具体实现并完成 wiring 的 composition root。
4. `internal/platform` 可被域模块依赖，但不能 import 任何域的具体实现。
5. 生成代码目录禁止手写；生成两次必须无 diff。

`port` 必须是独立 Go package。旧设计中把 `port.go` 与实现放在同一 package 的示例被本规则取代，因为文件名不能形成可机械检查的 import 边界。

## 5. 数据所有权铁律

| 所有者 | 独占写入对象 | 其他模块的合法方式 |
|---|---|---|
| contact | `customers`、`customer_tags`、`customer_events` 以及客户域表 | 调 contact port；合并时使用 transaction-bound `MergePort` |
| identity | `identities`、`customer_merges`、`pending_events` | 调 identity `Resolve/Bind/Ingest` |
| outbound | outbound 表与所有企微写 API | 调 `EnqueueOne/EnqueueBatch` |
| wecom | 企微读取、回调验签解密与同步游标 | 读取企微后把外部标识交给 identity；客户写入调 contact |
| events | `event_log` 与 dispatcher | 在业务事务中调 events `Append` |
| config | typed settings 与审计 | 通过强类型 config port；禁止散落 `os.Getenv` 或直查 settings |

额外铁律：

- 任何状态变更必须在同一 PostgreSQL 事务追加 `event_log`。
- 所有业务周期任务只通过 River periodic job 注册；禁止 `time.Ticker`、`time.AfterFunc` 和第三方 cron。
- AI 模块只生成内容，不决定发给谁或何时发送。
- 真实或客户定制 Extension 组件永不进入本仓库，也不能绕过 outbound 直接调用
  企微写 API；`/examples` 只允许不可部署、无凭据的 synthetic 契约夹具。

## 6. 公共领域契约

### 6.1 Identity

`IDRef` 的语义字段固定为：

| 字段 | 含义 |
|---|---|
| `Kind` | `wecom_external_userid`、`unionid`、`mp_openid`、`oa_openid`、`alipay_user_id`、`phone` 或 `ext` |
| `Scope` | 标识所属命名空间，所有 Kind 均必填；不同 scope 不互通 |
| `Value` | 调用者获得的原始值；只有 identity 模块能生成内部 normalized value |
| `Assurance` | 至少区分 `verified` 与 `declared` |
| `Source` | 可信来源，例如 wecom、survey 或 `ext:<key>` |

结果不是 `bool`：

- `Resolve`：`found | not_found | conflict`；不隐式建档。
- `Bind`：`bound | already_bound | merged | manual_review | rejected`。
- `Ingest`：`attributed | pending | conflict`。

P3-I00 将不可逆合并语义进一步冻结如下：

- HTTP/admin 请求只能形成 `declared/admin` 证据；`verified` 只能由完成自身验签或
  provider 凭据验证的内部 adapter 构造，任何请求体自报值都不能升级 assurance。
- 自定义 identity 的 Kind 统一为 `ext`，provider namespace 统一放在必填
  `scope=ext:<namespace>`；不再同时把 namespace 编进 Kind。
- verified unionid 冲突只在同一开放平台 scope 内进入自动规则。恰好一个
  effective customer root 具有 verified `wecom_external_userid` 时，它是 primary；
  两边都有或都没有时返回 `manual_review`，禁止以较小 ID、姓名、时间、字段数量
  或调用顺序猜测 primary。
- 锁顺序仍按 customer ID 升序，但锁顺序不是业务优先级。合并审计记录 policy
  version `verified_unionid_unique_wecom_v1`。
- verified phone 冲突与无法唯一选择 primary 的 unionid 冲突都创建或复用人工
  review；review approve 必须显式选择 current candidate root 内的 primary，并在同一
  UoW/锁内重验 pending/version/evidence/candidate-set，漂移即 409 且零副作用。
- review 展示指纹必须是 typed-secret-backed、版本化 HMAC-SHA256 的 128-bit
  base64url 截断值；禁止 raw/normalized identity、无密钥 hash 或 handler 自算。
- `customer.merged` payload 只含 primary/merged customer ID、merge audit ID、
  `auto|manual` mode 与 policy version；禁止外部 identity、PII 或 raw match key；
  idempotency key 固定为 `customer.merged:<merge_audit_id>`。

Scope 的 canonical 形式固定为：unionid 使用
`wechat-open-platform:<account-id>`，公众号/小程序 openid 使用
`wechat-app:<appid>`，企微 external userid 使用 `wecom-corp:<corp-id>`，手机号
使用 `phone:e164`，支付宝使用 `alipay-app:<app-id>`，扩展身份使用登记的
`ext:<namespace>`。任何模块不得自行
规范化并建立外部 ID 映射，也不得预判客户合并。`customers.id` 是唯一内部
OneID，渠道 ID 只能存在于 `identities`。

### 6.2 Transaction-bound contact/events

- contact 提供 transaction-bound `CreateForIdentity`、`MergeCustomers`、`AppendExternalEvent`。
- events 提供 transaction-bound `Append`。
- 新客户创建与首个 identity Bind 必须同事务。
- 客户合并时，identity 作出身份决策并写 identity 所有表；contact 执行客户、标签和时间线归并；events 追加 `customer.merged`。任一步失败均整体回滚。
- transaction handle 只能在当前 UoW callback 内使用，不能缓存、跨 goroutine 或泄漏到异步任务。

### 6.3 Outbound

outbound 只暴露 `EnqueueOne` 与 `EnqueueBatch`。调用方提交稳定幂等键；worker 负责限速、重试和状态记录。任何其他模块不得直接调用企微写 API。

### 6.4 HTTP

- 公共错误体至少包含 `code`、`message`、`request_id`，可选字段级错误详情。
- 列表统一 `{items, next_cursor}`，cursor 编码稳定排序列与 ID；禁止 OFFSET。
- 所有业务 HTTP 经固定中间件顺序：请求 ID、鉴权、账号并发预算、超时、panic recovery、统一错误、结构化访问日志。

## 7. 核心一致性与异步语义

一次业务命令的数据库状态变更与 `event_log` 追加在同一事务提交。dispatcher 以 `event_log.id` 为稳定身份，将事件送入 River 并持久化分发进度。River job、dispatcher 重试和消费者均按至少一次语义设计：允许重复尝试，不允许把它描述为基础设施 exactly-once。

每个消费者必须持久化或使用数据库唯一约束检查幂等键。外部企微调用若发生“请求可能已送达但响应丢失”的不确定结果，不能靠宣传 exactly-once 掩盖；必须保留可审计状态，并按接口可用的幂等/查询能力采取安全重试或人工处理。

## 8. 身份归因与合并策略

身份处理默认 fail-closed：

1. 精确标识只命中一个客户时可归因；命中零个为 `not_found`，出现冲突为 `conflict`。
2. 连接两个身份簇必须有可信桥接键。
3. 只有 verified unionid 冲突允许自动合并客户。
4. verified phone 冲突进入人工待合并；手机号存在换主风险。
5. declared phone 只作疑似提示，不能自动合并。
6. openid 只在同一 scope 内解析；跨 scope openid 永不作为桥接键。
7. 缺少可信桥接键的身份保持 floating；事件进入 `pending_events`，不得猜测归因。
8. 自动合并只做逻辑归并，不物理删除；`customer_merges` 永久追加审计，误合并拆分不在 v1 自动化范围。

详细事务和失败规则分别见 [ADR-003](../adr/ADR-003.md) 与 [ADR-004](../adr/ADR-004.md)。

## 9. 主要故障模式与防线

| 故障 | 风险 | 架构防线 |
|---|---|---|
| PostgreSQL 不可用 | API 与 worker 失效 | 健康检查失败、拒绝写入；备份恢复与 RPO/RTO 在 P6 外部门验证 |
| worker 被 kill | 任务延后 | River 持久化任务并重试；api 进程保持隔离 |
| dispatcher 在边界崩溃 | 事件重复或延迟 | event ID + consumer 幂等；不依赖内存游标 |
| 重复企微回调 | 重复业务效果 | 原始回调幂等键与数据库唯一约束 |
| 外部写超时 | 结果可能未知 | 审计请求与结果；仅按安全策略重试，不声称 exactly-once |
| 身份证据不足 | 错误归因或误合并 | floating identity、pending event、manual review，默认拒绝猜测 |
| 某业务 Slice 越界 | 模块耦合和回归 | import/table/API owner lint、允许路径检查、行为快照门 |
| 生成物漂移 | 前后端/SQL 契约不一致 | 锁定生成器、生成两次无 diff、锁文件硬门 |

## 10. 决策索引

- [ADR-001：模块化单体、角色隔离与技术基线](../adr/ADR-001.md)
- [ADR-002：渠道中立 OneID 与身份数据模型](../adr/ADR-002.md)
- [ADR-003：transaction-bound UoW 与 MergePort](../adr/ADR-003.md)
- [ADR-004：fail-closed 身份归因和合并](../adr/ADR-004.md)
- [ADR-005：Transactional Outbox 与至少一次投递](../adr/ADR-005.md)
- [ADR-006：HTTP 契约、权限范围与 Webhook](../adr/ADR-006.md)
- [ADR-007：强类型配置、Secret 与 API key](../adr/ADR-007.md)
- [ADR-008：Extension API 边界](../adr/ADR-008.md)
- [ADR-009：Spec-first 与可复现生成](../adr/ADR-009.md)
- [ADR-010：功能矩阵状态与证据门](../adr/ADR-010.md)
