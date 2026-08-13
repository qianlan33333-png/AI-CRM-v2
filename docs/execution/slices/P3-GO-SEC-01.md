# P3-GO-SEC-01：恢复共享 Go 标准库漏洞门基线

## 固定输入

- Base SHA：`04cdc854ade92df0c23cbb4ccb3ba18df7d00b62`
- 类型：shared security baseline repair
- 执行：Codex-Sol；根工具链、漏洞门与 repo-contract 不委派
- 触发证据：main workflow_dispatch `31750176613` 与 O7 PR run `31750813227`
- 冻结边界：O7 保持 OPEN 未合；不修改 Outbound 业务、生产或真实外部副作用

## 完整行为

1. 在 exact main、Go 1.26.5 下独立复现 `govulncheck` 的 6 个可达标准库漏洞。
2. 将仓库冻结工具链最小升级到共同修复版本 Go 1.26.6，不改扫描范围、不加 allowlist、不跳过漏洞门。
3. 保持 `application-go.yml` 从根 `go.mod` 读取版本且 `GOTOOLCHAIN=local`，确保 CI 使用审查后的精确补丁版本。
4. 通过 staged-index repo-contract 固化 `.tool-versions`、根模块、工具模块与 Makefile 的版本收据，并保留 pin 漂移永久负例。

## 路径边界

- 允许：`.tool-versions`、`go.mod`、`tools/go.mod`、`Makefile`、`scripts/check_repo_contract.sh`、`scripts/test_repo_contract.sh`、`scripts/staging_deploy.sh`、`docs/adr/ADR-001.md`、`docs/architecture/canonical.md`、本卡与 `docs/execution/slice-ledger.yml`
- 禁止：`.github/workflows/**` 漏洞门降级、`internal/outbound/**`、`cmd/aicrm/**` O7 业务、`api/**`、`migrations/**`、生产与真实外部副作用

## 验收

- Go 1.26.5 基线失败包含 `GO-2026-6218`、`GO-2026-6090`、`GO-2026-6089`、`GO-2026-6088`、`GO-2026-5972`、`GO-2026-5026`，且 O7 run 为同一集合。
- Go 1.26.6 下 `make version-check`、`go mod tidy -diff`、`make vuln`、workflow 同构完整 `make ci-go` 全部通过。
- `.tool-versions` 降回 1.26.5 的 repo-contract 永久负例必须失败。
- Web、repo-contract、secret-scan、required PR CI 与 exact-main 四门全绿。

## 回滚

直接回滚会恢复 6 个可达标准库漏洞，只允许与等价或更高的已审查 Go 安全补丁同步替换；不得通过弱化 `govulncheck`、allowlist 或扫描范围回滚。
