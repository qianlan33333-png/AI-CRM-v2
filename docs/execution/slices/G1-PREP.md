# G1-PREP：P1 收据与人工签字包

## 输入合同

- Base SHA: `72fb929257af595ab8852dd5b5b1eb1391ff8733`
- Phase/milestone: `P1 closeout / G1 input`
- slice_kind: `governance`
- task_inputs:
  - `docs/api-mapping.jsonl`
  - `docs/feature-matrix.csv`
  - `docs/migration-mapping.jsonl`
  - `docs/evidence/p1/legacy-routes-6cb989c.json`
  - `docs/evidence/p1/api-facts-contact-auth-admin-6cb989c.md`
  - `docs/evidence/p1/api-facts-wecom-segment-outbound-6cb989c.md`
  - `docs/evidence/p1/api-facts-upper-domains-6cb989c.md`
  - `api/openapi.yaml`
- Execution: `sol_vertical_slice`; task type: `human_gate_preparation`
- Executor: `gpt-5.6-sol`; reasoning: `root_session`; task ID: `G1-PREP`

## Goal

前向补录 M0-6 与 P1-S11 的精确 Git/CI 收据，汇总已经机械验证的路由、迁移与核心 OpenAPI 候选事实，并将仓库状态推进到 `G1_PENDING_HUMAN_SIGNOFF`。

## 边界

- 不在缺少路径级频次表时生成 A/B/C 分档，不以源码出现次数或静态猜测代替真实调用元数据。
- 不把 `PENDING_HUMAN_SIGNOFF` 改为 `SIGNED`。
- 不执行真实企微、线上页面操作、生产数据库、部署或真实迁移。
- 本片 `slice_induced_correction_count=0`、`infra_induced_correction_count=0`。

## Acceptance criteria

1. P1 reconciliation 精确通过 `781=156+184+441`，三批无重叠、无遗漏。
2. 迁移映射精确覆盖 316 表及 3313 个字段处置项。
3. 10 个核心 OpenAPI operation 均保留 `PENDING_HUMAN_SIGNOFF` 与 legacy mapping links。
4. ledger 补录 #90/#91 精确 head、merge、main CI 与 correction attribution。
5. G1 签字包明确列出未满足的人工输入，不提前进入 P2。
