# G1 人工裁决记录

## G1-D01：路由首批分档与核心 OpenAPI

- 决策日期：`2026-08-10`（Asia/Shanghai）
- 决策主体：repository owner
- 输入基线：`main@14ce03c27d2ab799d0a955c872e873682e61f473`
- 生产只读证据：`docs/evidence/p1/route-triage.csv`
- 核心合同：`api/openapi.yaml`

用户在同一 Codex 任务中逐项确认：

1. `C 档 12 条全部不迁；B 档暂缓；A 档先审核 10 个核心接口。`
2. `10 个核心接口全部保留，按当前安全边界冻结。`

由此冻结以下机器可验证状态：

- C 档 12 条 legacy route：`disposition=NOT_MIGRATED`、`signoff=APPROVED`，不得生成 v2 operation、兼容路由或迁移实现。
- B 档 268 条：仅记录 `DEFERRED`，最终 disposition 与 signoff 仍为 pending；不得把暂缓误写成废弃或批准。
- A 档其余 legacy route：继续 `PENDING_HUMAN_SIGNOFF`；本次批准 10 个新 OpenAPI operation 不等于批准其余旧路由映射。
- P1-S11 的 10 个 contact/identity/auth/admin operation：`APPROVED`，决策锚点统一为 `G1-D01-2026-08-10`。

10 个 operation 的冻结边界不变：Customer 不含渠道标识；IdentityRef 必须有 `type/scope/value/assurance/source`；resolve 不隐式创建；ingest 无可靠归因时进入 pending/conflict；admin overview 不返回 secret；全部业务接口要求 admin session。

## 仍未完成的 G1 项

- A 档剩余路由 disposition：`PENDING_HUMAN_SIGNOFF`
- B 档最终 disposition：`PENDING_HUMAN_SIGNOFF`
- feature matrix 页面行为抽查：`PENDING_HUMAN_SIGNOFF`
- 316 张迁移映射逐行签字：`PENDING_HUMAN_SIGNOFF`
- M0-4 分支保护：`PENDING_USER_CONFIRMATION`

因此 P1 总状态仍为 `G1_PENDING_HUMAN_SIGNOFF`，P2 不得启动。
