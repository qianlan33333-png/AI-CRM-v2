# P3-C07C-R3B：Contact external-event registry storage only

## 输入与目标

- Base SHA：`06efb99437c4691c14f49ddbd78a7f7bb805a813`（R3A CLOSED、Identity C1
  CLOSED 的 exact main）。
- 目标：以 migration `00011` 建立 Contact-owned `customer_event_idempotency` registry、
  down、ownership/canonical、catalog/constraint/rollback PostgreSQL 16.14 验收。

## 允许路径

- `migrations/00011_contact_external_event_idempotency.sql`
- `docs/architecture/{canonical.md,table-ownership.yml}`
- `scripts/ownership/main.go`
- `acceptance/contact/external_event_storage_integration_test.go`
- `Makefile`、`.github/workflows/application-go.yml`
- `scripts/{check_repo_contract.sh,test_repo_contract.sh}`
- `docs/execution/{slices/P3-C07C-R3B.md,slice-ledger.yml}`

## 明确排除

- 不修改 `AppendExternalEvent` runtime、Contact port/store/sqlc 或生成物；不实现 advisory
  lock、replay、conflict、并发或 merge 后行为。
- 不修改 Identity、Segment、API、UI、OpenAPI、`event_log` 或任何外部调用。
- 每次 ledger 更新必须以 R3A 的结构化 YAML 历史零漂移门验证，旧 C07C/R4B/R1 只读保留。

## 验收与回滚

- 在 PostgreSQL 16.14 slot 55432 执行 `00001→00011→down→up`，验证 registry catalog、
  event tuple/customer FK、唯一 tuple 与事实形状约束。
- migration/ownership 的最小永久负例、repo-contract、`make ci-go` 与 `npm run ci` 通过。
- 仅非生产测试库可以 Goose down；合并后回滚为 revert 本 PR，不执行 live migration。
