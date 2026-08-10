# G1 人工签字包（G1-D02 已固化）

## 状态

- 决策基线：`origin/main@57bb4ca4b4b8e1b46978e6e513f6d9cdf28f3af7`
- 决策锚点：`G1-D02-2026-08-10`
- 当前状态：`P2_READY`
- 旧系统事实 SHA：`6cb989c071255437d75953dabb943318a74eb8f4`
- M0-4 分支保护：`PENDING_USER_CONFIRMATION_NON_BLOCKING`

## 精确勾稽

```text
p1-reconciliation: PASS (routes=781 s02=156 s03=184 s04=441 migrate_routes=501 deferred_post_launch_routes=268 not_migrated_routes=12 tables=316 fields=3313 pending_routes=0 pending_tables=0)
feature-matrix-completion: PASS phase=p1 rows=293 synthetic=0 staging=0 production=0
migration-mapping: PASS (rows=316 physical=217 columns=3312 pending=0)
```

- 路由：A 501 条 `MIGRATE/APPROVED`，B 268 条 `DEFERRED_POST_LAUNCH/APPROVED`，C 12 条继续 `NOT_MIGRATED/APPROVED`。
- 页面行为：293 条治理签字为 `MIGRATE/APPROVED`，但实施/验证仍为 `NOT_STARTED/NOT_RUN`，不声称已做浏览器或生产核验。
- 迁移映射：316 条均为终态，分布为 33/57/14/7/24/20/160/1（MIGRATE/ARCHIVE_ONLY/DROP/MANUAL_REENTRY/REBUILD/RESET_RUNTIME/DEFER/NOT_APPLICABLE）。
- OpenAPI：G1-D01 冻结的 10 个核心 operation 继续有效；A 档路由批准不代替后续各域的 operation 冻结。

## 保留边界

- P5 人工全功能抽验是照搬一致性的唯一最终防线，不得压缩。
- ADR-002 OneID、identity 合并、secret/鉴权/企微凭据、数据不可逆风险不适用照搬放宽。
- 98 个 `ABSENT_AT_HEAD` 不等于真实旧库表不存在；未获授权前不做生产只读 preflight。
- 真实企微、staging/生产部署、生产数据库与 live migration 均为 `NOT_EXECUTED / PENDING_EXTERNAL_GATE`。

决策细节见 `docs/evidence/p1/g1-decisions.md`，例外清单见 `docs/evidence/p1/migration-exceptions.md`，抽样清单见 `docs/evidence/p1/feature-matrix-top20.md`。
