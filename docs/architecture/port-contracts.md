# 跨域 Port 契约

本文冻结跨域语义，不提供业务实现。未来 Go 类型必须位于
`internal/<domain>/port` 并与本文语义等价；其他域禁止导入该域的
`app/store/http/worker/generated`。

## 通用约定与事务

- `CustomerID`、`EventID`、`PendingEventID`、`BatchID`、`TaskID` 为正整数 ID；
  未适用于某结果时为零且不得序列化成伪值。
- `Actor` 与 `Source` 是经服务端认证后得到的稳定标识，不信任请求体自报值。
- 业务状态是正常结果；校验、权限、基础设施失败才返回 error。adapter 按 ADR-006
  映射 HTTP，不得把 `not_found/conflict/manual_review` 折叠为 bool 或字符串 error。
- `platform/port.UnitOfWork.Within(ctx, func(txCtx context.Context) error) error`
  创建一个数据库事务。回调必须把 `txCtx` 原样传给事务型 port；任一步 error 或
  panic 整体回滚，成功才提交。事务型 port 收到非 txCtx 返回
  `ErrTransactionRequired`；禁止嵌套开启新事务或在事务内做外部网络调用。
- 新客户+首身份、客户归并+审计+领域事件、状态变更+`event_log` 必须在同一个
  `Within` 中完成。River 和 Webhook 只在提交后消费。

## identity/port

```go
type IDKind string       // wecom_external_userid|unionid|mp_openid|oa_openid|alipay_user_id|phone|ext
type Assurance string    // verified|declared
type IdempotencyScope string // server-derived authenticated principal/integration

type IDRef struct {
    Kind IDKind
    Scope string
    Value string
    Assurance Assurance
    Source string
}

type ResolveStatus string // found|not_found|conflict
type ResolveResult struct { Status ResolveStatus; CustomerID CustomerID }

type BindStatus string // bound|already_bound|merged|manual_review|rejected
type BindCommand struct {
    CustomerID CustomerID
    Ref IDRef
    Actor Actor
    IdempotencyScope IdempotencyScope
    IdempotencyKey string
}
type BindResult struct {
    Status BindStatus
    CustomerID CustomerID
    PrimaryCustomerID CustomerID
    MergeAuditID int64
    ReviewID int64
}

type IngestStatus string // attributed|pending|conflict
type IngestCommand struct {
    Refs []IDRef
    EventType string
    Payload json.RawMessage
    Source string
    OccurredAt time.Time
    IdempotencyScope IdempotencyScope
    IdempotencyKey string
}
type IngestResult struct {
    Status IngestStatus
    CustomerID CustomerID
    EventID EventID
    PendingEventID PendingEventID
}

type MergeReviewStatus string // pending|approved|rejected
type MergeReview struct {
    ReviewID int64
    Status MergeReviewStatus
    Kind IDKind
    Scope string
    IdentityFingerprint string
    CustomerIDs []CustomerID
    Version int64
    CreatedAt time.Time
    ResolvedAt *time.Time
}
type MergeReviewPage struct { Items []MergeReview; NextCursor string }
type ApproveMergeReviewCommand struct {
    ReviewID int64
    ExpectedVersion int64
    PrimaryCustomerID CustomerID
    Reason string
    Actor Actor
    IdempotencyScope IdempotencyScope
    IdempotencyKey string
}
type RejectMergeReviewCommand struct {
    ReviewID int64
    ExpectedVersion int64
    Reason string
    Actor Actor
    IdempotencyScope IdempotencyScope
    IdempotencyKey string
}

type Service interface {
    Resolve(context.Context, IDRef) (ResolveResult, error)
    Bind(context.Context, BindCommand) (BindResult, error)
    Ingest(context.Context, IngestCommand) (IngestResult, error)
}
type ReviewService interface {
    ListMergeReviews(context.Context, string, int32) (MergeReviewPage, error)
    ApproveMergeReview(context.Context, ApproveMergeReviewCommand) (MergeReview, error)
    RejectMergeReview(context.Context, RejectMergeReviewCommand) (MergeReview, error)
}
```

`IdempotencyScope` 只能由认证 principal 或 integration 的稳定内部标识构造，不能从
HTTP body、`IDRef.Scope`、Actor 文本或自报 source 复制。raw `IdempotencyKey` 只存在
于调用栈，repository 仅持久化其 32-byte SHA-256 digest。

- 调用者传入原始 Value；Kind、Scope、Value 的验证和内部规范化只在 identity
  完成，调用者不得 lower-case、拼接或自建映射。Scope 对所有 Kind 必填：
  unionid 使用 `wechat-open-platform:<account-id>`，openid 使用
  `wechat-app:<appid>`，企微 external userid 使用 `wecom-corp:<corp-id>`，
  phone 使用 `phone:e164`，支付宝使用登记的 app namespace，自定义 Kind 固定为
  `ext` 且 scope 使用 `ext:<namespace>`。
- `found` 只允许返回一个非删除客户；零个为 `not_found`，多个/矛盾可信边为
  `conflict`，不得挑一个猜测。冲突不向 Extension 泄露候选 ID。
- `bound` 是新绑定，`already_bound` 是同一客户幂等重放，`merged` 仅允许 verified
  unionid 规则完成，`manual_review` 用于 verified phone，其他不可信桥接为
  `rejected`。`PrimaryCustomerID/MergeAuditID/ReviewID` 只在对应状态有效；v1 的
  ReviewID 指向 identity 在 `pending_events` 中持久化的 `merge_review` 项。
- `attributed` 必须已写客户时间线和 event_log；`pending` 必须已持久化待回放事件；
  `conflict` 必须持久化冲突而不得落到任意客户。IdempotencyKey 必填。
- mutation 的持久幂等状态只在 identity-owned `identity_operation_receipts`，禁止复用
  `pending_events` 或进程内 cache。reservation、全部领域写、event_log 与 completed
  result 同一 UoW；唯一并发算法为 `INSERT ... ON CONFLICT DO NOTHING RETURNING`，
  loser 在同一未 aborted transaction 的新 statement 读 completed receipt。
- 同 key 同规范化 payload 返回原闭集 result；异 payload 为 409 且零副作用。普通
  INSERT 捕获 `23505` 后 SELECT、SELECT-before-INSERT、跨 UoW complete 与提交
  `in_progress` 均禁止。未知 schema/state/result 或缺历史 HMAC key fail-closed。
- raw identity 永不持久化；normalized value 只允许在 `identities.normalized_value`
  与必要精确查询。fingerprint 只用于 review/audit，不能作为 Resolve key。

### Typed HMAC keyring

identity 只依赖 `config/port.HMACKeyring` 的签名/验证能力，不读取 key bytes、settings
或散落环境变量。purpose 固定区分 review fingerprint 与 receipt payload；当前版本可
签名，仍被 `fingerprint_key_version` / `payload_hmac_key_version` 引用的旧版本只能
verify-only 且不得删除。实现只允许从部署环境或只读挂载 secret source 构建 keyring，
不得通过 HTTP、settings、日志或 error 注入/导出。
- HTTP/admin adapter 只能构造 `declared/admin` 的 IDRef；verified 证据只能来自已完成
  provider 验真的内部 adapter。identity 必须再次校验 Kind/Scope/Source 组合。
- verified unionid 自动合并采用 `verified_unionid_unique_wecom_v1`：恰好一个
  effective root 具有 verified wecom external identity 才能成为 primary；无唯一
  primary 时返回 `manual_review`。锁 ID 顺序不得被当作业务 tie-break。
- ReviewService 的 CustomerIDs 必须稳定排序且恰好两个 current roots；三项以上保持
  conflict 并进入独立人工调查，不允许一次 approve 隐式 N-way merge。IdentityFingerprint 是不
  可离线枚举的 `hmac-sha256-v<version>:<128-bit-base64url>` 展示值，key 来自 typed
  secret store；不得返回 raw/normalized external identity 或无密钥 hash。approve
  在同一 UoW/锁内重验 pending/version/current roots/evidence，primary 必须属于当前
  candidate roots，漂移统一 409 且零副作用。approve/reject 使用幂等键，merge review
  不参与自动 pending replay。
- `BindResult.customer_id` 是同一 UoW 锁定并重验后、执行本次 merge 前的请求
  CustomerID effective root；merged 时 `primary_customer_id` 是归并提交后的最终
  root，二者可相同也可不同。

## contact/port

```go
type CreateForIdentityCommand struct {
    Name string
    OwnerStaffID *int64
    ChannelID *int64
    Actor Actor
}
type MergeCustomersCommand struct {
    PrimaryID CustomerID
    MergedID CustomerID
    Actor Actor
    Reason string
}
type ExternalEventCommand struct {
    CustomerID CustomerID
    EventType string
    Payload json.RawMessage
    Actor Actor
    OccurredAt time.Time
    IdempotencyKey string
}

type MergePort interface {
    CreateForIdentity(txCtx context.Context, cmd CreateForIdentityCommand) (CustomerID, error)
    MergeCustomers(txCtx context.Context, cmd MergeCustomersCommand) error
    AppendExternalEvent(txCtx context.Context, cmd ExternalEventCommand) (EventID, error)
}
```

- 三个方法都要求 txCtx。命令不含也不保存任何 external ID。
- MergeCustomers 只修改 contact 所有表：保留主客户、软删除从客户、标签取并集，
  时间线保持 append-only 并通过合并谱系在主客户视图呈现；不得写 identities、
  customer_merges 或 event_log。重复同一归并必须幂等，反向/成环归并拒绝。
- AppendExternalEvent 按 IdempotencyKey 幂等追加，不覆盖历史事件。

## events/port

`customer.merged` 的公共 payload 固定为
`{primary_customer_id, merged_customer_id, merge_audit_id, mode, policy_version}`；
所有 ID 为正数，mode 仅 `auto|manual`，policy_version 必须为持久化审计版本。
payload 禁止外部 identity、PII 或 raw match key；生产类型为
`events/port.CustomerMergedPayload`。event idempotency key 固定为
`customer.merged:<merge_audit_id>`；audit、payload 与 event 在同一 UoW，重放不得
产生第二条事实。

```go
type Event struct {
    Type string
    CustomerID CustomerID
    Payload json.RawMessage
    OccurredAt time.Time
    IdempotencyKey string
}
type Appender interface {
    Append(txCtx context.Context, event Event) (EventID, error)
}
```

Append 要求 txCtx，按 IdempotencyKey 幂等写 `event_log`。它不分发、不调用网络；
提交后的 dispatcher/River 是至少一次，消费者必须以 event ID/业务幂等键去重。

## segment/port

```go
type SegmentID int64
type CustomerID int64
type Definition json.RawMessage
type RefreshMode string   // manual|scheduled
type RefreshStatus string // idle|running|failed

type Segment struct {
    ID SegmentID
    Name string
    Definition Definition
    RefreshMode RefreshMode
    RefreshCron *string
    MemberCount int64
    RefreshedAt *time.Time
    RefreshStatus RefreshStatus
    CreatedAt time.Time
    UpdatedAt time.Time
}
type Page struct { Items []Segment; NextCursor string }
type MemberPage struct { CustomerIDs []CustomerID; NextCursor string }
type CreateCommand struct {
    Name string; Definition Definition; RefreshMode RefreshMode; RefreshCron *string
    Actor Actor; IdempotencyKey string
}
type UpdateCommand struct {
    SegmentID SegmentID; Name *string; Definition *Definition
    RefreshMode *RefreshMode; RefreshCron *string; Actor Actor; IdempotencyKey string
}
type RefreshCommand struct { SegmentID SegmentID; Actor Actor; IdempotencyKey string }
type Service interface {
    List(context.Context, string, int32) (Page, error)
    Get(context.Context, SegmentID) (Segment, error)
    Create(context.Context, CreateCommand) (Segment, error)
    Update(context.Context, UpdateCommand) (Segment, error)
    ListMembers(context.Context, SegmentID, string, int32) (MemberPage, error)
    RequestRefresh(context.Context, RefreshCommand) (Segment, error)
}
```

- `Definition` 是闭合 DSL v1 JSON；Segment 是其唯一 parser/compiler 和 `segments` /
  `segment_members` 写入方。调用者不能传 SQL、字段名、运算符或查询模板之外的任意
  可执行输入。DSL 语法、错误码与 S02 QueryProgram / S03 固定 sqlc query-family 分层
  要求见 `P3-S00.md`。
- `MemberPage` 只暴露 channel-neutral OneID；需要 Customer 展示数据的 HTTP adapter
  在 Segment 自己的读路径投影，其他域不得直接读 Segment store/generated。
- Create、Update、RequestRefresh 的 IdempotencyKey 必填。同 key 同规范化命令返回
  原事实；异 payload 返回 conflict 且零副作用。RequestRefresh 只接受 durable command，
  不代表 members 已更新、更不代表 outbound 已发送。
- scheduled refresh 与 members replacement 必须由后续 River `heavy` worker 在事务边界
  落地；跨域只能消费已提交事实或调用该 port，不能借 Segment definition 自行筛选。

## outbound/port

```go
type EnqueueStatus string // enqueued|already_enqueued|rejected
type EnqueueOneResult struct { Status EnqueueStatus; TaskID TaskID }
type EnqueueBatchResult struct { Status EnqueueStatus; BatchID BatchID; TaskCount int }

type Enqueuer interface {
    EnqueueOne(context.Context, TaskSpec) (EnqueueOneResult, error)
    EnqueueBatch(context.Context, BatchSpec) (EnqueueBatchResult, error)
}
```

`TaskSpec/BatchSpec` 必须含非空 IdempotencyKey、目标客户/人群和受支持消息模板；
enqueue 只持久化任务，不调用企微。相同 key+相同规范化 payload 返回
`already_enqueued` 和原 ID；相同 key+不同 payload 返回 `rejected`。

## 组合责任

- identity app 是 Create/Bind/Merge/Ingest 的事务编排者；contact 和 events 只执行
  自己的事务型 mutation。任何一个 port 失败必须整体回滚。
- automation/survey/wecom/gateway 只能调用上述 port 或发领域事件；不得读取 sqlc
  输出或写其他域表。外部调用只发生在事务提交后的 worker。
