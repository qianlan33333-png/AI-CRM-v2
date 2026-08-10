# G1 人工裁决记录

## G1-D01：路由首批分档与核心 OpenAPI

- 决策日期：`2026-08-10`（Asia/Shanghai）
- 决策主体：repository owner
- 输入基线：`main@14ce03c27d2ab799d0a955c872e873682e61f473`
- 决策锚点：`G1-D01-2026-08-10`

冻结结论：C 档 12 条为 `NOT_MIGRATED/APPROVED`；10 个 contact/identity/auth/admin OpenAPI operation 按现有安全与 OneID 边界批准。Customer 不含渠道标识；IdentityRef 必须有 `type/scope/value/assurance/source`；resolve 不隐式创建；ingest 无法可靠归因时进入 pending/conflict；admin overview 不返回 secret；业务接口要求 admin session。

## G1-D02：规则签字与 P2 放行

- 决策日期：`2026-08-10`（Asia/Shanghai）
- 决策主体：repository owner
- 决策锚点：`G1-D02-2026-08-10`
- 决策基线：`origin/main@57bb4ca4b4b8e1b46978e6e513f6d9cdf28f3af7`
- 实施计划：`docs/spec/AI-CRM-v2-P2P3执行计划.md`
- 例外复核：`docs/evidence/p1/migration-exceptions.md`
- 抽样清单：`docs/evidence/p1/feature-matrix-top20.md`

### 实际数量优先

用户原指令中的 758/494/264 是旧快照；当前 authority 已前向推进为 781 条，因此按实际 tier 全量签字，不回退也不遗漏多出的 23 条：

- A：501 条，全部 `MIGRATE/APPROVED`，保留 1:1 旧业务语义；具体 v2 operation 仍由对应域 OpenAPI 冻结片确定。
- B：268 条，全部 `DEFERRED_POST_LAUNCH/APPROVED`；这不是废弃，不得写成 `NOT_MIGRATED`，上线后重评。
- C：12 条，继续由 G1-D01 锨定为 `NOT_MIGRATED/APPROVED`。

### Feature matrix 签字

293 条全部为 `MIGRATE/APPROVED`，但实施与验证仍精确保持 `NOT_STARTED/NOT_RUN`。Top-20 清单已经用户确认；本次是治理签字，没有伪造浏览器或生产验证。P5 人工全功能抽验仍是新旧一致性的唯一最终防线，投入不得压缩。

### 迁移映射签字

316 条终态为：33 `MIGRATE`、57 `ARCHIVE_ONLY`、14 `DROP`、7 `MANUAL_REENTRY`、24 `REBUILD`、20 `RESET_RUNTIME`、160 `DEFER`、1 `NOT_APPLICABLE/NOT_REQUIRED`。

- 160 个 `DEFER` = 118 个目标 schema 未闭合项 + 42 个源表存在性未确认的 drop 候选。
- 98 个 `ABSENT_AT_HEAD` 只是 source-presence preflight 边界，不能写成已确认脏数据或已确认不存在。
- 本次不生成迁移 SQL、River job 或 provider 调用；`DEFER` 行禁止进入迁移代码生成。

### 风险声明

批量批准 A 档意味着：如果 api-facts 盘点误读了某条路由的行为，本期可能无人发现，直到 P5 人工抽验。这是“优先上线”的已定代价；P5 人工全功能抽验不得简化。

### G1 出口

`route-triage.csv` 的 781 条 `human_signoff` 均已终态；feature/mapping/reconciliation 三门精确通过。P1 转 `P2_READY`；M0-4 分支保护仍记为非阻塞外部门。真实企微、生产数据库、部署和 live migration 仍未授权。
