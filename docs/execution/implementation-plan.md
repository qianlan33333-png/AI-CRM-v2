# AI-CRM-v2 implementation plan

状态：`EXECUTING`

负责人：Codex

外部实现：ChatGPT Pro，一次一个 Slice

## 1. 执行模型

Codex 独占架构、ADR、公共 port、DDL、OpenAPI、生成器版本、黑盒验收、
Git/GitHub 与最终合并。ChatGPT Pro 只实现冻结任务卡中的一个行为。上一片
未验收并入库前，不发送下一片。

每片硬上限：一个模块、一个 API operation 或一个 UI flow、最多 8 个手写
文件和 400 行手写 diff。API 与 UI、迁移与外部 adapter 不得同片。连续两次
同根因失败或发生中央契约变更时，Codex 拒收并重新拆片。

## 2. 阶段依赖

```mermaid
flowchart TD
  B0["P0-B0 仓库/ADR"] --> P0["P0 应用绿色基线"]
  B0 --> P1["P1 行为冻结"]
  P0 --> P2["P2 平台样板"]
  P1 --> P2
  P2 --> C["Contact"]
  C --> I["Identity"]
  C --> S["Segment"]
  I --> W["WeCom 归因"]
  S --> O["Outbound"]
  I --> SV["Survey"]
  O --> A["Automation"]
  O --> E["Extension"]
  P2 --> AI["AI"]
  AI --> A
  A --> P4["P4 功能门"]
  SV --> P4
  E --> P4
  P4 --> R["Replay"]
  I --> M["Migration"]
  M --> Q["独立 Reconciler"]
  R --> P5["P5 总验收"]
  Q --> P5
  P5 --> P6["P6 切换交付物"]
```

## 3. Slice catalog

### P0

- S01 Go role 启动骨架；S02 `/healthz` strict handler；S03 sqlc 最小查询；
  S04 River 官方 migration/runtime adapter。
- S05 React/Vite 壳；S06 Orval health client。
- S07 import lint；S08 表和企微写入归属 lint；S09 env/SQL/timer lint。
- S10 空 contract-replay runner。

### P1

- S01 旧路由 exporter；S02–S04 三个域组 API 对照；S05–S07 三个页面组
  功能矩阵；S08 证据 validator；S09 contact/identity 映射；S10 其余迁移
  映射。
- OpenAPI 由 Codex 按核心、运营链、上层域三批串行冻结。

### P2

UoW、强类型 config、settings/secret、River role、scheduler、event append、
dispatcher、请求/错误中间件、并发预算、登录、RBAC、router、Web shell、
stages store/service/handler/page，共 17 片。

### P3

- Contact：列表 store、标签阶段、分区时间线、handler、列表 UI、详情/操作
  UI、20 万数据性能。
- Identity：规范化/upsert、Resolve、Bind、merge、Ingest、pending replay、
  人工待合并 API、页面、并发负例。
- WeCom：验签/AES、回调去重、token/read client、同步、identity/contact 集成。
- Segment：AST、编译器、EXPLAIN、成员刷新、handler、UI。
- Outbound：store、EnqueueOne、EnqueueBatch、sender、状态/事件、重试取消、
  handler、UI。

严格顺序：contact → identity → wecom → segment → outbound。

### P4

- AI：provider/fake、prompt、火山 adapter、API、页面。
- Automation：规则、enrollment、共享 DSL、四种 action、规则页、记录页。
- Survey：CRUD、H5、OAuth、提交 Ingest、后台页。
- Gateway/MCP：旧行为 adapter、兼容 harness。
- Extension：API key、resolve、bind、ingest、customer read、outbound、webhook
  状态、签名投递、支付 mock。
- Stats：聚合、API、看板；Ops：队列、日志、健康、三面板。

执行优先级：AI → Automation → Survey → Gateway → Extension → Stats →
Ops → 剩余功能矩阵。

### P5/P6

- Replay：loader、路由转换、diff、runner、报告。
- Migration：只读 extractor、contact/identity、timeline、outbound、survey/其余、
  checkpoint/incremental。
- Reconciler：使用全新 Pro 对话，且不提供 migration 源码；计数聚合、逐字段
  抽样、故障注入。
- P6 只准备构建/SBOM、迁移脚本、备份恢复、冒烟和观察回滚清单；真实切换
  不在当前授权内。

## 4. 阶段门

- G1：merged/deprecated、核心 API 字段、页面矩阵和迁移映射人工签字。
- G2：staging 登录/stages 浏览器验证。
- G3：测试企微、Audience diff、S 档性能和手机收信。
- G4：真实 automation/问卷/MCP/运维演练。
- G5：两轮 replay、只读副本迁移、独立对账、人工逐页面抽验。
- G6：切换窗口、停写、URL/DNS、回滚和旧机销毁授权。

未授权门统一写 `PENDING_EXTERNAL_GATE`，不得由 local/mock/synthetic 替代。
