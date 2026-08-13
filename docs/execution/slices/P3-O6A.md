# P3-O6A：River 自动重试与 attempt 历史

## 输入与目标

- Base SHA：`6e3bceba8aa16cc6799cd190075ec2170e7caed5`，开工时 origin/main 三项 required CI 全绿且 open PR 为 0。
- 目标：仅交付 River 对 `retryable_failed` 的自动重试、每次 River attempt 的生命周期历史、并发幂等以及 `outcome_unknown` 不自动重试。
- 连续 migration 为 `00022`；保留 O5 `outbound_send_attempts` marker、`UNIQUE(river_job_id)` 和旧 `ON CONFLICT` 契约，只新增 Outbound 所有的 append-only `outbound_send_attempt_history`。

## 冻结行为与边界

- River 是唯一 retry scheduler；worker 把 `attempt` / `max_attempts` / `state` 作为 typed 调用参数传入 Outbound，Outbound SQLC schema corpus 不引用、不读写 `river_job`。
- attempt 1 的 O5 marker 和结果事件形状保持兼容；attempt 2 及以后以 `(send_attempt_id, river_attempt)` 唯一记录生命周期，不重复 task、accepted event 或同一 attempt 的 provider 调用。
- 仅 `retryable_failed` 返回 River retryable error；`outcome_unknown`、明确永久失败与成功均不自动重试。不新增 `next_retry_at`、业务扫描器、第二调度器或新 River job。
- provider 边界仍为 at-least-once：`dispatching` 重放保守收敛为 `outcome_unknown`，不二次调用 provider。本片只用 fixture provider，绝不执行真实外发。
- 不含 cancel、manual retry、control receipt、HTTP API、O7/O8/UI/OpenAPI、生产 DB 或 live migration。

## 迁移与黑盒验收

- C07/55432（PostgreSQL 16.14）从 exact-main 水位 21 执行 `21→22`，用真实 River 产生 attempt 1 `retryable_failed` 和 attempt 2 `succeeded`，再 down 到 21。
- 降级后 history 仍为两条，旧 O5 `INSERT ... ON CONFLICT (river_job_id) DO UPDATE ... RETURNING id` 返回原 marker；再 up 到 22 后 history 仍为两条且 marker 仍唯一。
- 真实 River normal/boundary：429 fixture 经 attempt 1+2 成功；timeout 收敛 `outcome_unknown` 后 job 在 attempt 1 完成，稳定观察窗内 provider 仍只调用一次。
- 已有 O4/O5 race/rollback 合同继续通过；O6A worker 单元合同额外固定 River `attempt/max_attempts/running` 传递及仅 retryable 返错。

## Route / matrix 映射

- O6A 没有 HTTP route，因此不把任何 legacy route 虚报为已实现。
- `LEGACY-S06-026` 发送记录行继续保持 `implementation=NOT_STARTED` / `verification=NOT_RUN`；本片只提供其 O7 读 API 将调用的真实 attempt 历史与重试服务前置，由 O7 在具有 route、PR/SHA 和黑盒证据时精确更新矩阵。

## 修正归因与外部门

- `slice_induced=2`：首次编译暴露结果事实查询改为 history 字段后 adapter 仍用 O5 生成名，改为实际 SQLC 字段；首轮 O4/O5 PG 回放暴露 legacy O5 零值命令的 max-attempt 兼容冲突，仅对该 legacy 路径保留 attempt-1 回放兼容。达到 2 后已冻结范围。
- `verification_induced=8`：激活后 PATH 无 `rg` 而改用仓库内置检索；首个 River RED harness 将 deadline context 传给 client start 导致截止器日志噪声，改用 background start 且 RED 结论不变；首次收紧验收命令使用了激活脚本未导出的 URL 变量而整组 skip，以明确 c07 URL 重跑；兼容脚本首次将 `psql -At` 的 `INSERT 0 1` 状态行一并解析，在只读确认 waterline/history 和旧 upsert 均成功后改为 quiet 输出并通过；clean full CI 收据首次在 amend 前执行 append-only ledger 门而被正确拒绝，改为先 amend 同一业务 commit 再从 parent 复验；首次 GitHub application/go 在共享测试库中按 O2→O3→O4→O5→O6A 运行，前序夹具只清理 Outbound 业务表却留下 River jobs，使真实 O6A worker 被已失去 task 外键的测试残留饥饿；按 AGENTS 将其归为测试夹具时序，仅在显式 O6A real-River fixture 中清理 `queue=outbound` 且两个已知 job kind，不改业务语义；首次在已 push 后将新验证计数原位写入已成为历史的 `P3-O6A` ledger 条目，append-only 门正确拒绝，恢复原条目并改为追加 `P3-O6A-PR197-CI1` 前向收据；第一版夹具直接 `DELETE river_job` 被完整 CI 的 ownership-lint 正确拒绝，改为只读选取限定 jobs 并通过 River `JobDelete` API 清理，不弱化 catalog 所有权。
- `infra_induced=0`，`scope_induced=0`。
- 外部门：`REAL_WECOM_NOT_EXECUTED`、`OUTBOUND_EXTERNAL_EFFECT_NOT_EXECUTED`、`PRODUCTION_DATABASE_NOT_EXECUTED`、`LIVE_MIGRATION_NOT_EXECUTED`、`DEPLOYMENT_NOT_EXECUTED`。
