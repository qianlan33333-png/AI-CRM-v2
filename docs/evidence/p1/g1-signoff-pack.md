# G1 人工签字包（待签）

## 状态

- 候选基线：`main@14ce03c27d2ab799d0a955c872e873682e61f473`
- 当前状态：`G1_PENDING_HUMAN_SIGNOFF`
- 旧系统事实 SHA：`6cb989c071255437d75953dabb943318a74eb8f4`
- 真实路径频次表：`docs/evidence/p1/route-triage.csv`（A/B/C=`501/268/12`；C 已批准不迁、B 暂缓、A route disposition 待签）
- 已决记录：`docs/evidence/p1/g1-decisions.md`（G1-D01）
- M0-4 分支保护：`PENDING_USER_CONFIRMATION`

此包只整理已冻结的静态事实与候选合同。没有人工签字的条目仍是候选，不能进入 P2 实现，更不能触发真实外部效果或迁移。

## 1. 781 条路由勾稽

`make p1-reconciliation-contract` 在候选基线上精确通过：

```text
p1-reconciliation: PASS (routes=781 s02=156 s03=184 s04=441 tables=316 fields=3313 pending_routes=769 approved_not_migrated_routes=12 pending_tables=315)
```

- S02 contact/auth/admin：156 条。
- S03 WeCom/segment/outbound：184 条。
- S04 上层域：441 条。
- 三批并集 781、交集 0、遗漏 0、重复 0。
- 12 条 C 已以 G1-D01 批准不迁；其余 769 条 route mapping 仍待人工处置；没有伪签字。

### A/B/C 分档候选

用户已授权生产环境只读取证。`docs/evidence/p1/route-triage.csv` 覆盖完整 30 天窗口与全部 781 条 authority 路由：A=501、B=268、C=12；339 条有真实流量，162 条因 UI 引用但零流量保守进入 A。完整方法、最小化边界与输入收据见 `docs/evidence/p1/route-triage.md`。

G1-D01 已确认 C 全部不迁、B 暂缓，并批准 10 个核心新 OpenAPI。B 的最终 disposition、A 的 legacy route mapping 与其余 G1 项仍待签。

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
| 1 | `listCustomers` | `GET /api/v1/customers` | `LEGACY-API-0609` | `APPROVED` |
| 2 | `getCustomer` | `GET /api/v1/customers/{customer_id}` | `LEGACY-API-0619`, `LEGACY-API-0743` | `APPROVED` |
| 3 | `updateCustomer` | `PATCH /api/v1/customers/{customer_id}` | `LEGACY-API-0736` | `APPROVED` |
| 4 | `listCustomerEvents` | `GET /api/v1/customers/{customer_id}/events` | `LEGACY-API-0620`, `LEGACY-API-0739`, `LEGACY-API-0745` | `APPROVED` |
| 5 | `resolveIdentity` | `POST /api/v1/identity/resolve` | `LEGACY-API-0355`, `LEGACY-API-0680` | `APPROVED` |
| 6 | `bindIdentity` | `POST /api/v1/identity/bind` | `LEGACY-API-0709` | `APPROVED` |
| 7 | `ingestIdentityEvent` | `POST /api/v1/identity/ingest` | `LEGACY-API-0780` | `APPROVED` |
| 8 | `getAuthSession` | `GET /api/v1/auth/session` | `LEGACY-API-0758` | `APPROVED` |
| 9 | `logoutAdmin` | `POST /api/v1/auth/logout` | `LEGACY-API-0760` | `APPROVED` |
| 10 | `getAdminConfigOverview` | `GET /api/v1/admin/config/overview` | `LEGACY-API-0269` | `APPROVED` |

合同门精确输出：

```text
openapi-contract: PASS (candidate_operations=10 approved=10 pending=0 legacy_links=14)
```

G1-D01 已按现有边界冻结 10 个 operation。Customer 不含渠道标识字段；IdentityRef 强制 `type/scope/value/assurance/source`；列表为 keyset cursor；统一 `ErrorResponse`。这些 operation 的批准不自动批准其关联 legacy route 的迁移 disposition。

## 4. 精确 Git 与 CI 收据

- M0-6：PR #91，head `d75730180fb3c0bbcd54ef637dc50f1fdefdb92b`，merge `0abf07ebd3ea8f3ac795468df0cd7bbacd21ed9c`，main Go/web/repo-contract/secret-scan 全绿，修正 `slice=0 / infra=0`。
- P1-S11：PR #90，最终 head `bb1067549a70745231b3afb3d311627e16efdef9`，merge `72fb929257af595ab8852dd5b5b1eb1391ff8733`，main Go/web/repo-contract/secret-scan 全绿，修正 `slice=0 / infra=2`。
- #90 的两轮 infra 修正不作为切片过大信号；门禁缺陷已经由独立 M0-6 修复。

## 5. G1 人工动作与停止线

1. 继续处理 501 条 A 的 legacy route disposition；268 条 B 当前仅暂缓；12 条 C 已完成。
2. 10 个核心 OpenAPI operation 已完成 G1-D01 签字。
3. 逐页面核对 feature matrix 的线上行为。
4. 对 316 行迁移映射逐行签字，尤其身份 scope 与历史外发状态。
5. 确认 M0-4 分支保护状态。

在上述四类业务签字全部完成前：P2 `NOT STARTED`；真实企微、生产数据库、部署、迁移均 `NOT EXECUTED / PENDING_EXTERNAL_GATE`。
