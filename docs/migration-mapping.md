# T1.4 legacy 迁移映射候选

`docs/migration-mapping.jsonl` 是逐表、逐字段、逐行待签字的候选合同，不是可执行迁移。语义基线为
v2 `d2a260e130940a6466bb95c84d26fffb22a3b560`，源基线为 legacy
`6cb989c071255437d75953dabb943318a74eb8f4`。

## 证据与覆盖

- lifecycle manifest SHA-256 `710a01ee3813051b4ec13de8ef8b8ad64b39bc380b3a5a81c669580df24b488e`；baseline SQL SHA-256 `f19e4739ba7733be2d14bf47ea9af43cbb895d1a26ac3a4d301dcd0a3fad137f`；全部 baseline/version 文件清单 SHA-256 `67c4c9dc9b9c0393ec3c4a3521e901c168382d46de98bc4ce8da167afe62c2dc`。
- 冻结机器索引 `docs/evidence/p1/migration-lifecycle-index-6cb989c.json` 直接由上述 lifecycle manifest 生成，固定 316 张表及 domain/lifecycle/migration source/source line；validator 要求它与 mapping 逐表完全相等。
- 本地空 PostgreSQL 16.13 仅重放冻结 baseline+Alembic head，得到 217 张物理业务表/3312 字段；提取后已删除，它不是真实旧库或 PostgreSQL 16.14 验收。
- 316 行覆盖 lifecycle manifest：217 `HEAD_PHYSICAL`、98 `ABSENT_AT_HEAD`、1 `FRAMEWORK_METADATA`；缺失表的真实存在性必须等获授权的旧库只读 preflight。
- 3313 个 `field_mappings` 均有非空字段级 `reason`，理由绑定精确 `legacy_table.source` 与 target；每个物理字段恰好出现一次。新增理由没有改变任何 target、recommendation、decision 或 signoff。
- 候选分类：33 migrate、57 archive-only、56 drop、7 manual re-entry、24 rebuild、20 reset-runtime、118 target-schema-pending、1 framework-only；315 行均待人工签字。

## 硬边界

1. 当前物理业务 DDL 仅有 `stages`；`planned:*` 只表示详细设计的语义方向，全部仍是 `PENDING_TARGET_SCHEMA`。
2. `customers` 不存 external ID；unionid/openid/external_userid/phone 缺 scope/provenance 时保持 floating/quarantine，不得猜客户、owner、stage 或 merge。
3. 冻结源中未找到 `oauth_bindings` 物理表；不创建虚假行，openid 无 appid 时继续 PENDING。
4. 每个 migrate 候选必须使用 `(legacy_source_table, legacy_pk)` import ledger 改写 FK；未解析引用隔离。watermark 按表固定为 updated-at+key、created-at+key、full-only 或 pending。
5. 任何 queue/lease/retry/webhook/effect 运行态都不恢复；`unknown_after_dispatch`、`provider_accepted`、claimed/queued/retryable 不得伪映射为 sent/pending，不创建 River 任务。
6. active DND 未覆盖数必须为零；在 suppress model 或 launch deny-list 签字前禁止切换。commerce 仅 archive-only；secret/session/raw provider payload 不迁。

## 签字和后续

每行必须分别填写 `decision/signoff/decision_evidence`，实现与对账另行记录；未签字不得编写 T5.3 迁移。P5 需独立 Agent 执行行数、关键聚合和随机 100 条逐字段对账。

P1-C02 只补齐覆盖与字段处置理由，不生成迁移 SQL、不执行真实数据库读取、不把 `PENDING_HUMAN_SIGNOFF` 改成已批准。
