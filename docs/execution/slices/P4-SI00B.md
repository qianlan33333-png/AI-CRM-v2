# P4-SI00B：企微 CorpID 命名收口

## 输入、范围与语义核验

- 实时 base 为 SI00A CLOSED 后 exact-green `55d301a2dce28e244dc1f1e45ef1a3a7be5e036c`；从全新隔离
  worktree/branch 开始，根工作树与所有历史 WIP 只读，未复制或 cherry-pick。
- 非生产 PG16.14（`server_version_num=160014`）的实际基线行只有 `auth_provider=wecom`，
  `provider_tenant_id=corp-a01-migration`；运行时企微 adapter 的 `CorpID()` 正是该列的输入。
  因此该列是 WeCom provider identity namespace，不是 SaaS tenant 标识。
- 唯一运行时改动是 `VerifiedLogin.TenantID→CorpID`、查询/repository/SQLC 参数同步；
  A01 provider interface 继续提供 `CorpID()`。账号定位的三元键、session、CSRF、redirect、
  RBAC/capability 和 actor 语义都不变。

## 连续 migration 与历史兼容

- 新增 `00028_auth_wecom_corp_id.sql`，不原改 `00004_auth.sql`。Up 只将
  `provider_tenant_id` 改名为 `wecom_corp_id`，并前向改名其 CHECK/UNIQUE 约束及 UNIQUE
  背后索引；Down 精确反向改名，无列重建、数据拷贝或删除。
- 非生产 PG 对真实历史行执行 `27→28→27→28`：两个 CorpID 下相同 provider subject
  仍定位不同账号，账号行、已有 `admin_sessions`、三元唯一性和新旧约束/索引名均保留。
- 历史 A01 兼容证据固定在 `26→27→26→27`；当前 waterline 断言机械同步为 28，
  但不在本地重放与 Auth 无关的领域。

## 复用基座、未建能力与验证分级

- 复用 SI00A 的单实例/单企业/单数据库部署约束，复用 A01 的企微人类登录、
  canonical/legacy session cookie、session-bound CSRF、RBAC/capability 与安全站内 redirect；
  后续 Auth 直接消费者不再重复解释该 provider namespace。
- 未建设 tenant model/selector/switch/RBAC/column/compound index/cross-tenant test、通用
  provider/SSO、风控、审计告警、运维 UI/dashboard、恢复器、新业务语义或旧 UI 变更。
- 本地只验证 Auth/query/SQLC、现有 session、A01/P2 直接 Auth acceptance、
  `27→28→27→28` 和 repo-contract/secret；不重放无影响领域、全系统 acceptance 或 20 万数据。

## 修正、外部门与进度口径

- `slice_induced=0`、`infra_induced=1`、`verification_induced=3`：初始隔离数据库尚未建表，
  先用现有 A01 兼容脚本建立 27 基线再只读核验；激活环境后辅助 `rg` 不在 PATH，
  改用未覆盖 PATH 的只读检索完成核验；首次 A01 聚焦回放在历史脚本证明 27 后直接
  运行已要求 28 列名的黑盒，仅在该证据后迁回当前 waterline 再重放既有黑盒。
  三项都不改运行时或业务范围；首个聚焦负例的临时目录清理命令被本机安全策略在执行前拒绝，
  改用无删除动作的同一临时夹具后负例通过，记为一项 infra，redline=none。
- 本片不计入 781/293/316 业务进度；不修改历史 evidence/ledger 条目、旧 migration、
  源映射、旧 UI、业务 DTO 或终态。
- `PRODUCTION_DATABASE_NOT_EXECUTED`、`LIVE_MIGRATION_NOT_EXECUTED`、
  `REAL_WECOM_NOT_EXECUTED`、`REAL_SEND_NOT_EXECUTED`、
  `REAL_PAYMENT_REFUND_NOT_EXECUTED`、`DEPLOYMENT_NOT_EXECUTED`。
