# P4 Automation Agents A+B 本地集成收据

## 锁定对象

- exact-green base：`be62a43eb0865287c0372b9cbe295da7732458df`。
- 已验证实现 HEAD：`82a4b805c5affc0b6fb2071928a9c171cab86802`。
- 已验证实现 tree：`295283c8f5acbbb06ad9d7bc41580e40af23f6d0`。
- worktree：`agent/p4-automation-agents-ab-be62a43-20260816`。
- 当前水位：`42`；新增 `00042_automation_agents.sql`。历史 `down-to`/target-version 锚点保持，所有现行水位断言同步至 `42`。

## 冻结范围

仅覆盖 `LEGACY-API-0005`、`0006`、`0129` 至 `0138`：后台列表/编辑兼容页，以及列表、创建、删除、详情、更新、启用、暂停、复制、固定内容、发布的既有后台兼容接口。

- 复用 Session → Actor → Capability、CSRF、ownership/UoW 与既有 Events appender；配置、业务回执和事件同一 UoW。
- 不新增 V2 OpenAPI DTO、产品 UI、tenant、worker、River、真实 AI 或外发能力；没有超出冻结的 12 条兼容路由。
- 删除为 archived 状态转换；code 不可变；幂等和状态机由回执及现有所有权边界保持。
- repair-only：继承旧候选已达到的第 2 个 `slice_induced` 阈值，不因 fresh replay 重置。唯一补入的冻结内修复为 `name`、`code` 必填，`role`、`task`/提示词缺省为空；未称为新的首个修正预算。

## 验证

- `aicrm-v2-doctor`：隔离 PG16.14 slot 可用；base 与 `origin/main` 精确一致，开始时 open PR 为 0。
- `scripts/check_repo_contract.sh`：PASS。
- `make ci-go`：在锁定 HEAD、工作流同构的安全 loopback 输入下 PASS。
- 真实测试库 `55431/aicrm_test`：Goose `up → down → up` PASS，最终水位 42。
- focused normal/boundary/error：`go test -race -count=1 -timeout=180s ./internal/automation/... ./internal/events/store ./internal/platform/http ./internal/auth/... ./cmd/aicrm` PASS；A+B 覆盖缺失 prompt、不可变 code、退役字段、CSRF/RBAC、跨源拒绝、UoW rollback、copy/pause/activate/archive 及事件无 delivery。
- `scripts/run_ci_acceptance_manifest.sh`：一次成功汇总 `entries=39`；其中 `P4 Automation Agents A+B migration compatibility: PASS (41/42/41/42; Auth/session/Event/Survey history preserved; no tenant, foreign key, worker, AI, or outbound effect)`。

清单运行前仅重置了隔离 `55432/aicrm_test` 的 `public` 测试 schema，随后 Goose 恢复至 42；未接触生产库。生产迁移、部署、真实 AI、外发和 provider 调用均为 `NOT_EXECUTED`。本收据不宣称上线；CLOSED 仍以 PR 四门、match-head squash 与 exact-main 验证为准。
