# Contributing

本仓库采用 Codex root 总指挥的内部 Terra 交付流程。开始前阅读 `AGENTS.md`、
相关 ADR、任务卡和 [`agent-orchestration.md`](docs/governance/agent-orchestration.md)。

## 分支与 PR

- 除唯一一次 `P0-B0` 引导提交外，所有变更均由 Codex root 走分支和 PR。
- Slice 分支：`slice/<phase>-<id>-<slug>`；Codex 架构分支：
  `codex/<phase>-<slug>`。
- 一个 PR 只对应一个 Slice；默认使用 squash merge，合并后删除分支。
- PR 标题、正文和面向仓库的进度说明使用中文。
- Terra 仅在 root 分配的独立 worktree 修改/测试；root 独占验收、Git/GitHub、
  PR、rebase、merge 和 main CI，并串行执行。
- 当前私有仓库套餐不能强制 Ruleset，因此发起人必须在合并前主动验证
  所有适用检查；详情见 `docs/governance/limitations.md`。

## Slice 限制

- 一个行为或状态转换、一个模块、一个 API operation 或一个 UI flow。
- 最多 8 个手写文件、400 行手写 diff。
- API 与 UI、迁移与外部 adapter 不得在同一片。
- 任务卡白名单之外的修改全部拒收。
- 中央契约裁决和冻结/批准只属于 Codex root。Terra 仅可在中央合同任务的 root
  白名单内机械实现/测试；业务 Slice 不得修改中央契约。

## 验证

按当前阶段运行适用门禁，并在 PR 中记录完整命令和退出码：

- Go：生成无 diff、tidy 无 diff、fmt、vet、race、test、build、漏洞扫描。
- Web：`npm ci`、生成无 diff、lint、typecheck、test、build。
- DB：PostgreSQL 16 fresh DB、Goose、River、sqlc、集成测试和 EXPLAIN。
- Security：secret scan、权限边界、依赖/锁文件和 PII 日志脱敏。

未执行项必须写 `NOT EXECUTED`。真实外部条件未授权时写
`PENDING_EXTERNAL_GATE`；本地或 synthetic 结果不得写成生产验证。

## 内部 Terra 交付

- 每个任务固定 `gpt-5.6-terra` / `reasoning_effort=ultra`，记录精确 base、task id、
  绝对 worktree、payload manifest、测试和 correction；root stage 后计算 canonical hash。
- 最多 3 个 self-contained 任务在 DAG 依赖满足、白名单不重叠时并行。
- Terra 不得 stage、commit、push、PR、rebase、merge、部署、真实迁移/外部调用或外部
  上传；失败由同一 task follow-up，连续两次同根因失败或越界即拒收重拆。
- 不得新建、上传或续接网页 ChatGPT Pro 对话；P0-S01 的原始链接仅为历史证据。
