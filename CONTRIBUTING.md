# Contributing

本仓库采用由 Codex 管理的单 Slice 交付流程。开始前必须阅读 `AGENTS.md`、
相关 ADR 和任务卡。

## 分支与 PR

- 除唯一一次 `P0-B0` 引导提交外，所有变更必须走分支和 PR。
- Slice 分支：`slice/<phase>-<id>-<slug>`；Codex 架构分支：
  `codex/<phase>-<slug>`。
- 一个 PR 只对应一个 Slice；默认使用 squash merge，合并后删除分支。
- PR 标题、正文和面向仓库的进度说明使用中文。
- 当前私有仓库套餐不能强制 Ruleset，因此发起人必须在合并前主动验证
  所有适用检查；详情见 `docs/governance/limitations.md`。

## Slice 限制

- 一个行为或状态转换、一个模块、一个 API operation 或一个 UI flow。
- 最多 8 个手写文件、400 行手写 diff。
- API 与 UI、迁移与外部 adapter 不得在同一片。
- 任务卡白名单之外的修改全部拒收。
- 中央契约区只能由 Codex 修改，除非任务卡逐文件明确授权。

## 验证

按当前阶段运行适用门禁，并在 PR 中记录完整命令和退出码：

- Go：生成无 diff、tidy 无 diff、fmt、vet、race、test、build、漏洞扫描。
- Web：`npm ci`、生成无 diff、lint、typecheck、test、build。
- DB：PostgreSQL 16 fresh DB、Goose、River、sqlc、集成测试和 EXPLAIN。
- Security：secret scan、权限边界、依赖/锁文件和 PII 日志脱敏。

未执行项必须写 `NOT EXECUTED`。真实外部条件未授权时写
`PENDING_EXTERNAL_GATE`；本地或 synthetic 结果不得写成生产验证。

## 外部 Pro 交付

Pro 仅提供基于精确 base SHA 的 patch、变更文件包、报告、测试日志和哈希。
Pro 不提交 Git、不开 PR、不改中央契约、不部署、不访问真实数据。Codex
在隔离 worktree 复验后才决定是否入库。
