# P3-R4A：Contact acceptance FK parent fixture

## 输入与目标

- Base SHA：`05bdd4f24a33cb9e1c8021441f0a20f3a537da72`
- 依赖：P3-R3A 已 CLOSED，#153 已 CLOSED 且 NOT_MERGED，主 PR 队列为空。
- 目标：在可导入的 `acceptance/contactfixture` 提供唯一的 Contact-owned、
  transaction-bound `pgx.Tx` FK 父行 fixture，返回 `customers.id` OneID。

## 允许路径

- `acceptance/contactfixture/**`
- `acceptance/identity/doc.go`
- `acceptance/identity/contactfixture_import_test.go`
- `docs/execution/slices/P3-R4A.md`
- `docs/execution/slice-ledger.yml`

## 明确排除

- 不修改 migration、Identity DDL、ownership/canonical、OpenAPI、ports、keyring、
  runtime、R3C data-security 或 R3D 能力。
- 不在 `cmd` 或 `internal/contact` 放置 fixture、SQL 或数据库调用；不写任何
  Identity 表，也不创建生产代码路径。

## 验收与回滚

- 单元测试证明 OneID 由调用方 transaction 返回，Identity acceptance 可导入该包；
  ownership lint 仅将这个精确 acceptance package 视为 Contact-owned，其他
  acceptance 包写入 `customers` 仍必须 fail-closed；`make source-policy-lint` 与
  repo-contract 继续通过。
- 回滚只需 revert 本 PR；该片不含 DDL 或外部效果。
