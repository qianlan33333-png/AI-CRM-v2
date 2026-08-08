# Slice <ID>: <single behavior>

## Fixed input

- Base SHA: `<40-hex>`
- Phase/milestone: `<P0-P6>`
- Relevant ADRs: `<links>`
- Specification rows: `<section or matrix IDs>`
- Execution: `internal_terra`; task type: `<business_implementation|central_contract>`
- Executor: `gpt-5.6-terra`; reasoning: `ultra`; task ID: `<stable task id>`
- Worktree: `<absolute local path>`; dependencies: `<accepted IDs or none>`
- Payload paths: `<all implementation paths; excludes receipts>`
- Receipt carriers: `docs/execution/slice-ledger.yml`,
  `docs/evidence/slices/<ID>.md`
- Secret scan: `<tool/version/result>`

## Goal

用一句可观察、可验收的话描述本片唯一目标。

## Frozen contract

列出公开接口、输入、输出、状态转换、错误和幂等规则。实现者不得自行修改。
中央合同专用任务只能机械实现 root 已冻结并逐文件批准的合同；业务实现 Slice 不得
修改中央契约。

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

中央合同任务只能把 root 冻结的精确中央文件放入 Allowed paths；其他中央契约仍禁止。

上限：8 个手写文件、400 行手写 diff、一个模块、一个 API operation 或
一个 UI flow。任何超限先停止并报告，不能自行扩片。

## Required implementation and tests

1. 先写能够证明目标行为的失败测试。
2. 运行并记录预期失败。
3. 写最小实现。
4. 运行 focused tests 和任务卡指定门禁。
5. 连续执行生成器两次，确认第二次无 diff（如适用）。

## Required output

- 不得制作外部上传 ZIP。交回 Base、task id、model、reasoning、worktree、payload/
  receipt paths、手写 diff 行数、correction、命令、退出码和关键日志。
- 对全部 payload（包括 untracked）按 `LC_ALL=C` PATH 排序，输出
  `MODE BYTES SHA256 PATH` manifest 及 `file_manifest_sha256`。
- root stage 后执行 `git diff --cached --binary <base_sha> -- <payload_paths...>`；
  该原始输出的 SHA-256 是 canonical `diff_sha256`，receipt carriers 由 PR head/merge
  SHA 覆盖完整性。
- 未执行项写 `NOT EXECUTED`；未授权外部门写 `PENDING_EXTERNAL_GATE`。

## Executor prohibitions

Terra 只在分配 worktree 修改/测试；不得 stage、commit、push、PR、rebase、merge、部署、
真实迁移/外部调用、读取凭据、改变冻结契约、增加未授权依赖，或把 mock/synthetic 说成
真实验证。Git/GitHub、验收、rebase、PR、merge 和 main CI 由 Codex root 串行执行。

## Acceptance criteria

列出 Codex 独立黑盒验收、允许的精确输出和失败条件。
