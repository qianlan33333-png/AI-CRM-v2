# AI-CRM v2 · P2–P3 执行计划

> 生效基线：`main@57bb4ca4b4b8e1b46978e6e513f6d9cdf28f3af7`
> 覆盖范围：G1 收口 → P2 → P3 → G3。P4 及以后另议。
> 上级文档：《AI-CRM-v2-重构详细设计.md》《AI-CRM-v2-执行方案-v2-至P3.md》。同范围内本文档优先。

---

## 0. 总纲：照搬优先原则（本期最高业务准则）

**一句话：完全照搬旧能力，只做新架构必需的最小改造，优先让新系统跑起来，其余全部进 backlog。**

这条原则的作用是**减少停报次数**——Sol 遇到下列情况一律照搬并记 backlog，不得停下来问：

| 遇到 | 处置 |
|---|---|
| 旧行为不合理、丑陋、绕远 | **1:1 照搬业务语义**，记 backlog，本期不优化 |
| 旧行为有 bug（非安全非数据损坏） | **照搬并记 backlog**，不在本期顺手修复 |
| 旧实现缺少产品判断（默认排序、文案、分页大小等） | 取旧系统**实际生产值**，不自行设计 |
| 旧功能在新架构下需要改造（外部 ID、timer、直调企微） | 只做**架构必需的最小改造**，业务语义照搬 |
| 性能不理想但不阻塞使用 | 记 backlog（但 §3.6 的 S 档硬门除外） |
| 旧代码有明显更好的写法 | **不重构**，先照搬，backlog 记录改进点 |

**backlog 机制**：`docs/backlog/post-launch.md`，每条记录 `来源片 / 旧行为 / 为何暂不处理 / 建议处理时机`。这是本期唯一允许"知道有问题但先不修"的出口，必须留痕，不得静默略过。

**照搬原则不适用的四种情况**（这些仍须按既有铁律与门禁办，不得以"照搬"为由放宽）：
1. 八条铁律与所有 CI 门禁
2. 安全边界：secret 处理、鉴权、企微凭据
3. 数据不可逆风险：identity 合并语义
4. ADR 已裁决事项：尤其 ADR-002 渠道中立 OneID

---

## 1. G1 收口（P2 的硬前置）

### 1.1 固化后状态

已签：C 档 `NOT_MIGRATED`、10 个核心 operation `APPROVED`（锚点 `G1-D01-2026-08-10`）；A/B、迁移映射与 feature matrix 已按 `G1-D02-2026-08-10` 固化。M0-4 保持非阻塞外部门。

### 1.2 G1-D02 批量裁决（用户已确认）

用照搬原则把逐条签字转为规则签字：

| 对象 | 裁决 | 依据 |
|---|---|---|
| A 档 501 条 | 全部 `MIGRATE/APPROVED`，1:1 照搬语义 | 具体 v2 operation 由各域合同冻结片确定 |
| B 档 268 条 | 全部 `DEFERRED_POST_LAUNCH/APPROVED` | **明确不是废弃**，上线后重评 |
| C 档 12 条 | 维持 `NOT_MIGRATED/APPROVED` | G1-D01 已签 |
| 迁移映射 316 行 | 全部转终态 | 分布为 33/57/14/7/24/20/160/1 |
| feature matrix 293 行 | 全部 `MIGRATE/APPROVED` | 仍为 `NOT_STARTED/NOT_RUN`，未伪造线上验证 |

**迁移映射的例外机制**（防止批量批准变成盲签）：Sol 在固化前先扫一遍 316 行，把**无法 1:1 照搬的行**单独列成 `docs/evidence/p1/migration-exceptions.md`，判定标准三类——
- 旧字段在新 schema 无对应落点（尤其外部身份列，须转 identities）
- 类型/语义发生变化（枚举值重定义、时区、精度）
- 旧库存在脏数据模式，直迁会失败

实际去重例外集为 193 行：95 行目标落点未闭合 + 98 行 source-presence 未闭合。用户已确认例外清单。

### 1.3 feature matrix 抽样规则

已取 A 档中调用量 top-20 的页面行为并由用户确认治理签字。本次未执行线上实际操作；差异仍留待 P5 人工全功能抽验兜底。

### 1.4 必须明示的风险

批量批准 A 档意味着：**若 api-facts 盘点误读了某条路由的行为，本期不会有人发现，直到 P5 人工抽验。** 这是"优先上线"的既定代价，不是疏漏。相应地——**P5 人工全功能抽验成为新旧一致性的唯一防线，其人工投入不得再压缩**（快照门只防新系统自我回归，不能证明新旧一致，见设计文档 §5）。

### 1.5 G1 出口

`docs/evidence/p1/g1-decisions.md` 已追加 `G1-D02`，route-triage.csv 实际 781 条 `human_signoff` 已转终态，ledger 已转 `P2_READY`，M0-4 为非阻塞外部门。

---

## 2. 子代理（Terra Max）委派规则

AGENTS.md 已允许"边界清晰、无架构/产品/安全判断的机械任务"委派。本期具体化为白名单：

### 2.1 可委派（Terra Max）

| 类型 | 前提 | 例子 |
|---|---|---|
| **UI 页面实现** | 对应 OpenAPI 已冻结、Orval client 已生成 | 客户列表页、详情页、人群条件编辑器、群发进度页、待合并队列页 |
| **测试语料扩充** | 规则规格由 Sol 冻结在先 | segment DSL 50 组用例、identity 合并 20 组用例 |
| **sqlc 查询实现** | schema 与 port 已冻结 | 各模块 queries/*.sql 编写 |
| **数据生成脚本** | 无 | 20 万模拟客户生成器 |
| **快照夹具生成** | 接口行为已由 Sol 定稿 | acceptance/snapshots 初始化 |
| **迁移映射→Go 结构转译** | 映射表已签字 | 机械转译 |
| **独立复核** | AGENTS.md 已强制要求 | 迁移与对账复核（不得提供迁移源码） |

### 2.2 禁止委派（Sol 独占）

- 中央契约区：`.github/**`、ADR、架构文档、`api/openapi.yaml`、`migrations/**`、公共 ports、根依赖、黑盒验收夹具
- **identity 合并算法本体**（数据不可逆风险）
- **segment DSL 编译器核心**（AI 生成代码最高风险处）
- **wecom 验签/AES 解密**（安全判断）
- UoW、event dispatcher、scheduler（架构骨架）
- 任何触及八条铁律判定的代码

### 2.3 委派纪律

- Sol 始终负责：范围冻结 → 结果复核 → rebase/门禁/PR/squash/main CI 闭环。**Terra 产出不直接进 main。**
- 委派时启用 hash 收据（`input_sha256`/`output_hashes`/`file_manifest_sha256`），Sol 自执行片仍免除。
- 并行上限 3，路径不重叠。
- Terra 片的 `correction_count` 归入 Sol 该片总数，停报阈值不变。

---

## 3. P2：平台层（约 18 片）

**目标：P2 结束时浏览器里必须有能点的东西。** 若全部做完仍只能 curl /healthz，立即叫停。

### 3.1 分组

| 组 | 内容 | 委派 |
|---|---|---|
| **A 事务与配置** | UoW（ADR-003 落点）、强类型 config、settings/secret 存储 | Sol |
| **B 队列与事件** | River role 装配、scheduler 唯一注册入口、event append、dispatcher | Sol |
| **C HTTP 面** | 请求/错误中间件、并发预算、登录、RBAC、router | Sol |
| **D 前端壳** | Web shell：布局/路由/登录页/侧边栏（60s 进程内缓存） | Web shell 可委派 |
| **E 样板模块** | stages store/service/handler/page + **首组真实快照** | store/handler Sol；page 可委派 |
| **F 部署与分档** | `--tier=s\|m\|l` 配置生成 + Docker Compose + staging 部署脚本 | 可委派（脚本机械） |

### 3.1.1 River 六队列隔离（可靠性增补锚点 `REL-2026-08-10`）

本规格并入 P2-04 River role 装配、P2-05 scheduler 注册、P2-07 dispatcher 和 P2-18 分档生成，不新增片。队列固定为 `critical/event/outbound/sync/heavy/ai`，不得增减；其中 `critical` 用于 webhook inbox/identity resolve，`event` 用于域事件，`outbound` 用于企微发送，`sync` 用于企微增量同步，`heavy` 用于 segment/stats/维护，`ai` 预注册但 P4 前无实际 job。

| queue | S(2C4G) | M(4C8G) | L(8C16G) |
|---|---:|---:|---:|
| critical | 2 | 3 | 4 |
| event | 1 | 2 | 4 |
| outbound | 1 | 4 | 8 |
| sync | 1 | 2 | 3 |
| heavy | 1 | 2 | 4 |
| ai | 1 | 2 | 3 |
| **合计** | **7** | **15** | **26** |
| **worker pgx 池** | **9** | **18** | **30** |

规则：每个 enqueue 点显式指定队列，禁止 default queue，并以 lint + 永久负例入 CI；`critical` 默认 timeout 30s 且禁长任务；`heavy` 不借用其他队列空闲槽；worker pgx 池必须大于等于队列并发总和 + 2，参数只由 `--tier` 生成。验收时灌满 `heavy`，`critical` 与 `outbound` 仍必须正常消费。

`event_log` 在 P3 仍使用现有 `dispatched` 布尔边界；per-consumer `event_deliveries` 明确推迟 P4。Dispatcher 不得添加只在单消费方成立的唯一约束或状态语义。

### 3.2 中间件顺序（固定，不得调整）

请求ID → 鉴权 → 单账号并发预算 → 超时 → panic 恢复 → 统一错误码 → 结构化访问日志

### 3.3 G2 出口（人工门）

- `--role=api` 不注册 worker；`--role=worker` 不监听业务端口（除健康检查）
- 同账号第 5 个并发请求返回 429
- 配置写错一项 → 启动失败并指明字段
- dispatcher `kill -9` 重启后不丢不重
- 同一部署脚本在 dev(S) 与 staging(M) 生成的参数与设计文档 §1.3 表格逐项一致
- **人工（需单独授权部署）**：staging 浏览器登录 → stages 页 → 新增阶段 → 列表出现 → 改名 → 生效

---

## 4. P3：业务核心（三波次）

```
波次 1（串行地基）   contact
波次 2（并行）       identity  ∥  segment
波次 3（并行）       wecom     ∥  outbound
```

**注**：仓库 `implementation-plan.md` 仍写"严格顺序 contact→identity→wecom→segment→outbound"，与 AGENTS.md §4 的波次划分冲突。以 AGENTS.md 为准，implementation-plan.md 需同步（见 §7 housekeeping）。

每波次启动前须冻结对应域 OpenAPI 与公共 port。

### 4.1 波次 1：contact（约 6 片）

| 片 | 行为 | 委派 |
|---|---|---|
| C-1 | 客户列表查询（keyset 分页，禁 COUNT(*)/深 OFFSET，>1万走估算） | Sol |
| C-2 | 标签与阶段写入（与 event_log 同事务，记 actor） | Sol |
| C-3 | 分区时间线（按月 RANGE，预建未来 3 月，BRIN + 复合索引） | Sol |
| C-4 | 客户列表 UI | **Terra** |
| C-5 | 详情/操作 UI | **Terra** |
| C-6 | 20 万数据性能验收 | 生成器 Terra，验收 Sol |

**专项控制**
- `customers` 表禁止任何外部身份列（ADR-002），schema lint 必须拦。
- C-6 **必须在 S 档（2C4G）dev 机跑**：任意筛选组合 P95 < 200ms，附 EXPLAIN + 压测输出。**S 档不达标即优化查询，不得要求升配**——这是上线可用性硬门，不适用照搬原则的 backlog 出口。
- M0-3 的 EXPLAIN 门在此首次承载真实业务查询。

**出口**：功能矩阵 contact 行全勾；C-6 达标；contact port 冻结。

### 4.2 波次 2A：identity（约 9 片，全 P3 最高风险）

**风险来源**：无旧代码对应（纯新增），且合并错误**不可逆**——两客户错误合并后标签/时间线/归属已归并，回退需人工工单。

| 片 | 行为 | 委派 |
|---|---|---|
| I-1 | 标识规范化 + upsert（scope 按 ADR-002：unionid→开放平台账号 / openid→appid / external_userid→corp_id / phone→E.164） | Sol |
| I-2 | Resolve（查不到返回 found=false，**禁止隐式建档**） | Sol |
| I-3 | Bind + 冲突检测分流 | Sol |
| I-4 | 自动合并（仅 verified unionid 冲突） | **Sol 独占** |
| I-5 | Ingest（归因成功落时间线 / 失败入 pending_events） | Sol |
| I-6 | pending replay worker | Sol |
| I-7 | 人工待合并 API | Sol |
| I-8 | 待合并页面 | **Terra** |
| I-9 | 并发负例 | 语料 Terra，断言 Sol |

**专项控制**
- 合并逻辑表驱动测试 **≥ 20 组**：主记录选择、标签并集、时间线归并、`customer_merges` 审计完整性。
- **并发负例必做且用真实 PG 事务**（非 mock）：同一 unionid 两个 Bind 并发提交，不得产生重复合并或孤儿绑定。
- fail-closed（ADR-004）：无唯一可信归因时保持 floating identity / pending event，**禁止猜测**。verified phone 冲突进人工队列；declared phone 与跨 scope openid 不得自动桥接。
- contact ↔ identity 联合写入走 transaction-bound UoW（ADR-003），禁止跨域直写。

**出口**：场景验收——"先以手机号建游离身份 → 后加企微好友带 unionid → 自动归因且 pending 事件回放落时间线"在 staging 跑通。

### 4.3 波次 2B：segment（约 6 片，AI 生成代码最高风险处）

| 片 | 行为 | 委派 |
|---|---|---|
| S-1 | 筛选 DSL 的 AST 定义与解析 | Sol |
| S-2 | DSL → SQL 编译器 | **Sol 独占** |
| S-3 | 每条生成 SQL 的 EXPLAIN 验证 | Sol |
| S-4 | 成员物化刷新（River periodic job） | Sol |
| S-5 | handler | Sol |
| S-6 | 人群包 UI（列表/条件编辑器/成员预览/手动刷新） | **Terra** |

**专项控制**
- 编译器表驱动测试 **≥ 50 组**，每组附生成 SQL 的 EXPLAIN（语料可委派 Terra，断言与编译器本体 Sol）。
- **与旧系统对拍**：取线上 AI Audience 3 个真实人群包定义，新旧各跑一次，成员集合 diff 必须为空。这是唯一能客观证明筛选语义一致的手段（定义由用户提供）。
- 可筛字段白名单 = customers 实体列 + tag；新增字段须"加列+加索引+进白名单"三件套。
- 编译器是**唯一**的筛选→SQL 转换代码，禁止第二份实现。

### 4.4 波次 3A：wecom（约 5 片）

| 片 | 行为 | 委派 |
|---|---|---|
| W-1 | 回调验签 / AES 解密 | **Sol 独占（安全）** |
| W-2 | 回调去重（幂等键） | Sol |
| W-3 | token 管理 + read client | Sol |
| W-4 | 通讯录/外部联系人同步（游标续跑） | Sol |
| W-5 | identity / contact 集成（建档与归因） | Sol |

**专项控制**
- W-1/W-2 必须以 `inbox_events` 实现持久化收件箱：`id/provider/external_id/raw_payload/signature_verified/received_at/status/attempts/last_error/processed_at`，并以 `(provider, external_id)` 唯一；状态仅为 `pending/processing/processed/failed/skipped`。
- handler 固定顺序：验签 → 解密 → `BEGIN` → `INSERT ... ON CONFLICT DO NOTHING` → 确实插入时同事务 `River InsertTx` 到 `critical` → `COMMIT` → 立即 ACK。重复回调不插 job 但仍 ACK；业务处理一律在 River job 内，禁止在 HTTP handler 内建档/归因/写时间线。
- 验收：同一回调重推 10 次只有 1 inbox + 1 job + 1 次处理；handler P95 < 200ms；异常后 raw payload 可重放；真实企微调试仍留 G3 外部门。
- 验签/AES 错一位静默失败：企微官方回调调试工具 + 测试企微真实回调**双验证**。
- 同步中断重跑不得产生重复数据（`wecom_sync_state` 游标幂等）。
- 企微读 API 只允许本模块（铁律 3）；外部标识一律经 identity port（铁律 6、9），禁止自建映射。

**出口**：真实加好友 → 10 秒内建档 + 时间线事件（G3 外部门）。

### 4.5 波次 3B：outbound（约 8 片）

| 片 | 行为 | 委派 |
|---|---|---|
| O-1 | batches / tasks store | Sol |
| O-2 | EnqueueOne | Sol |
| O-3 | EnqueueBatch（大批量分块） | Sol |
| O-4 | sender worker（令牌桶限速） | Sol |
| O-5 | 状态流转 + 事件回写 | Sol |
| O-6 | 重试 / 取消 | Sol |
| O-7 | handler | Sol |
| O-8 | 群发 UI（创建/进度/失败重发） | **Terra** |

**专项控制**
- O-1/O-5/O-6 的任务状态固定为 `pending/sending/sent/retryable_failed/final_failed/outcome_unknown/cancelled`。HTTP timeout、连接中断或无响应体 5xx 进 `outcome_unknown`；429/企微临时错误进 `retryable_failed`；参数非法/客户删好友等进 `final_failed`。
- `outcome_unknown` 永不自动重试；有可用的企微结果查询接口时经对账队列核实，否则保留待人工判定。`outbound_tasks` 不再以 `next_retry_at/attempts` 作调度状态；可保留 `attempt_count` 仅供审计/UI，调度由 River 独占。
- 批次统计与 UI 必须单列 unknown，文案明示“结果未知，重发可能导致重复”。验收需注入 timeout，证明任务进 `outcome_unknown`、River 不重试、批次单独计数。
- 20 万条任务入队按档位分块（S=1000 / M=5000 / L=10000），不得一次性插入锁库。
- worker 重启不丢不重；批次进度数字与实际一致。
- 企微写 API 只允许本模块（铁律 3）。
- **群发耗时瓶颈是企微限速而非机器规格**：压测只验"匀速消费 + 不影响 API 响应"，不追求总耗时。

---

## 5. G3：P3 收口门

全部满足方可宣告 P3 完成：

1. 五模块功能矩阵相关行 100% 打勾
2. S 档 20 万数据客户列表 P95 < 200ms（附 EXPLAIN + 压测输出）
3. 3 个真实人群包新旧 diff 为空
4. 测试企微真实加好友建档 + 归因链路通
5. 测试企微真实群发，手机确认收到
6. identity 并发负例通过（真实 PG 事务）
7. 全部 EXPLAIN 门无 Seq Scan
8. 快照门全绿；main CI 全绿于精确 SHA

未执行写 `NOT EXECUTED`，未授权外部门写 `PENDING_EXTERNAL_GATE`，**禁止用 mock/synthetic 顶替**。

---

## 6. 停报边界（P2–P3 期间）

### 6.1 必须停下来问用户

1. 数据不可逆风险的语义分歧（尤其 identity 合并规则）
2. 安全边界：secret、鉴权、企微凭据处理方式
3. 需要真实外部效果：企微调用、部署、live migration
4. spec 与已决 ADR 出现新矛盾
5. 单片 correction_count 达到 2
6. 某完整行为超过硬顶 15 文件 / 1500 行
7. 到达 G2 / G3 人工门
8. **旧行为存在安全漏洞或会导致数据损坏**（此时不适用照搬原则）

### 6.2 不得停下来问（照搬即可）

旧行为丑陋/不合理、旧行为有普通 bug、缺少产品判断的小选择、非阻塞性能问题、发现更好写法——**一律照搬 + 记 backlog**，继续推进。

---

## 7. Housekeeping（顺带修正，各自并入相邻片）

1. `docs/execution/implementation-plan.md` 的 P3"严格顺序"与 AGENTS.md §4 波次划分冲突 → 以 AGENTS.md 为准同步。
2. 设计文档 §9 的 M1–M6 周排期里程碑表已被 P0–P3/G1–G3 体系取代 → 删除或标注作废，避免与 M0-x 编号混淆。
3. `docs/backlog/post-launch.md` 需在 P2 第一片时创建。

---

## 8. 人工介入点（P2–P3 全程）

| 节点 | 动作 | 预估 |
|---|---|---|
| G1-D02 | 确认批量裁决 + 看迁移例外清单 + top-20 页面抽查 | 1.5–2 h |
| M0-4 | 开 main 分支保护 | 5 min |
| G2 | 授权 staging 部署 + 浏览器验证 stages | 30 min |
| 波次 2B | 提供 3 个真实人群包定义 + 看 diff 报告 | 30 min |
| G3 | 微信加测试企微好友 / 手机确认收信 | 30 min |
| 全程 | 停报响应 + PR 抽查 | 2 h |
| **合计** | | **约 5–6 h** |
