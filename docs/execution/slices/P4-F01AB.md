# P4-F01 A+B：旧 UI 问卷定义完整管理板块

## 冻结合同

- 基线为 exact-green main `4c204b0f250fe0eb9f2fc4004ec983590ef54a6a`；历史 questionnaire worktree
  仅作只读取证，未复制、未 cherry-pick。
- A 侧维持 `GET/POST /api/admin/questionnaires` 和
  `GET /api/admin/questionnaires/{questionnaire_id}`（`LEGACY-API-0423/0424/0427`；
  `LEGACY-S07-116/117/123`）。B 侧闭合 `PUT/PATCH`、复制、启用/停用、删除（
  `LEGACY-API-0426/0428/0429/0430/0431/0432`；`LEGACY-S07-118/119/120/124`）。
- 物理目标仍是 `LEGACY-T14-237/238/243`；本片只恢复问卷定义管理，不声称 legacy 数据导入完成。
- 所有管理写入沿用旧 UI 已消费的嵌套 questionnaire/question/option schema 与 human
  session、RBAC、same-origin CSRF 合同；不新增 UI、tenant、DTO 或 F02 评测能力。

## 原子性与验收

- 创建使用 F01A create receipt；更新、启停和删除使用独立的 Survey-owned management receipt，避免
  F01B 操作破坏 F01A `create` 收据的回滚合同。每项写入与 Questionnaire、Event 和 receipt completion
  同一 UoW；复制遵循旧合同，默认停用且不改动源定义。
- `00037_survey_questionnaire_management.sql` 不新增 tenant 列、索引、RLS、跨域 FK 或外部效果。
- 本地 PG16.14（55431）已执行 `36→37→36→37`，并验证 Survey/Event 历史指纹不变；最终 CLOSED
  仍以单一中文 PR 的四门 required CI、match-head squash 和 exact-main required CI 为准。

## 明确排除

- F02 assessment、提交/导出、H5/public publish、OAuth/identity ingest、分析、新 UI、真实外部操作及生产部署。
