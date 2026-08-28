# V1 Member Grid 正式历史落表（最小 A 逻辑）

本包只处理已封存的五张来源表 2541 行：视图 2、使用快照 2534 进入 Product 历史表；同步运行 2、协作者 1、旧分享 2 留在封存归档并逐行记账。不是重建当前授权、分享地址、实时登录状态或任意旧 Saved View，也不销项 S07-153/154/160/161/162 尚缺的现行业务语义。

## 冻结契约

- `00117` 新增 `product_v1_member_view_history` 与 `product_v1_member_usage_history`，不触碰当前成员、权限、分享、事件或任务表。
- 视图保留来源视图/周期商品 ID、名称、位置、默认标记、Schema/版本、时间和 Config 摘要；Config 与来源租户/创建人留在加密归档，不引入 V2 租户模型，不执行旧表达式。位置、Schema、版本保留负值，不凭名称猜测合法范围。
- `ProductID` 只来自已有封存周期定义 receipt + 真实 Product 目标核验；不能核验则 NULL，不把旧 ID 当新 ID。
- 使用快照的 `CustomerID` 仅通过 DM01 已核验 unionid 映射；其余来源身份留在归档。历史布尔值、学习计划计数、打开次数和时间原样转型，不当作当前权益或登录断言。
- 原归档把非敏感 `has_token_usage` 误脱敏。仅允许既定冻结 dump/run/table/field 的恢复凭证，经来源键、原 payload/field HMAC 和 entry HMAC 核验后在内存补回 bool；原归档/封存回执绝不改写。正式行保留原 `SourcePayloadDigest` 及 SHA256(JSON marshal 完整恢复凭证) 的 `RecoveryEntryDigest`。
- Writer/Journal 同事务，canonical SourceIdentifier 为 SourceKeyHMAC 小写 hex；重放核对真实行完整 typed digest（UTC microseconds），不能只相信 journal。
- AdminRead 四个 GET 列表/详情：视图仅允许 `product_id` 筛选，使用快照仅允许 `customer_id`；limit 1..100、默认 50、offset >=0，明确历史只读/外部调用 false。不透出原 payload、身份、授权或旧分享 token。

## 验证与上线边界

先用 V2 上已封存副本，在 network=none PostgreSQL 16.14 做真实 store roundtrip/rollback、全量导入、全量重放、两轮 source/receipt/typed-target 对账；运行中事件、队列与外部效果必须不增长。然后本地测试、集中 PR CI、最终候选 SHA Full Nightly、合并后 exact-main Full Nightly 均通过才进入 V2 正式库和独立人工测试入口。

V1 全程零写入，现有流量零切换，真实 Provider 效果关闭。完成历史迁移不是当前 Member Grid 全能力验收；仍须逐项列出 blocker 并等用户人工确认。
