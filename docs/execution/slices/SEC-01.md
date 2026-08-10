# SEC-01：升级受影响根依赖并补齐 Orval 本地工具链

## 固定输入

- Base SHA：`021c624b88b73ceb0695707e40c2a2f9deb43945`
- 类型：security repair
- 执行：Codex-Sol；根依赖、生成门与 repo-contract 不委派
- 阻塞片：P2-00，状态为暂停恢复，不是 supersede
- 安全锚点：`SEC-01-2026-08-10`

## 完整行为

1. 将 `golang.org/x/text` 从 `v0.29.0` 升至修复 `GO-2026-5970` / `CVE-2026-56852` 的 `v0.39.0`，不使用扫描抑制或范围降级。
2. `go mod tidy` 只允许解释清楚的 `go.mod`/`go.sum` 差异；`x/sync` 因 `x/text@v0.39.0` 的最小版本选择同步至 `v0.21.0`。
3. 全量 `govulncheck -show verbose ./...` 不得再有 Symbol Result；仅 Module Result 写入 `docs/backlog/post-launch.md`。
4. `make bootstrap-tools` 安装并核对根模块、`tools/go.mod` 和 npm 锁定工具；Orval 缺失或版本错误时生成目标必须明确失败并给出恢复命令。

## 路径边界

- 允许：`go.mod`、`go.sum`、`Makefile`、`CONTRIBUTING.md`、`scripts/test_orval_generated_check.sh`、`scripts/check_repo_contract.sh`、`scripts/test_repo_contract.sh`、`docs/backlog/post-launch.md`、`docs/execution/slices/SEC-01.md`、`docs/execution/slice-ledger.yml`
- 禁止：`acceptance/fixtures/**`、`internal/**`、`api/**`、`migrations/**`、P2-00 任何功能实现

## 验收

- `go mod tidy -diff` 无输出；根依赖只出现已解释的 `x/text` 与传递 `x/sync` 升级。
- `govulncheck -show verbose ./...`：Symbol Results 为 `No vulnerabilities found`。
- 删除/替换 Orval 路径的永久负例必须失败，并包含 `run 'make bootstrap-tools'`。
- Orval 连续生成两次无 tracked/untracked diff；Go/Web、repo-contract 与 secret scan 全绿。

## 回滚

回滚本片 merge 会恢复存在漏洞的旧依赖，因此只能在同时撤回 P2-00 helper 的前提下作为临时紧急回滚；不得通过关闭漏洞扫描回滚。
