# AGENTS.md

本文件适用于整个仓库。

## 1. 权威与权限

- 优先级：用户最新指令 > 已决 ADR/canonical architecture > 详细设计 >
  执行方案 > Slice 卡。
- ADR 只能解决明确冲突，不能删减用户验收标准。
- 未明确授权时，禁止部署、修改服务器/旧系统、连接真实用户数据、执行
  真实企微写操作或运行 live migration。
- 每个 Slice 必须基于精确 `main` SHA；不得顺手扩大修改范围。
- 部署模型固定为单实例、单企业、单数据库私有化。禁止建设 tenant
  model/selector/switch/RBAC/column/复合索引/public port 或跨租户测试；
  完整决策见 [ADR-013](docs/adr/ADR-013.md)。

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
  `verification_induced`。非红线 `slice_induced` 第 1、2 个均允许在原片修复并继续；
  第 2 个起立即降档并进入 `SCOPE_FROZEN_REPAIR_ONLY`，冻结已批准能力范围，禁止新增
  能力、扩 scope 或无关重构。第 3 个及以后保持该状态，不得仅因计数达到 3 丢弃候选；
  仍允许修复既有缺陷、补永久负例、完成原始 DoD、同步真实 generated/lockfile，
  运行相关性 CI 并完成 PR/merge。不得为解锁合并伪造 mapping、acceptance、no-schema、
  Evidence Status、Slice 或其他治理文案。
- 红线缺陷无论第几个都立即进入 `HARD_STOP_REDLINE_READ_ONLY`：停止修复、重跑、
  generate、commit、push、PR、merge，必须在全新任务从 latest exact-green main 重切。
  红线是封闭集合：actor/授权/数据归属破坏或越权；认证绕过、
  密钥泄露、注入、开放跳转等安全边界缺陷；跨域直写或业务事实/event/delivery/River
  acceptance 未按合同处于同一要求事务等 ownership/原子性破坏；支付、退款、provider
  或真实外部效果重复执行，或 `outcome_unknown` 被自动重试；不可逆数据损坏或迁移
  丢失；未授权生产写、真实企微/真实发送/真实支付退款等外部操作。未触及上述红线的
  纯实现错误、错误分类、JSON 规范化、sentinel 链、文件结构、lint、测试断言或性能
  索引缺陷均属非红线，可在冻结范围内修复。
- 既存门禁误判、共享工具链/环境问题或 CI 抖动记入
  `infra_induced_correction_count`；验收命令、本地/CI 环境、测试夹具时序或证据调用
  方式的修正记入 `verification_induced_correction_count`。两者必须精确记录，但不
  降档、不硬停；机械环境、命令与时序问题在原任务内修复。只有需要修改共享基础设施
  或业务范围时才按归属另片，禁止绕过或降低门禁。
- 预期生成物与 lockfile 的真实同步属于 Definition of Done，不是 correction。
  文档、mapping、manifest、ledger 或仓库 fingerprint 只在对应事实真实变化时更新；
  不得仅为满足合并格式而补写，也不得让这些文本替代运行时测试。
- 切片卡范围欠定义导致的修正记入 `scope_induced_correction_count`；Sol 应依
  完整行为自行合并、拆分或标记 `SUPERSEDED_BY_RESCOPE`，不得清零原片历史计数。
- 新规则只适用于规则合入后从 exact-green main 新建、或规则合入时尚未产生 WIP 的
  候选；已按旧规则 HARD STOP 的 W0/A/H/I 及两次 W0 候选永久只读，不追溯复活、
  复制或 cherry-pick。历史计数与证据必须在 ledger 原样保留。
- 业务 PR 应围绕用户或权威计划批准的完整可观察行为；CI/platform 修正可作为独立、
  可验收的中央能力单元。ledger、feature matrix、mapping 与 evidence 可记录事实，
  但 CI 不得读取这些文本或 PR 标题/正文来判断是否可合并。
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
- Sol 可让非共享业务 PR 并行运行相关性 CI；中央契约裁决与 squash merge 仍按累计
  main 串行。合并资格只由 PR 上唯一的 `ci / merge-gate` 决定；无关 main 推进不得
  重新引入旧四门、strict exact-main 复验或合并后 provenance 反向认证。
- 常规进度仅在 P2 全部完成、P3 每个波次完成时批量汇报。只有真实外部效果、
  identity 不可逆语义分歧、鉴权/secret/企微凭据实质变更、需要用户真实输入或
  人工验收、与已决 ADR 或架构铁律实质冲突时立即停报。

## 5. 生成、测试与证据

- oapi-codegen、sqlc、Orval 生成目录禁止手写；连续生成必须无 diff。
- `go.sum`、`package-lock.json` 缺失或 `go mod tidy`/`npm ci` 后出现未解释
  diff 都是硬失败。
- PR 唯一 Required Check 是 `ci / merge-gate`；Ruleset 的 `strict=false` 由总负责人在
  仓库外维护，仓库内 Agent 未经明确授权不得修改 Ruleset。
- PR CI 只能依据 base SHA、head SHA、changed paths 与 `.github/ci-map.yml` 选择测试。
  `classify`、changed-range `secret-diff` 与 `merge-gate` 始终执行；无关 job 在 job 级
  明确 skipped，`success|skipped` 合格，`failure|cancelled` 阻断。
- 未知 Go/Web/SQL/migration 等可执行变更必须走保守 full fallback；不得使用 workflow
  级 paths 过滤，也不得削弱生成一致性、真实 PostgreSQL 验收、依赖审计或密钥扫描。
- `.github/workflows/ci.yml` 是唯一 PR 链路；不得恢复旧 application/repo-contract/
  secret-scan workflow、promotion/provenance 或 Candidate Guard，也不得解析 PR 标题、
  正文、Evidence Status、not-wired、no-schema、mapping/acceptance/Slice 文本来否决合并。
- `.github/workflows/nightly.yml` 承担全 Go race/test/build、全部 acceptance、完整生成物、
  漏洞/依赖审计、全历史 secret scan 与全域 PostgreSQL 回归；Nightly 不是 PR 合并门。
- mock/synthetic/local/staging/production 证据必须分别标注；未执行写
  `NOT EXECUTED`，外部未授权门写 `PENDING_EXTERNAL_GATE`。
- 所有 PR 使用中文说明，记录命令、退出码、生成物/锁文件差异、未执行项
  和回滚方式。
