# Contributing

本仓库默认采用 FAST 开发流程。开始前阅读 `AGENTS.md` 和改动直接涉及的 ADR。

## 日常流程

1. 从最新 `main` 创建分支；当前工作树有其他改动时使用新 worktree。
2. 围绕一个连贯能力直接实现。API、UI、数据库、生成物和测试可以在同一 PR 闭环。
3. 运行 changed paths 对应的聚焦测试并修复失败；普通缺陷不需要重切任务。
4. 提交简短中文 PR，等待唯一 Required Check `ci / merge-gate` 通过后 squash merge。
5. 合并即完成；无需补写 Slice 卡、ledger、mapping、证据文件或 exact-main 收据。

## 什么时候需要更多检查

- OpenAPI/sqlc/Orval 输入变化：重新生成并确认生成物一致。
- Migration：验证 up/down/up、数据兼容和对应 PostgreSQL 测试。
- 依赖或锁文件：运行依赖审计并说明原因。
- 鉴权、Secret、支付、真实外发或不可逆数据：执行专项安全验证；没有明确授权时
  不运行真实效果。
- 其他情况只跑直接相关测试；全仓回归由 CI 的保守 fallback 和 Nightly 承担。

历史 `docs/execution/slices/**`、`slice-ledger.yml`、feature/API/migration mapping 和
evidence 继续作为历史资料保存，但不是新功能的交付清单。
