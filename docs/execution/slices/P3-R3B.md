# P3-R3B：Identity storage-only 00010

## 输入与目标

- Base SHA：`05bdd4f24a33cb9e1c8021441f0a20f3a537da72`
- 依赖：P3-R3A 已 squash merge，exact-main 的 application/go、repo-contract 与
  secret-scan 均成功，且共享 PR 队列为空。
- 目标：只用 migration `00010` 落地 ADR-012 的四张 Identity-owned 表、down、
  table ownership/canonical 与 PostgreSQL 16.14 catalog/constraint 验收。

## 允许路径

- `migrations/00010_identity_storage.sql`
- `docs/architecture/table-ownership.yml`
- `docs/architecture/canonical.md`
- `scripts/ownership/main.go`
- `internal/contact/acceptance/identity_storage_fixture.go`
- `acceptance/identity/doc.go`
- `acceptance/identity/storage_integration_test.go`
- `Makefile`
- `.github/workflows/application-go.yml`
- `docs/execution/slices/P3-R3B.md`
- `docs/execution/slice-ledger.yml`
- `scripts/check_repo_contract.sh`
- `scripts/test_repo_contract.sh`

## 明确排除

- 不修改 ADR、OpenAPI、public/internal ports、auth、keyring、runtime、normalizer、
  fingerprint 计算、receipt repository/UoW、Resolve/Bind/Ingest、UI 或外部调用。
- 不新增 raw/normalized/receipt/privacy 专项门、不新增 TTL、River 或 GIN/state 索引；
  这些只属于 R3C。
- 不修改 Contact C07C 或 migration `00011`；Identity FK 父行仅由
  Contact-owned acceptance helper 创建。

## 验收

- 在 55437 的 PostgreSQL 16.14 `aicrm_test` 执行 `00001→00010→down→up`，并验证
  四表 catalog、约束、append-only merge、receipt 的 in-progress commit 拒绝与
  Contact-owned FK parent。
- repo-contract 正门与 migration 缺失、第五张表、receipt ownership 漂移的负例通过。
- 连续双生成无 diff；`make ci-go`、`npm run ci` 通过且无 Web diff。

手写范围因 Go 的 `./...` build 要求新增一个仅声明 test package 的 `doc.go`，为
13 文件，未增加业务行为，仍低于 15 文件硬顶。

## 回滚

在非生产测试库对 `00010` 执行 Goose down；合并后如需回退则 revert 本 PR，再按既有
迁移治理流程评估，不执行 live migration。
