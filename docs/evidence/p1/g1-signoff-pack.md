# G1 人工签字包（待签）

## 状态

- 候选基线：`main@72fb929257af595ab8852dd5b5b1eb1391ff8733`
- 当前状态：`G1_PENDING_HUMAN_SIGNOFF`
- 旧系统事实 SHA：`6cb989c071255437d75953dabb943318a74eb8f4`
- 真实路径频次表：`PENDING_PATH_FREQUENCY_INPUT`
- M0-4 分支保护：`PENDING_USER_CONFIRMATION`

此包只整理已冻结的静态事实与候选合同。没有人工签字的条目仍是候选，不能进入 P2 实现，更不能触发真实外部效果或迁移。

## 1. 781 条路由勾稽

`make p1-reconciliation-contract` 在候选基线上精确通过：

```text
p1-reconciliation: PASS (routes=781 s02=156 s03=184 s04=441 tables=316 fields=3313 pending_routes=781 pending_tables=315)
```

- S02 contact/auth/admin：156 条。
- S03 WeCom/segment/outbound：184 条。
- S04 上层域：441 条。
- 三批并集 781、交集 0、遗漏 0、重复 0。
- 781 条当前全部为待人工处置；没有伪签字。

### A/B/C 分档的唯一未满足输入

尚未收到旧系统路径级频次表，因此没有生成 `docs/evidence/p1/route-triage.md`，也没有猜测任何 A/B/C 档。

所需输入至少包含：`path`、`call_count`、`last_called_at`、统计窗口起止时间；建议窗口不少于 30 天。收到后按 v2 附录 B 结合 UI 引用关系生成 781 行分档表：有真实流量为 A；零流量且无 UI 引用为 B；明确 retired/blocked/fixture/恒定 404·410 为 C。窗口不足 30 天时，前端仍引用的零流量路径必须升为 A 人工看。

## 2. 迁移映射覆盖结论

`make migration-mapping-contract` 与 reconciliation 门均通过：

```text
migration-mapping: PASS (rows=316 physical=217 columns=3312 pending=315)
```

- lifecycle 索引的 316 张表全部出现：217 张 `HEAD_PHYSICAL`、98 张 `ABSENT_AT_HEAD`、1 张 framework metadata。
- 217 张物理表的 3312 个物理字段，加 1 个 framework metadata 字段，共 3313 个字段处置项；每项都有非空 reason。
- 315 张表仍为 `PENDING_HUMAN_SIGNOFF`；没有因机器校验通过而提升为可迁移。
- ADR-002 边界已机械锁定：`external_userid`、`unionid`、`openid`、`phone/mobile` 不得回写 `customers`，只能进入带 scope 的 identities、quarantine/pending 或明确不迁。
- queued/claimed/dispatching/retryable/unknown-after-dispatch 等执行态不得恢复成可发送任务；本阶段没有生成迁移 SQL、River job 或 provider 调用。

## 3. 10 个核心 OpenAPI 候选

| # | Operation | Method + path | Legacy mapping IDs | 状态 |
|---:|---|---|---|---|
| 1 | `listCustomers` | `GET /api/v1/customers` | `LEGACY-API-0609` | `PENDING_HUMAN_SIGNOFF` |
| 2 | `getCustomer` | `GET /api/v1/customers/{customer_id}` | `LEGACY-API-0619`, `LEGACY-API-0743` | `PENDING_HUMAN_SIGNOFF` |
| 3 | `updateCustomer` | `PATCH /api/v1/customers/{customer_id}` | `LEGACY-API-0736` | `PENDING_HUMAN_SIGNOFF` |
| 4 | `listCustomerEvents` | `GET /api/v1/customers/{customer_id}/events` | `LEGACY-API-0620`, `LEGACY-API-0739`, `LEGACY-API-0745` | `PENDING_HUMAN_SIGNOFF` |
| 5 | `resolveIdentity` | `POST /api/v1/identity/resolve` | `LEGACY-API-0355`, `LEGACY-API-0680` | `PENDING_HUMAN_SIGNOFF` |
| 6 | `bindIdentity` | `POST /api/v1/identity/bind` | `LEGACY-API-0709` | `PENDING_HUMAN_SIGNOFF` |
| 7 | `ingestIdentityEvent` | `POST /api/v1/identity/ingest` | `LEGACY-API-0780` | `PENDING_HUMAN_SIGNOFF` |
| 8 | `getAuthSession` | `GET /api/v1/auth/session` | `LEGACY-API-0758` | `PENDING_HUMAN_SIGNOFF` |
| 9 | `logoutAdmin` | `POST /api/v1/auth/logout` | `LEGACY-API-0760` | `PENDING_HUMAN_SIGNOFF` |
| 10 | `getAdminConfigOverview` | `GET /api/v1/admin/config/overview` | `LEGACY-API-0269` | `PENDING_HUMAN_SIGNOFF` |

合同门精确输出：

```text
openapi-contract: PASS (candidate_operations=10 pending=10 legacy_links=14)
```

抽查重点：Customer 不含渠道标识字段；IdentityRef 强制 `type/scope/value/assurance/source`；列表为 keyset cursor；统一 `ErrorResponse`；全部 operation 的迁移/合并/废弃与字段定义仍待你签字。

## 4. 精确 Git 与 CI 收据

- M0-6：PR #91，head `d75730180fb3c0bbcd54ef637dc50f1fdefdb92b`，merge `0abf07ebd3ea8f3ac795468df0cd7bbacd21ed9c`，main Go/web/repo-contract/secret-scan 全绿，修正 `slice=0 / infra=0`。
- P1-S11：PR #90，最终 head `bb1067549a70745231b3afb3d311627e16efdef9`，merge `72fb929257af595ab8852dd5b5b1eb1391ff8733`，main Go/web/repo-contract/secret-scan 全绿，修正 `slice=0 / infra=2`。
- #90 的两轮 infra 修正不作为切片过大信号；门禁缺陷已经由独立 M0-6 修复。

## 5. G1 人工动作与停止线

1. 提供路径级频次表后，生成并审阅 781 行 A/B/C 分档；先 C、再 B、最后逐条 A。
2. 抽查并签署上表 10 个核心 OpenAPI operation 的处置与字段。
3. 逐页面核对 feature matrix 的线上行为。
4. 对 316 行迁移映射逐行签字，尤其身份 scope 与历史外发状态。
5. 确认 M0-4 分支保护状态。

在上述四类业务签字全部完成前：P2 `NOT STARTED`；真实企微、生产数据库、部署、迁移均 `NOT EXECUTED / PENDING_EXTERNAL_GATE`。
