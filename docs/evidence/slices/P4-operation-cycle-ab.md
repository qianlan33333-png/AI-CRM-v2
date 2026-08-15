# P4 运营周期完整 A+B 验证收据

- scope：冻结且仅覆盖 `LEGACY-API-0055..0057`、`LEGACY-API-0396..0404`、`LEGACY-API-0691..0697`，共 19 条；第 2 个 slice-induced 修正后保持 `repair-only`。
- frozen_base：`cf125e3b0b66e396dd4f7230328d3c569f9e5b6a`（消息归档 PR #224 CLOSED 后的 exact-green main）。
- replay_source：本地候选 `e177942972cc8c1030ccde65cc4404c14ade1feb` 仅作白名单/语义证据；未 cherry-pick，未作为运行时来源。
- worktree：`/Users/qianlan/Downloads/新CRM/worktrees/AI-CRM-v2-p4-operation-cycle-ab-replay-cf125e3-20260816`。
- candidate_head：`0f4155387560064c22bde92a64f6699286ae2433`。
- candidate_tree：`519a8318354df8a536e79582ad269b324d256ef3`。
- candidate_parent：`0f0a1cea48369d8a2cc3a41092d37aab67a16c44`；`merge-base(candidate, frozen_base)=cf125e3b0b66e396dd4f7230328d3c569f9e5b6a`。

## 语义边界

- A 侧继续使用既有 Session→Actor→Capability、CSRF/RBAC、ownership 与 UoW；同一幂等键只保留一个本地事实。
- B 侧没有新增 service identity、token/JWT 生命周期；缺少权威 runner 事实即 fail-closed。普通非外效动作不新增 provider、缓存、重复 projection 或通用抽象。
- `accepted`/`queued` 与真实执行严格分界；`outcome_unknown` 为终态且不自动重试。没有 UI、tenant、产品新语义、生产库或真实外发。

## migration/latest-current 一次性扫描

新增水位为 `00041_operation_cycles.sql`，真实 PG16.14 C07 执行 `up/down/up`，最终 `41`。所有直接/间接 current/latest 消费者已同步到 41：

| 消费者 | 41 断言 | 保留的历史锚点 |
|---|---|---|
| `acceptance/auth/si00b_migration_compatibility.sh` | upgrade/final `41` | `27→41→27→41` |
| `acceptance/automation/d01_migration_compatibility.sh`、`d01_integration_test.go` | upgrade/final/expected `41` | `24→41→24→41` |
| `acceptance/outbound/o6a_migration_compatibility.sh` | current upgrade `41` | O6A 定向历史 fixtures |
| `acceptance/outbound/o6b1_migration_compatibility.sh` | current upgrade `41` | O6B1 定向历史 fixtures |
| `acceptance/outbound/o6b2_migration_compatibility.sh` | current upgrade `41` | O6B2 定向历史 fixtures |
| `acceptance/stats/l01_migration_compatibility.sh`、`l01_integration_test.go` | upgrade/final/expected `41` | `25→41→25→41` |
| `acceptance/order/ab_migration_compatibility.sh` | final current `41` | `38→40→38→40` 与 target-40 rollback |
| `acceptance/wecom/message_archive_*` | 仍锁定 target `40` | `39→40→39→40` |
| `acceptance/operationcycle/ab_integration_test.go`、manifest、SQLC/generated、repo-contract | expected/current `41` | operation-cycle 自身 `41` target |

历史 target-version/rollback 仍明确存在（A01 26/27、I01A 28/29、H01A1 29/30、H03 33/34、F01A 30/31、F01AB 36/37、C01 31/32、J01 32/33、I03 34/36、消息归档 39/40、订单 38/40），未把历史证明伪装成当前水位。

## 证据

- `make ci-go`：PASS；包含 generate-check、race test、vet、build、vuln、P0/P2、架构/ownership/source-policy、feature/migration mapping、OpenAPI、query-plan `checked=35`。
- `scripts/run_ci_acceptance_manifest.sh`：PASS，`entries=38`；所有 targets 在指定 `postgres://postgres:postgres@127.0.0.1:55432/aicrm_test` 上完成。
- 真实 migration：`00001..00041` up、全量 down、再 up，PASS；operation-cycle focused normal/boundary/error/race/ownership 与直接消费者 PASS。
- `scripts/check_repo_contract.sh`、`make generate-check`、`git diff --cached --check`、`sensitive-path-scan`：PASS。
- staged patch `gitleaks detect --no-git --pipe`：`no leaks found`。
- production database、live migration、真实 provider/WeCom、真实外效、部署：`NOT EXECUTED`。

此收据在候选 head/tree 锁定且上述验证全绿后填写；PR/merge/exact-main CLOSED/promotion 收据待真实 GitHub 结果追加。
