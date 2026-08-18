# AGENTS.md

本文件适用于整个仓库。默认工作模式是 **FAST：小团队、快速交付、风险分级**。
历史 Slice 卡、ledger、mapping 和 evidence 是存量资料，不是新任务的必经流程。

## 1. 权限与交付

- 优先级：用户最新指令 > 已决 ADR/canonical architecture > 当前任务验收目标。
- 未明确授权时，禁止部署、修改服务器/旧系统、连接真实用户数据、运行 live
  migration，或执行真实企微、支付、退款、群发等外部写操作。
- 从最新 `main` 创建普通分支或干净 worktree。当前目录有用户改动时保留它们，换
  worktree 开发；无需为每个任务制作 base SHA、接管证明或 exact-main 证据。
- 默认完成实现、必要测试、提交、PR，并以 PR 的 `ci / merge-gate` 为合并资格。
  合并后无需补“闭环证据”PR，也无需重复认证 merge SHA。

## 2. 稳定架构边界

1. 域之间只能导入 `internal/<domain>/port` 或使用领域事件；禁止导入其他域的
   `app`、`store`、`http`、`worker`。
2. `customers`、`customer_tags`、`customer_events` 只允许 contact 写。
3. 企微写 API 只允许 outbound 调用；企微读 API 只允许 wecom 调用。
4. 业务状态变化与对应 `event_log` 必须在同一数据库事务提交或回滚。
5. 配置通过 config 的强类型结构读取；禁止散落 `os.Getenv` 或直查 settings。
6. `identities`、`customer_merges`、`pending_events` 只允许 identity 写；外部标识
   通过 identity port 解析、绑定或归因。
7. 客户定制组件通过 gateway Extension API 接入，不进入核心仓库。
8. 业务周期任务使用 River periodic jobs，禁止另建进程内 cron/ticker。
9. 部署模型固定为单实例、单企业、单数据库私有化；不建设 tenant 模型、字段、
   索引、切换器、权限层或跨租户测试。

## 3. OneID 与安全语义

- `customers.id` 是渠道中立 OneID；外部标识不能进入 customers 表。
- 无唯一可信归因时保持 floating identity/pending event，不猜测客户归属。
- 只有 verified unionid 冲突可自动合并；verified phone 冲突进入人工队列；
  declared phone 与跨 scope openid 不自动桥接。
- 跨 contact/identity/events 的创建、绑定、归并通过 transaction-bound UoW 与
  公开 port 完成，禁止跨域直写表。

## 4. FAST 开发方式

- 一个 PR 交付一个连贯的用户能力或缺陷修复。为闭环同一能力，可以同时修改
  API、UI、migration、内部 port、生成物、测试和少量文档；不设文件数或行数硬顶。
- Agent 可依据现有代码、接口和测试决定内部 DTO、表结构、包路径和实现细节。
  只有会改变用户可见行为、数据归属、安全边界或产生不可逆效果的歧义才询问用户。
- 普通实现错误、测试失败、lint、生成物、锁文件、性能索引或 fixture 问题在原
  分支直接修到通过。不记录 correction count，不因次数降档、停报或重切任务。
- 不要求新建或更新 Slice 卡、slice ledger、route triage、feature matrix、API/
  migration mapping、fingerprint 或独立 evidence，除非当前任务本身就是维护这些
  资产，或真实契约/清单变化使自动生成物必须同步。
- 中央契约不是角色专属区。完成当前能力所需时可以修改 OpenAPI、migration、
  公共 port、根依赖、CI 或验收夹具，并运行对应检查。
- 可以在互不冲突的路径上并行开发；不要求特定 Sol/Terra 角色、integration token、
  独立 Agent 复核或串行接管仪式。
- 只在以下红线立即停止并报告：未授权生产/外部写；鉴权绕过、密钥泄露或注入；
  跨域数据归属和事务原子性破坏；支付/退款/provider 效果可能重复；不可逆数据
  损坏；迁移会丢失数据。其余问题继续修复。
- 进度默认在完成或遇到真实阻塞时汇报，不为协议同步、计数修正或证据分级单独
  创建任务。

## 5. 测试与 PR

- oapi-codegen、sqlc、Orval 生成目录禁止手写；相关输入变化时同步真实生成物。
- `go.sum`、`package-lock.json` 的变化必须来自对应依赖操作并可解释。
- 本地优先运行与 changed paths 直接相关的测试；不要求每个开发任务本地跑全仓。
- PR 唯一 Required Check 是 `ci / merge-gate`。未知可执行变更由 CI 保守回退到
  full；Nightly 承担全仓 race、acceptance、生成、依赖和 PostgreSQL 回归。
- PR 用简短中文说明“改了什么、怎么验证、有什么风险”。只有 migration、真实外部
  效果或回滚复杂时才补充专项说明；不要求命令退出码表或证据等级表。
- mock/local/staging/production 不得混称。涉及真实部署或外部效果时，如未执行，
  简单写明“未执行”即可。
