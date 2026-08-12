# P3-R3A：Identity ADR-only 持久化裁决

## 输入与目标

- Base SHA：`e655ff473a99a3052476bacd1ff29e9c6d1cad7d`
- 依赖：S03 exact-main CI（application/go、repo-contract、secret-scan）均成功；
  GitHub 无开放 PR。
- 目标：仅接受 ADR-012，并将其存在与六项关键裁决纳入最小永久保护。

## 允许路径

- `docs/adr/ADR-012.md`
- `docs/execution/slices/P3-R3A.md`
- `docs/execution/slice-ledger.yml`
- `scripts/check_repo_contract.sh`
- `scripts/test_repo_contract.sh`

## 明确排除

- migration、DDL、table ownership、OpenAPI、ports、keyring、runtime、normalizer、
  fingerprint 计算、receipt repository/UoW、Resolve/Bind/Ingest、UI 与外部调用。
- 旧 I01A1、I01A1R、I01A1R2A 候选只读保留；不得 cherry-pick、发布或修改其历史。

## 验收

- repo-contract 正门及 ADR 缺失、关键裁决弱化的负例通过。
- 连续双生成无 diff；`make ci-go` 与 Web 门通过（无 Web diff）。
- 不执行 PostgreSQL migration、真实企微、生产数据库或 live migration。

## 回滚

revert 本 PR；本片没有 DDL、运行时或外部效果。
