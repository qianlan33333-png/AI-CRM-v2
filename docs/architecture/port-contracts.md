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
type IDKind string       // wecom_external_userid|unionid|mp_openid|oa_openid|alipay_user_id|phone|ext:<namespace>
type Assurance string    // verified|declared

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
type BindCommand struct { CustomerID CustomerID; Ref IDRef; Actor Actor }
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
    IdempotencyKey string
}
type IngestResult struct {
    Status IngestStatus
    CustomerID CustomerID
    EventID EventID
    PendingEventID PendingEventID
}

type Service interface {
    Resolve(context.Context, IDRef) (ResolveResult, error)
    Bind(context.Context, BindCommand) (BindResult, error)
    Ingest(context.Context, IngestCommand) (IngestResult, error)
}
```

- 调用者传入原始 Value；Kind、Scope、Value 的验证和内部规范化只在 identity
  完成，调用者不得 lower-case、拼接或自建映射。Scope 对所有 Kind 必填：
  unionid 使用 `wechat-open-platform:<account-id>`，openid 使用
  `wechat-app:<appid>`，企微 external userid 使用 `wecom-corp:<corp-id>`，
  phone 使用 `phone:e164`，支付宝和 `ext:*` 使用登记的 provider namespace。
- `found` 只允许返回一个非删除客户；零个为 `not_found`，多个/矛盾可信边为
  `conflict`，不得挑一个猜测。冲突不向 Extension 泄露候选 ID。
- `bound` 是新绑定，`already_bound` 是同一客户幂等重放，`merged` 仅允许 verified
  unionid 规则完成，`manual_review` 用于 verified phone，其他不可信桥接为
  `rejected`。`PrimaryCustomerID/MergeAuditID/ReviewID` 只在对应状态有效；v1 的
  ReviewID 指向 identity 在 `pending_events` 中持久化的 `merge_review` 项。
- `attributed` 必须已写客户时间线和 event_log；`pending` 必须已持久化待回放事件；
  `conflict` 必须持久化冲突而不得落到任意客户。IdempotencyKey 必填。

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
