# G1-TRIAGE：生产路径频次与 781 路由候选分档

## 输入合同

- Base SHA: `506b07e553269dae6bf33257c8339f15cd06589e`
- Phase/milestone: `G1 human signoff input`
- slice_kind: `evidence`
- task_inputs:
  - `docs/evidence/p1/legacy-routes-6cb989c.json`
  - `docs/api-mapping.jsonl`
  - `docs/feature-matrix.csv`
  - `docs/spec/AI-CRM-v2-执行方案-v2-至P3.md`
- Execution: `sol_vertical_slice`; task type: `production_read_only_evidence`
- Executor: `gpt-5.6-sol`; reasoning: `root_session`; task ID: `G1-TRIAGE`

## Goal

使用用户明确授权的生产环境只读路径元数据，结合冻结 route authority 与 UI 引用关系，为 781 条路由生成可供人工签字的 A/B/C 候选表。

## Boundaries

- 仅 journal 与数据库目录结构只读查询；未写数据库、未改配置、未重启服务、未部署、未调用企微。
- 仅保留规范化路由、次数、首次/最近时间、状态与耗时聚合；不保存 IP、请求体、响应体或查询参数。
- 所有候选保持 `PENDING_HUMAN_SIGNOFF`；AI 不代替用户完成 G1。
- 本片 `slice_induced_correction_count=0`、`infra_induced_correction_count=0`。

## Acceptance criteria

1. CSV 精确 781 行，mapping ID 与 authority route key 唯一且完整。
2. A/B/C 精确分区为 501/268/12，总计 781；S02/S03/S04 总计仍为 156/184/441。
3. 生产流量、UI 升档、blocked/retired 与歧义匹配均有显式 basis，不能静默归类。
4. 每行 signoff 仍为 `PENDING_HUMAN_SIGNOFF`。
5. repo-contract、P1 reconciliation、敏感扫描和 PR/main CI 通过。
