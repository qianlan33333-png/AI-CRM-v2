# Slice <ID>: <single behavior>

## Fixed input

- Base SHA: `<40-hex>`
- Phase/milestone: `<P0-P6>`
- Relevant ADRs: `<links>`
- Specification rows: `<section or matrix IDs>`
- Execution: `<sol_vertical_slice|delegated_agent>`; task type: `<vertical_behavior|investigation|red_team>`
- Executor: `<gpt-5.6-sol|delegated model>`; reasoning: `<session|explicit>`; task ID: `<stable task id>`
- Worktree: `<absolute local path>`; dependencies: `<accepted IDs or none>`
- Payload paths: `<all implementation paths; excludes receipts>`
- Receipt carriers: `docs/execution/slice-ledger.yml`,
  `docs/evidence/slices/<ID>.md`
- Secret scan: `<tool/version/result>`

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

上限：8 个手写文件、400 行手写 diff、一个模块、一个 API operation 或
一个 UI flow。任何超限先停止并报告，不能自行扩片。

## Required implementation and tests

1. 先写能够证明目标行为的失败测试。
2. 运行并记录预期失败。
3. 写最小实现。
4. 运行 focused tests 和任务卡指定门禁。
5. 连续执行生成器两次，确认第二次无 diff（如适用）。

## Required output

- Sol 垂直 Slice 记录 base SHA、分支/PR/head/merge/main SHA、手写 diff 范围、命令、
  退出码、关键日志和外部门；不制作中间交接 ZIP 或 Terra 回执。
- 委派任务交回 Base、task id、model、reasoning、worktree、payload/receipt paths、
  手写 diff 行数、correction、命令、退出码和关键日志。
- 委派任务对全部 payload（包括 untracked）按 `LC_ALL=C` PATH 排序，输出
  `MODE BYTES SHA256 PATH` manifest 及 `file_manifest_sha256`。
- Sol stage 后执行 `git diff --cached --binary <base_sha> -- <payload_paths...>`；
  该原始输出的 SHA-256 是 canonical `diff_sha256`，receipt carriers 由 PR head/merge
  SHA 覆盖完整性。
- 未执行项写 `NOT EXECUTED`；未授权外部门写 `PENDING_EXTERNAL_GATE`。

## Executor prohibitions

本节仅适用于委派 Agent。其只在分配 worktree 修改/测试；除非任务卡明确授权，不得
stage、commit、push、PR、rebase、merge、部署、
真实迁移/外部调用、读取凭据、改变冻结契约、增加未授权依赖，或把 mock/synthetic 说成
真实验证。Git/GitHub、验收、rebase、PR、merge 和 main CI 由 Codex Sol 串行执行。

## Acceptance criteria

列出 Codex 独立黑盒验收、允许的精确输出和失败条件。
