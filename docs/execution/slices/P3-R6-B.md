# P3-R6-B：Portable SQL DDL parser foundation only

## 输入与目标

- Base SHA：`b2375c36f6d77df50336c2bdb939fb826bb358f2`。
- 前置：P3-R6-A #161 与优先入队的 Segment S04B #162 已 CLOSED，且该 exact main 的
  application/go、repo-contract、secret-scan 三门 required CI 全绿，共享 PR 队列为空。
- 目标：只提供可移植的 SQL lexer、顶层 table DDL 顺序应用与最终有效 table constraint
  canonical 模型，使多行约束和后续 `DROP CONSTRAINT` 都由结构而非文本 contains 判定。

## 允许路径

- `scripts/sqlddl/{lexer.go,model.go,parser.go,parser_test.go}`
- `docs/execution/slices/P3-R6-B.md`
- `docs/execution/slice-ledger.yml`
- `scripts/check_repo_contract.sh`（只维护 ledger 的既有 SHA 锚点）

## 冻结 parser foundation

- lexer 必须把单引号、双引号、dollar-quote、行注释、可嵌套块注释中的分号与关键字
  保持为非顶层 token，跨 macOS/Linux，不调用外部 parser 或数据库。
- parser 按语句顺序应用 `CREATE TABLE`、`ALTER TABLE ADD/DROP CONSTRAINT` 与
  `DROP TABLE`；未命名约束、命名约束、列和 quoted qualified name 进入确定性模型。
- `ALTER TABLE ... DROP CONSTRAINT` 必须从最终有效模型移除已命名约束；未知约束在无
  `IF EXISTS` 时 fail closed。
- canonical 输出统一非引用词大小写与 token 间距；columns、named constraints 与全量
  effective constraints 都提供稳定排序。

## 政策隔离

- 本片不包含任何 receipt/pending 专属表名、字段、结果闭集、状态、TTL、River、GIN、
  PII 或索引政策；这些只允许 R6-C 基于本片 CLOSED API 实现。
- 不读取或复用旧 R5-C2 parser WIP；旧 worktree 继续只读保留。
- 不修改 DDL、ADR、OpenAPI、ports、API、runtime、Make、依赖、CI workflow，也不将
  `sqlddl` 接入 repo-contract/CI；接线只属于 R6-D。

## 验收与回滚

- `go test -race -count=1 ./scripts/sqlddl` 必须覆盖多行约束、后续 drop、多 action、
  quoted/string/comment/dollar-quote 分号、unknown drop fail closed 与未闭合 DDL。
- `scripts/check_repo_contract.sh`、`scripts/test_repo_contract.sh` 与全量 Go/Web 门必须通过。
- 不连接生产数据库，不执行 live migration、真实企微或外发；回滚仅为 revert 本 PR。
- R6-C 必须等待本片 squash merge 且 exact-main 三门 CI 全绿。
