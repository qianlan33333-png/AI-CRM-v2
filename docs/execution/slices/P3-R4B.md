# P3-R4B：Identity storage-only 00010

## 输入与目标

- Base SHA：`7a2243c23fa0d0000bae8ec9035ca89ccd449bc5`
- 依赖：P3-R4A 已 CLOSED，exact-main 的 application/go、repo-contract 与
  secret-scan 均成功，且共享 PR 队列为空。
- 目标：以 migration `00010` 落地 ADR-012 的四张 Identity-owned 表、down、
  table ownership/canonical 与 PostgreSQL 16.14 catalog/constraint 验收。

## 允许路径

- `migrations/00010_identity_storage.sql`
- `docs/architecture/{canonical.md,table-ownership.yml}`
- `scripts/ownership/main.go`
- `acceptance/identity/storage_integration_test.go`
- `Makefile`、`.github/workflows/application-go.yml`
- `scripts/{check_repo_contract.sh,test_repo_contract.sh}`
- `docs/execution/{slices/P3-R4B.md,slice-ledger.yml}`

## 明确排除

- 不修改 ADR、OpenAPI、public/internal ports、auth、keyring、runtime、normalizer、
  fingerprint 计算、receipt repository/UoW、Resolve/Bind/Ingest、UI 或外部调用。
- 不新增 raw/normalized/receipt/privacy 专项门、不新增 JSONPath、`keyvalue`、TTL、
  River 或 GIN/state 索引；这些属于 R3C。
- 不修改 Contact C07C 或 `00011`；FK 父行只通过 R4A 的 Contact-owned acceptance
  helper 创建，Identity 测试不直接写 `customers`。

## 验收与回滚

- 在 55437 的 PostgreSQL 16.14 `aicrm_test` 执行 `00001→00010→down→up`，验证四表
  catalog、约束、append-only merge、receipt 的 in-progress commit 拒绝与 Contact FK。
- repo-contract 正门及 migration 缺失、第五张表、receipt ownership 漂移负例通过；
  `make ci-go` 与 `npm run ci` 通过。
- 回滚为在非生产测试库 Goose down；合并后如需回退则 revert 本 PR，不执行 live migration。
