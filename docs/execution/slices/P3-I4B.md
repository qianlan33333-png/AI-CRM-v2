# P3-I4B：verified phone 人工归并待办

## 输入与目标

- Base SHA：`98b097989f54675dbe65bd9a9808f8692f9c058e`。
- Phase：P3 / identity wave / implementation。
- task_inputs：`docs/spec/AI-CRM-v2-执行方案-v2-至P3.md`、
  `docs/spec/AI-CRM-v2-重构详细设计.md`、P3-I00、P3-I1、P3-I2、P3-I3、P3-I4A。
- 已冻结中央契约：`migrations/00010_identity_storage.sql` 的 `pending_events` merge_review
  shape、Bind `manual_review` result、contact-owned customer roots；本片只消费，不修改。
- 目标：Bind 遇到 verified E.164 phone 的跨 active customer-root 冲突时，同一 UoW
  durable 创建一个可稳定重放的人工 merge review，而不执行客户自动归并。

## 冻结行为

- 仅 phone `e164`、identity assurance 为 `verified`、现有 identity 已绑定且请求 customer
  root 与现有 root 不同时进入本片；candidate roots 必须恰好两个、升序保存。
- 同一事务依次完成 `pending_events(kind=merge_review,state=pending,version=1)`、
  不含 raw/normalized identity 的 versioned 16-byte fingerprint、
  `identity.merge_review.created` 事件和 completed Bind receipt；任一步失败全回滚。
- 返回 `manual_review` 与 `ReviewID`，receipt 以同一 `result_pending_event_id` 重放同一事实。
  同 idempotency key 且同规范化 payload 不重复创建；同 key 不同 payload fail-closed。
- policy 固定为 `verified_phone_manual_review_v1`；不含审批、拒绝、或完整 merge 执行。

## I4B 边界

- 不重做 I4A unionid 自动归并；verified unionid 无唯一 primary 的 review、非 verified
  phone、declared phone、openid、external_userid 与 `ext:*` 都不在本片。
- 不触及 `migrations/**`、ADR、OpenAPI、public port、HTTP/API/UI、后台待办、Ingest、
  pending replay、根依赖、真实 WeCom、生产数据库或 live migration/cutover。

## 路径与验收

允许：

- `internal/identity/app/bind{,_test}.go`
- `internal/identity/store/{repository.go,queries/identities.sql,generated/**}`
- `acceptance/identity/bind_integration_test.go`
- `scripts/{generated-sources.sha256,check_generated_sources.sh,check_repo_contract.sh}`
- `docs/execution/{slices/P3-I4B.md,slice-ledger.yml}`

验收：isolated PostgreSQL 16.14 的 verified-phone review shape、replay、同 key 异 payload
冲突、event 失败回滚与双连接并发；并运行 migration up/down/up、identity acceptance、
generated source、repo-contract、Go、Web 与 secret scan。生产、live migration、真实 WeCom
与外发均为 `NOT EXECUTED`。

## 回滚

若本片已合并需撤销，revert 对应 PR；不会执行生产数据库回滚或任何外部写操作。
