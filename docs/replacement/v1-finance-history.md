# V1 交易历史：V2 本地只读投影

本包落实新的“全部源数据保留、按 V2 领域落表、仅供人工测试、不切流”要求。
旧 `migration-mapping.jsonl` 中 `LEGACY-T14-300/303` 是早期冻结的归档决策；
本补充只增加非执行型历史投影，不恢复旧支付/退款协议或运行态，不把旧归档决策改成 Provider 成功。

## 两张源表

| V1 归档源 | V2 正式目标 | 保留内容 |
| --- | --- | --- |
| `wechat_pay_orders` | `order_list_projections`，`record_origin=v1_history` | 原单号、交易号、状态、整数分金额/CNY、客户/商品名称快照、身份值、原时间 |
| `wechat_pay_refunds` | `order_historical_refunds` | 原退款 ID/编号、Provider 退款号、交易号、状态、原始原因、整数分金额/CNY、原时间，以及经本批回执核验的 V2 历史订单 ID |

全部其余源字段仍在加密归档内；不把 prepay/凭据/请求响应材料复制成可执行配置。
不写 `order_refunds` 或 PE01 金融命令，不生成事件、EER、River、支付、退款或权益。
原生退款和付费权益读取排除历史订单；历史详情的可退款金额始终为零。
页面只展示历史退款，明确标注“V1历史只读，非V2支付/退款确认”。

## 映射与重复运行

- 必须先完成相同归档的 `v1-static-a1` 对账及指定 DM01 full/imported run。
- 客户只关联 DM01 源 HMAC、lineage、行回执和当前 root 全部一致的 V2 ID。
- 商品只关联静态导入回执、归档 payload 摘要及实际禁用商品字段均一致的 V2 ID。
- 源中缺失的客户/商品不创建替代对象：关联保持 NULL，原历史快照保留。已存在但冲突的映射终止导入。
- 退款只引用本次订单导入/回放验证得到的 V2 ID，不复制 V1 主键作为目标外键。
- 输入不符合目标契约、脱敏覆盖必需业务字段、缺少可导入父订单时逐行隔离；不截断原文、不改金额、不补造状态。
- 每个目标和迁移回执在同一事务内提交。重放比较 source/payload/field/target 摘要和实际目标字段；已封存回执不覆盖。

## 操作入口

只连接 V2 内的归档/目标库。需要以下现有环境项，不在命令行传密钥：

```text
AICRM_V1_ARCHIVE_TARGET_DATABASE_URL
AICRM_V1_ARCHIVE_ENCRYPTION_KEY
AICRM_DM01_SOURCE_HMAC_KEY
```

```sh
aicrm-v1-domain-import --domain=finance --mode=import \
  --archive-run-id=<reconciled-archive-run> --dm01-run-id=<verified-full-run>
aicrm-v1-domain-import --domain=finance --mode=reconcile \
  --archive-run-id=<reconciled-archive-run>
```

重复执行 import 和 reconcile 必须报告 replay，并保持目标数据与执行记录不变。
本包独立版本 `v1-finance-a1`，不重写首批十表或静态六表的 seal。
对账逐条比较归档 source/payload 摘要、目标全部静态字段和退款同批父订单归属。

## 验证与发布边界

先本地/包级验证，再集中 PR CI、最终 PR SHA 候选 Full Nightly；绿灯后合并，
exact-main Full Nightly 通过后才允许正式导入/部署。排队期间不提前合并或部署。
V1 只读，现网流量不动；本包不修改 DNS、旧反向代理、企微/OAuth/小程序入口。

冻结源聚合预检为 708 订单、115 退款；6 条订单无客户源记录、52 条无商品源记录，
这些是可能重叠的关联缺口，不能相加当订单缺口。预检不是正式导入或验收完成证明。
迁移/对账 run、实际 NULL 关联数量、版本、Nightly 和部署证据必须在执行后单独记录。

## 回滚

迁移 109 的 down 在存在任何历史订单/退款时拒绝执行。不得强行删除历史行或回执解除门禁。
应用回退与数据回退分开：保留新业务数据；如需数据恢复，将迁移前验证过的快照恢复至独立新库，
完成核验后再人工决定处理方式，不对 V1 或现有正式库执行 `pg_restore --clean`。
