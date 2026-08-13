# P3-I4A：verified unionid 自动客户归并事务

## 输入与目标

- Base SHA：`cc2178455fddde7b58f63fae4e594c2f45f72d70`。
- Phase：P3 / identity wave / implementation。
- task_inputs：`docs/spec/AI-CRM-v2-执行方案-v2-至P3.md`、
  `docs/spec/AI-CRM-v2-重构详细设计.md`、P3-I00、ADR-003、ADR-004。
- 已冻结中央契约：`internal/{identity,contact,events}/port`、
  `migrations/00010_identity_storage.sql`、contact-owned `MergeCustomers` 与
  `customer_merge_lineage`；本片只消费，不修改它们。
- 目标：将 Bind 已判定的、同一 `wechat-open-platform:<account-id>` scope 的
  verified unionid 冲突，原子执行为一次可审计的两客户自动归并。

## 冻结行为

- 仅当两个 active customer roots 中**恰好一个**持有 verified
  `wecom_external_userid` 时自动归并；该 root 为 primary。主记录选择不依赖
  customer ID，锁的升序仅用于死锁规避。
- 同一 UoW 内固定顺序为：contact `MergeCustomers`（标签并集、soft delete、
  lineage 保留）→ identity 重绑 merged customer 的全部 identities → identity
  append-only `customer_merges` audit → `customer.merged` event_log → completed
  Bind receipt。任一步失败完整回滚。
- audit 仅保存 versioned HMAC fingerprint 与闭合 detail；`customer.merged` payload
  固定为 primary/merged customer ID、audit ID、`auto`、
  `verified_unionid_unique_wecom_v1`，不含 raw/normalized external identity。
- 同 key 同规范化 payload replay 首次 `merged` 结果，不重复 contact merge、audit
  或 event；不同 payload 仍 fail-closed。
- 真实 PG16.14 以两个独立连接并发同一 Bind，最终只允许一个 effective primary、
  一个 audit、一个 `customer.merged` event 和一个 completed receipt；不得遗留
  orphan identity 或半归并。

## I4A 边界

- verified phone 冲突在 I4A 保持非自动归并（现有 `rejected` 分流）；创建 durable
  merge review / `manual_review` 结果属于后续 I7 人工待办片，避免在本片抢 migration
  与 review 生命周期中央契约。
- 双方皆有或皆无 verified WeCom identity、跨 scope unionid、declared phone、
  openid、external_userid 与 `ext:*` 均不自动归并。
- 不触及 migration、ADR、OpenAPI、public port、HTTP/API/UI、Ingest、pending replay、
  review、根依赖、真实 WeCom、生产数据库或 live migration/cutover。

## 路径与验收

允许：

- `internal/identity/app/bind{,_test}.go`
- `internal/identity/store/{repository.go,queries/identities.sql,generated/**}`
- `acceptance/identity/{bind_integration_test.go,normalize_upsert_integration_test.go}`
- `acceptance/contactfixture/contactfixture.go`（仅 Contact-owned 测试准备数据）
- `scripts/{generated-sources.sha256,check_generated_sources.sh,check_repo_contract.sh}`
- `docs/execution/{slices/P3-I4A.md,slice-ledger.yml}`

验收：isolated PostgreSQL 16.14 migration up/down/up、identity storage acceptance、
Bind success/replay/verified-phone-no-auto/rollback/two-connection race、generated source、
repo-contract、Go、Web 与 secret scan。生产、live migration、真实 WeCom 与外发均为
`NOT EXECUTED`。

## 回滚

若本片已合并需撤销，revert 对应 PR；不会执行生产数据库回滚或任何外部写操作。
