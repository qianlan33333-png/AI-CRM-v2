# Service Period Member Grid Canonical Local Core：V2 后端能力账本

## 口径

基线为 `origin/main@2615b5409dc35b49db7bd5922bbe3710159af4db`。本包只 canonical 化已有私有
member-grid 的 schema 与 query；不新增 member 生命周期 operation、migration、receipt、public/share、
Provider、支付或前端能力。Matrix 仅严格纠偏下述两行。

本包严格纠偏 `LEGACY-S07-153/154`：两行均为 `IN_PROGRESS/NOT_RUN`，不计为完成。153 只覆盖
access/default view 的 legacy exact 行为与 native V2 schema；其余 legacy schema 仍未交付。154 只覆盖
canonical local query、state/source 与 bound cursor；legacy arbitrary sort/group、saved-view switching 与
legacy row semantics 仍未交付。其余 Service Period 旧行保持 pending。本地 grid 的 read receipt 或 cursor
绝不等同于支付、Provider 或外部转移事实。

基线分层统计为 `179 IMPLEMENTED / 3 IN_PROGRESS / 112 NOT_STARTED / 294`；纠偏后为
`177 IMPLEMENTED / 5 IN_PROGRESS / 112 NOT_STARTED / 294`。implemented 为 **60.2%**，不把
IN_PROGRESS 计为完成。

## 四个既有读操作

| operation | 本包状态 | 数据与边界 |
| --- | --- | --- |
| `getServicePeriodMemberGridAccess` | 回归，不改 legacy mapping | 仍为 `LEGACY-API-0476`，只读本地 access |
| `getServicePeriodMemberGridSchema` | canonical | native operation；closed 12-column canonical schema |
| `queryServicePeriodMemberGrid` | canonical | native operation；只读 `service_period_members JOIN customers` |
| `listServicePeriodMemberViews` | 回归，不改 legacy mapping | 仍为 `LEGACY-API-0484`，只返回 built-in default |

Canonical row 的唯一字段集合是 `member_ref, service_product_id, customer_id, state, source, starts_at,
expires_at, expired_at, removed_at, version, updated_at, display_name`。不公开 remark、alliance、mobile、
external identity、legacy entitlement ID 或 payment/provider 字段。

`source` 缺省为所有 canonical source；显式值只允许 `manual` 或 `paid_order`。state 是
`active|expired|removed|all`。`mg2` cursor 使用 `(updated_at, member_ref)`，并加密绑定 product、state、
source 与 limit；旧 `mg1`、篡改或跨查询 cursor 均 fail closed。

## 本地验收范围

- direct contract/repository/service/http 覆盖 closed DTO、RBAC、strict JSON、source/state、cursor tamper
  与跨 product/state/source/limit 冲突；保存视图仍冻结自己的 v1 `revoked`/column contract。
- PostgreSQL 16.14 集成同时插入 canonical members 与同 product legacy entitlement decoy，验证只返回
  canonical rows、legacy-only row 排除、state/source、同 timestamp 分页、scope 与 cursor 冲突。
- 既有 canonical member lifecycle/CAS/idempotency 的 focused regression 继续作为本包依赖验证；本包不复制
  它的七个 operation 或 receipt 表。

通过只证明本地读取能力与本地数据库事实；不证明 Matrix 完成、Nightly、合并、部署或任何外部效果。
