# P4-ORDER-AB-REPLAY：订单 A+B 完整板块

## 基线与冻结合同

- 基线是 exact-green main `52653d02f22ee8261f13590ec91fad6169288d91`；工作区为 `codex/p4-order-ab-replay-52653d02`。客户标签 PR 已 CLOSED，开始时未合并 PR 为 0，四项必需检查均成功。
- 重放的接口清单严格为 `LEGACY-API-0119/0120/0316/0317/0405/0406/0407/0463/0464/0518/0519/0520/0521/0522/0523/0524`，共 16 条。`LEGACY-S07-174..183` 是完整业务清单；174 已上线的 I03 列表只作为回归，不替代其余能力。
- `LEGACY-T14-022/023/297..308` 保持 ARCHIVE_ONLY、DROP_CANDIDATE 或 PENDING_TARGET_SCHEMA：不导入、激活或伪造旧支付表。只建立 Order-owned 的本地导出、退款意图、外部处理与幂等收据。
- 本重放不读取旧 Python 源码，不整体 cherry-pick 旧候选；不做 UI、tenant、生产数据库、真实支付/退款、provider 调用、外部推送、回调或部署。

## 已冻结的业务语义

- 所有订单、微信交易和支付宝交易读取统一订单投影；明细和订单项只返回持久化商品快照。`cursor` 的非空编码没有冻结证据，必须 fail-closed。
- CSV 导出按管理员、操作和幂等键保存同一 UoW 内的本地回放收据；同键不同内容冲突。
- 退款仅保存退款意图、外部处理闸门、幂等收据与 Order 事件；初始状态为 `pending_external_gate`，没有 provider 调用。
- `outcome_unknown` 不自动重试；兼容“重试”入口只记录人工复核请求，数据库强制 `auto_retry_allowed=false`。Order 通过 Events appender 在同一 UoW 写事件，不直写 Event 表。

## 迁移与验收

- 主线已有 00038 客户标签迁移；订单重放使用新编号 `00039_order_ab_board.sql`，并验证 `38→39→38→39`。直接消费者在当前水位 39 上验证，历史片段测试仍保留其各自水位。
- 本地验证仅可使用 `127.0.0.1:55437/aicrm_test`。真实生产、live migration、支付、退款、provider/external effect 和部署均为 `NOT EXECUTED`。
- `PRODUCTION_DATABASE_NOT_EXECUTED`、`LIVE_MIGRATION_NOT_EXECUTED`、`REAL_PAYMENT_NOT_EXECUTED`、`REAL_REFUND_NOT_EXECUTED`、`REAL_PROVIDER_OR_EXTERNAL_EFFECT_NOT_EXECUTED`、`DEPLOYMENT_NOT_EXECUTED`。
- PR、merge、exact-main 与 promotion 收据只在真实发生后追加，禁止预填。
