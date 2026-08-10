# Contributing

本仓库采用 Codex Sol 总负责的垂直 Slice 交付流程。开始前阅读 `AGENTS.md`、
相关 ADR、任务卡和 [`agent-orchestration.md`](docs/governance/agent-orchestration.md)。

## 分支与 PR

- 除唯一一次 `P0-B0` 引导提交外，所有变更均由 Codex root 走分支和 PR。
- Slice 分支：`slice/<phase>-<id>-<slug>`；Codex 架构分支：
  `codex/<phase>-<slug>`。
- 一个 PR 只对应一个 Slice；默认使用 squash merge，合并后删除分支。
- PR 标题、正文和面向仓库的进度说明使用中文。
- P0 由 Sol 在独立 worktree 完成实现、验收、Git/GitHub、PR、merge 和 main CI；
  每个完整行为尽量只有一个 PR。
- 当前私有仓库套餐不能强制 Ruleset，因此发起人必须在合并前主动验证
  所有适用检查；详情见 `docs/governance/limitations.md`。

## Slice 限制

- 一个行为或状态转换、一个模块、一个 API operation 或一个 UI flow。
- 最多 8 个手写文件、400 行手写 diff。
- API 与 UI、迁移与外部 adapter 不得在同一片。
- 任务卡白名单之外的修改全部拒收。
- 中央契约只能由 Sol 在当前垂直 Slice 内裁决和修改，或在冻结后以精确白名单
  委派机械实现。

## 验证

按当前阶段运行适用门禁，并在 PR 中记录完整命令和退出码：

- 首次检出或锁文件更新后运行 `make bootstrap-tools`。该目标按 `.tool-versions`、
  `tools/go.mod` 和 `package-lock.json` 安装并核对 Go 工具及 Orval；缺少或版本错误
  会明确失败，不允许跳过生成门。

- Go：生成无 diff、tidy 无 diff、fmt、vet、race、test、build、漏洞扫描。
- Web：`npm ci`、生成无 diff、lint、typecheck、test、build。
- DB：PostgreSQL 16 fresh DB、Goose、River、sqlc、集成测试和 EXPLAIN。
- Security：secret scan、权限边界、依赖/锁文件和 PII 日志脱敏。

未执行项必须写 `NOT EXECUTED`。真实外部条件未授权时写
`PENDING_EXTERNAL_GATE`；本地或 synthetic 结果不得写成生产验证。

## 分阶段协作

- P0：单 Sol 垂直闭环；P1：Terra 可分组并行调查，Sol 汇总裁决；P2：Sol 主做共享
  平台，孤立组件按需委派；P3/P4：契约冻结后恢复并行。
- 并行实现需至少两个互不依赖、路径不重叠且足以覆盖交接成本的任务，并且公共
  契约已冻结。最多 3 个 Terra task；独立红队复核可单独委派。
- 迁移与对账必须重新引入与实现者独立的 Agent；实现者不得自证正确。
- 不得新建、上传或续接网页 ChatGPT Pro 对话；P0-S01 的原始链接仅为历史证据。
