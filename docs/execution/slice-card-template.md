# Slice <ID>: <single behavior>

## Fixed input

- Base SHA: `<40-hex>`
- Phase/milestone: `<P0-P6>`
- Relevant ADRs: `<links>`
- Specification rows: `<section or matrix IDs>`
- Input ZIP: `<filename>`
- Input size: `<bytes>`
- Input SHA-256: `<64-hex>`
- Secret scan: `<tool/version/result>`

## Goal

用一句可观察、可验收的话描述本片唯一目标。

## Frozen contract

列出公开接口、输入、输出、状态转换、错误和幂等规则。实现者不得修改。

## Path boundary

Allowed paths（逐项）：

- `<path>`

Forbidden paths至少包含：

- `.github/**`
- `docs/adr/**`
- `docs/architecture/**`
- `api/openapi.yaml`
- `migrations/**`
- `internal/*/port/**`
- 根依赖与锁文件（任务卡未明确授权时）
- 黑盒验收夹具

上限：8 个手写文件、400 行手写 diff、一个模块、一个 API operation 或
一个 UI flow。任何超限先停止并报告，不能自行扩片。

## Required implementation and tests

1. 先写能够证明目标行为的失败测试。
2. 运行并记录预期失败。
3. 写最小实现。
4. 运行 focused tests 和任务卡指定门禁。
5. 连续执行生成器两次，确认第二次无 diff（如适用）。

## Required output

- 基于 Base SHA 的 unified patch。
- 只含本片变更文件的 ZIP。
- 文件清单、大小和 SHA-256。
- 实施报告、完整命令、退出码和关键日志。
- 未执行项写 `NOT EXECUTED`；未授权外部门写 `PENDING_EXTERNAL_GATE`。

## Prohibitions

不得执行 Git/GitHub、部署、真实数据库迁移、服务器操作、真实企微调用、
读取凭据、改变冻结契约、增加未授权依赖，或把 mock/synthetic 说成真实验证。

## Acceptance criteria

列出 Codex 独立黑盒验收、允许的精确输出和失败条件。
