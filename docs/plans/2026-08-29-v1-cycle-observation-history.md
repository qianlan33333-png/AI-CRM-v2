# V1 Cycle Observation History Implementation Plan

> 执行使用本会话可用的 executing-plans 与 Code；用户已授权本会话并线开发，不另建用户任务。技能所称 superpowers 别名未安装，使用上述同用途技能。

**Goal:** 将已封存的21条周期指标和18条引用按V2领域保存并提供真实只读入口，不恢复旧执行状态。

**Architecture:** Operationcycle-owned 两类不可变观察事实，复用现有导入回执、对账和静态历史页面。旧 run_id、last_snapshot_id 和 reference.source_id 仅为来源事实，不猜测当前V2外键；原href保留私有字段，不自动访问或变成可点击链接。所有来源字段进入事实/摘要校验，原加密归档继续保留。

**Tech Stack:** Go、PostgreSQL16.14/Goose/SQLC、现有OpenAPI/Orval、TypeScript历史页。

---

## 范围与并行边界

基于已验证 origin/main 5f4b9f48 建隔离worktree；PR587候选期间只能并行准备，当前包不得提前合并。下一包集成基线须更新至587的已验证exact-main。主代理独占迁移序号、Port、OpenAPI、路由、SQLC/Orval生成、账本和CI；leaf只写下列独立文件。

备选为仅归档（缺业务读取）或写当前run snapshot（没有可信run crosswalk）。选择只读观察事实；不新增调度、风控、动作平台或产品功能。全量数量/列数以V2冻结archive manifest实查为准，旧mapping仅作来源线索。

## Task 1: 私有来源转换与选择

Create: `cmd/aicrm-v1-domain-import/internal/v1cycleobservationhistory/history.go`、`history_test.go`、`selector.go`、`selector_test.go`。

先写测试：完整17/13列、nullable double/空串/负数/原JSON和UTC微秒保留；任一source/payload/field HMAC损坏或重复/跳号必须失败。运行 `go test ./cmd/aicrm-v1-domain-import/internal/v1cycleobservationhistory`，先确认新行为失败，再实现最小adapter。

私有facts含来源三个32-byte摘要，Metric含sourceID/runID/lastSnapshotID、三个 `*float64`、标签/窗口/质量/来源/单位/状态、limitations JSON、isCausal和两个时间；Reference含sourceID/runID/lastSnapshotID、key/type/label/sourceSystem/referenceSourceID/href/evidenceHash/dataStatus和两个时间。不访问V1，不调用Provider，不写当前表。

选择器复用archive reader和终态回执的一一认证，验证ordinal/去重/整表manifest数量。测试通过提交独立commit，不改中央入口。

## Task 2: 主代理冻结领域契约与DDL

Create: `internal/operationcycle/port/observation_history.go`、下一空闲迁移文件（587门禁后确认序号）；Modify: `sqlc.yaml`、`docs/architecture/table-ownership.yml`。

最小接口为metric/reference的create/get/list及事务内receipt journal；read-only分页limit1..100、offset>=0。私有摘要与href不进入公开JSON。原始JSON保留语义，禁止对可疑JSON字符串擅自展开。两表不引用current run，不生成事件/任务。真实SQLC生成后跑 `make migration-validate ownership-lint generate-check`。

## Task 3: 领域存储与导入对账

Create: `internal/operationcycle/app/observation_history.go`、对应test；`internal/operationcycle/store/observation_history.go`、对应test、`queries/observation_history.sql`；`cmd/aicrm-v1-domain-import/internal/v1domain/cycle_observation_history.go`、对应test。

先写全字段digest/同来源重放/漂移拒绝/caller transaction回滚测试，再实现现有history writer模式。主代理仅在 `journal.go`、`reconcile.go`、`main.go` 串行接入独立import version和local-only CLI。运行相关 `go test` / `go test -race`，隔离PG验证39首次导入、重放新增0、两次同摘要对账，旧数据与runtime=0不变。

## Task 4: 复用只读入口

Modify: `api/openapi.yaml`、候选生成配置、`cmd/aicrm/api.go` / `legacy_api.go`（主代理）；Create: `cmd/aicrm/cycle_observation_history_api.go` 与测试。四个GET只读admin鉴权，未配置503；route/DTO冻结后再生成客户端。

Modify: `web/src/api/staticHistory.ts`、`web/src/admin/sections/staticHistory.ts` 和对应行为脚本。增加metric/reference两类，不建新页面；鉴权、分页、详情、空/失败态、X转义和无Mock回退均测试。运行 `make orval-check openapi-p1-contract`、`npm run typecheck`、对应E2E与build。

## Task 5: 集中门禁和部署

一份PR包含实现和必要验收记录。候选最终精确SHA Full绿灯才合并；exact-main Full绿灯才备份、正式迁移/重放/双对账和独立id-dev部署。新备份只入 `/data/aicrm/backups`。V1零修改、旧入口零切流、真实外部效果关闭；完成后仍不把历史观察当当前HXC漏斗或Sidebar运行成功。
