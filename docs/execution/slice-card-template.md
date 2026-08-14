# Slice <ID>: <single behavior>

## Fixed input

- Base SHA: `<40-hex>`
- Phase/milestone: `<P0-P6>`
- slice_kind: `<implementation|governance|evidence|red_team>`
- task_inputs:
  - `<repo-relative path>`
- Relevant ADRs: `<links>`
- Specification rows: `<section or matrix IDs>`
- Execution: `<sol_vertical_slice|delegated_agent>`; task type: `<vertical_behavior|investigation|red_team>`
- Executor: `<gpt-5.6-sol|delegated model>`; reasoning: `<session|explicit>`; task ID: `<stable task id>`
- Worktree: `<absolute local path>`; dependencies: `<accepted IDs or none>`
- Payload paths: `<all implementation paths; excludes receipts>`
- Receipt carriers: `docs/execution/slice-ledger.yml`,
  `docs/evidence/slices/<ID>.md`
- Secret scan: `<tool/version/result>`

`slice_kind: implementation` 的 `task_inputs` 只能引用 `docs/rules/*.md`、
`docs/evidence/p1/api-facts-*.md`、`docs/spec/*.md` 和冻结的
`api/openapi.yaml`。实现片不得读取 `.py`、`aicrm_next/`、legacy snapshot
或绝对旧仓路径；事实调查、证据和红队片不受该条输入隔离限制。

## Goal

用一句可观察、可验收的话描述本片唯一目标。

## Frozen contract

列出公开接口、输入、输出、状态转换、错误和幂等规则。P0 由 Sol 在同一垂直
Slice 内裁决并修改必要的中央契约；委派执行者不得自行改变冻结契约。

## Path boundary

Allowed paths（逐项）：

- `<path>`

业务实现任务的 Forbidden paths 至少包含：

- `.github/**`
- `docs/adr/**`
- `docs/architecture/**`
- `api/openapi.yaml`
- `migrations/**`
- `internal/*/port/**`
- 根依赖与锁文件（任务卡未明确授权时）
- 黑盒验收夹具

委派任务只能把 Sol 冻结的精确文件放入 Allowed paths；Sol 垂直 Slice 可将必要的中央契约
列入同一白名单。

上限：P2 为 12 个手写文件/800 行，P3/P4 为 12 个手写文件/1000 行。
无法闭环完整行为时可先说明理由再扩大，但硬顶为 15 文件/1500 行；超过
硬顶必须停止报告，不得拆成无法独立验收的半成品。

## Correction state

- `slice_induced_correction_count`: `<integer>`
- `correction_state`: `<ACTIVE_REPAIR_ALLOWED|SCOPE_FROZEN_REPAIR_ONLY|HARD_STOP_REDLINE_READ_ONLY>`
- `redline_category`: `<none|tenant_actor_auth_data_isolation_or_cross_tenant_privilege_violation|security_boundary_auth_bypass_secret_leak_injection_or_open_redirect|cross_domain_ownership_or_required_transaction_atomicity_violation|duplicate_payment_refund_provider_real_external_effect_or_outcome_unknown_auto_retry|irreversible_data_damage_or_migration_loss|unauthorized_production_write_or_real_wecom_send_payment_refund_external_operation>`
- `frozen_capability_scope`: `<exact original capability set; required from count 2>`
- `historical_candidate_policy`: `<forward_applicable|permanent_read_only_no_revival_or_copy>`

非红线第 1 个修正可原片修复；第 2 个起降档并进入 `SCOPE_FROZEN_REPAIR_ONLY`；第 3 个
及以后保持 repair-only，不因计数丢弃候选。任一红线立即进入
`HARD_STOP_REDLINE_READ_ONLY`，停止修复、重跑、generate、commit、push、PR、merge，
并从 latest exact-green main 全新重切。红线只允许使用上述封闭枚举；未触及红线的
纯实现、分类、规范化、sentinel、结构、lint、测试或索引错误为非红线。

## Required implementation and tests

1. 先写能够证明目标行为的失败测试。
2. 运行并记录预期失败。
3. 写最小实现。
4. 运行 focused tests 和任务卡指定门禁。
5. 连续执行生成器两次，确认第二次无 diff（如适用）。

## Required output

- Sol 垂直 Slice 只在 ledger 前向记录 `pr_head_sha`、`merge_sha`、
  `main_ci_status`、`correction_count`；不制作中间交接 ZIP 或 hash manifest。
- 委派任务交回 Base、task id、model、reasoning、worktree、payload/receipt paths、
  手写 diff 行数、correction、命令、退出码和关键日志。
- 委派任务对全部 payload（包括 untracked）按 `LC_ALL=C` PATH 排序，输出
  `MODE BYTES SHA256 PATH` manifest 及 `file_manifest_sha256`。
- 仅当真实委派 Terra 子任务时启用上述 payload hash manifest、
  `file_manifest_sha256` 与 canonical `diff_sha256`。历史收据不回溯删改。
- 未执行项写 `NOT EXECUTED`；未授权外部门写 `PENDING_EXTERNAL_GATE`。

## Executor prohibitions

本节仅适用于委派 Agent。其只在分配 worktree 修改/测试；除非任务卡明确授权，不得
stage、commit、push、PR、rebase、merge、部署、
真实迁移/外部调用、读取凭据、改变冻结契约、增加未授权依赖，或把 mock/synthetic 说成
真实验证。Git/GitHub、验收、rebase、PR、merge 和 main CI 由 Codex Sol 串行执行。

## Acceptance criteria

列出 Codex 独立黑盒验收、允许的精确输出和失败条件。
