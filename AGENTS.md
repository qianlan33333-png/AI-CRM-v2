# AGENTS.md

本文件适用于整个仓库。

## 1. 权威与权限

- 优先级：用户最新指令 > 已决 ADR/canonical architecture > 详细设计 >
  执行方案 > Slice 卡。
- ADR 只能解决明确冲突，不能删减用户验收标准。
- 未明确授权时，禁止部署、修改服务器/旧系统、连接真实用户数据、执行
  真实企微写操作或运行 live migration。
- 每个 Slice 必须基于精确 `main` SHA；不得顺手扩大修改范围。

## 2. 九条架构铁律

1. 域之间只能导入 `internal/<domain>/port` 或使用领域事件；禁止导入其他
   域的 `app`、`store`、`http`、`worker`。
2. `customers`、`customer_tags`、`customer_events` 只允许 contact 写。
3. 企微写 API 只允许 outbound 调用；企微读 API 只允许 wecom 调用。
4. 任何业务状态变化必须与 `event_log` 在同一数据库事务提交或回滚。
5. 配置只能通过 config 的强类型结构读取；禁止散落 `os.Getenv` 或直查
   settings。
6. `identities`、`customer_merges`、`pending_events` 只允许 identity 写；
   外部标识只能经 identity port 解析、绑定或归因。
7. 客户定制组件不进入核心仓库，只能通过 gateway Extension API 接入；
   `/examples` 仅允许不可部署的测试参照。
8. 业务周期任务一律使用 River periodic jobs；禁止 `time.Ticker`、
   `time.AfterFunc` 和第三方 cron。
9. 业务模块实现阶段不得读取旧 Python 源码；旧系统行为只能经
   `docs/rules/*.md` 或 `docs/evidence/p1/api-facts-*.md` 传递。实现片
   的任务输入中出现旧仓源码路径即为越界。

## 3. OneID 与事务

- `customers.id` 是渠道中立 OneID；customers 表不得出现 external_userid、
  unionid、openid 或手机号列。
- identity 的 `customer_id` 可空。无唯一可信归因时必须保持 floating
  identity/pending event，禁止猜测。
- 只有 verified unionid 冲突可自动合并客户；verified phone 冲突进入
  人工队列；declared phone 与跨 scope openid 不得自动桥接。
- 跨 contact/identity/events 的创建、绑定、归并通过共享 transaction-bound
  UoW 与公开 port 完成，禁止跨域直写表。

## 4. Slice 边界与内部执行编排

- 每片只解决一个完整可观察行为：一个模块、一个 API operation 或一个 UI flow；
  API 与 UI 不得同片。此条不因规模上限而放宽。
- 手写规模上限：P2 为 12 文件 / 800 行；P3–P4 为 12 文件 / 1000 行。
  生成物与测试文件不计入手写额度。
- 当一个完整行为无法在上限内闭环时，优先突破上限而非拆成无法独立验收的
  半成品；突破需在 slice 卡写明理由与实际规模，硬顶 15 文件 / 1500 行。
- 修正归因分为 `slice_induced`、`infra_induced`、`scope_induced` 与
  `verification_induced`。只有本片业务代码或设计缺陷导致的
  `slice_induced_correction_count` 参与降档与硬停：达到 2 时立即冻结范围、不得
  扩 scope，允许当前片完成既定闭环，下一片回退一档；达到 3 时立即停报并把未闭环
  行为重切为更小业务片。其他归因不参与硬停；历史计数必须在 ledger 保留。
- 既存门禁误判、共享工具链/环境问题或 CI 抖动记入
  `infra_induced_correction_count`；验收命令、本地/CI 环境、测试夹具时序或证据调用
  方式的修正记入 `verification_induced_correction_count`。两者必须精确记录，但不
  降档、不硬停；机械环境、命令与时序问题在原任务内修复。只有需要修改共享基础设施
  或业务范围时才按归属另片，禁止绕过或降低门禁。
- 预期生成物及既有 hash、manifest、ledger receipt 的正常同步属于 Definition of
  Done，不是 correction。只有首次遗漏且被门禁发现时记一次 `verification_induced`，
  在原任务补齐且不触发硬停。
- 切片卡范围欠定义导致的修正记入 `scope_induced_correction_count`；Sol 应依
  完整行为自行合并、拆分或标记 `SUPERSEDED_BY_RESCOPE`，不得清零原片历史计数。
- P3/P4 的每个 PR 必须关闭一个 ledger 中的官方业务 Slice，或关闭一个经用户/权威
  计划批准且能在 feature matrix 定位的完整业务 flow；禁止 parser-only、
  checker-only 或 governance-only PR。本次业务交付优先策略迁移 PR 是唯一例外，
  合并后不得再以迁移治理为名扩张。
- 并行最多 3 个任务。互不依赖、路径不重叠且不修改共享契约的业务路径允许并行 PR；
  `.github/**`、ADR、架构、OpenAPI、migrations、公共 ports、根依赖与黑盒验收夹具等
  中央契约仍串行。P3 波次划分为 contact → (identity ∥ segment) →
  (wecom ∥ outbound)。
- 迁移与对账必须由与实现者独立的 Agent 复核，且不得向复核方提供迁移源码。
- 独立安全片只允许处理不可逆数据污染、鉴权、迁移或真实外发的明确风险；能在业务
  垂直片内闭环时必须随业务片完成，不得把一般防御性加固扩成独立片。
- 相对简单、边界清晰且不需要架构、产品或安全判断的机械任务，如确有需要可
  委派 Terra Max 执行；Sol 仍负责范围冻结、结果复核、Git/PR 与 main CI 闭环。
- `.github/**`、ADR、架构、OpenAPI、migrations、公共 ports、根依赖与黑盒验收
  夹具是中央契约区；只能由 Sol 在当前垂直 Slice 内裁决和修改，或在冻结后以精确
  白名单委派机械实现。
- Sol 可让非共享业务 PR 并行运行门禁；中央契约裁决、最终 rebase、squash merge 与
  精确 main SHA CI 必须按累计 main 串行闭环。
- 常规进度仅在 P2 全部完成、P3 每个波次完成时批量汇报。只有真实外部效果、
  identity 不可逆语义分歧、鉴权/secret/企微凭据实质变更、需要用户真实输入或
  人工验收、与已决 ADR 或架构铁律实质冲突时立即停报。

## 5. 生成、测试与证据

- oapi-codegen、sqlc、Orval 生成目录禁止手写；连续生成必须无 diff。
- `go.sum`、`package-lock.json` 缺失或 `go mod tidy`/`npm ci` 后出现未解释
  diff 都是硬失败。
- mock/synthetic/local/staging/production 证据必须分别标注；未执行写
  `NOT EXECUTED`，外部未授权门写 `PENDING_EXTERNAL_GATE`。
- 所有 PR 使用中文说明，记录命令、退出码、生成物/锁文件差异、未执行项
  和回滚方式。
