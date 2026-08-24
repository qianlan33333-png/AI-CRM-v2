# Contact Owner Reassignment Local Core：V2 后端能力账本

## 口径与状态

开发基线为 `origin/main@24360c7e5af8da276272535b289677d2cd53a695`。本包提供的是
Contact-owned、本地数据库内的负责人变更能力，不包含前端、Excel、外部联系人标识、企微
userid、手机号、欢迎语、企微转属或任何 Provider 调用。

`LEGACY-S07-110` 至 `LEGACY-S07-115` 全部继续是 `NOT_STARTED/NOT_RUN`，
`docs/feature-matrix.csv` 为 **0 diff**；`LEGACY-S07-023` 不属于本包。新 V2 operation
并不等同于逐项完成 legacy 页面行为。

## 六个 V2 operation 与 legacy 差异

| V2 operation | 本地事实 | 与 legacy 的差异 | Matrix |
| --- | --- | --- | --- |
| `GET /api/v1/contact-owner-reassignments/template` | 只下载四列安全 CSV header | 不提供 Excel 或旧页面下载行为 | 110 pending |
| `POST .../previews` | 上传即严格解析并创建 durable、actor-bound、TTL preview/hash | **V2_DIFFERENCE**：不创建无业务价值的第二个临时 import session；preview 是唯一耐久的上传事实 | 111/112 pending |
| `GET .../previews/{preview_id}` | 查询同一 durable preview、行/错误/TTL 状态 | 没有前端 session 状态 | 112 pending |
| `POST .../execute` | confirmation phrase、preview hash、CSRF、admin-only RBAC、idempotency 后，原子更新 `customers.owner_staff_id` | 不执行企微 transfer，也不接受任何外部身份字段 | 113 pending |
| `GET .../errors.csv` | preview 创建后立即可稳定下载的脱敏 `line,code` CSV | 不输出原始 CSV、姓名、联系方式或外部标识 | 114 pending |
| `GET .../results.csv` | execute 后下载 customer/owner/version timestamp 的本地结果 | 结果不证明外部企微转属或旧 XLSX 文件 | 115 pending |

CSV 仅允许 `customer_id,expected_owner_staff_id,expected_updated_at,target_owner_staff_id`，
有 bounded byte/row 限制、严格 header、UTF-8、未知列拒绝、重复 customer 拒绝及公式前缀拒绝。
preview 有解析错误仍作为稳定、可下载 errors 的耐久资源；含错误 preview 不能 execute。

## 事务、安全与事实边界

- 新 capability `contact.owner_reassignment` 只赋予 global admin；ops/sales 被拒绝。
  上传与 execute 同时要求 browser session CSRF 和 Idempotency-Key；preview 的
  `(actor_id, idempotency_key_digest)` 是其 durable create receipt：同 key/同 CSV replay，
  同 key/不同 CSV conflict。
- migration `00070` 只建 preview/receipt 事实，不复制 Customer 或 Staff 主数据。目标 staff
  通过 canonical `staff.id` 在同一事务 `FOR SHARE` 校验 active；不读取 WeCom userid directory。
- execute 先排序去重锁 target staff，再依 `customer_id` 升序 `FOR UPDATE` 锁 Contact customer，
  锁后比较 expected owner/updated_at。任一 staff/customer/owner/version 漂移均 conflict 且批量
  零写；这与单客户 PATCH 使用同一 customers 行锁语义。
- 成功事务包含 customers owner write、每客户一条 Contact-owned `customer_events` 的
  `customer.updated` 事实和一条 event log、single-use preview result 与 idempotency receipt complete；
  同 key同 payload replay，同 key异 payload conflict。receipt/down guard
  保留事实，非空 migration down 返回 SQLSTATE 55000。
- local receipt/result 只证明本地提交、重放和数据库写入；它**不等于**企微转属、Provider 成功、
  Nightly、合并、部署或任何外部效果。
