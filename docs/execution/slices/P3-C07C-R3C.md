# P3-C07C-R3C：Contact external-event behavior-only

## 输入合同

- Base SHA：`f6f86909ad23a85f7a29b4f7c4c30c8d52ef5cc9`
- dependency：P3-C07C-R3B #160、Identity R6-B #163 与 Segment S04B #162 均
  CLOSED，exact-main application/go、repo-contract、secret-scan 全部成功，开放 PR 为 0。
- R3A 的结构化 ledger 历史零漂移保护、R3B 的正式 migration `00011` 与
  Identity C1 checker/ledger 必须原样保留。
- 历史 C07C 候选及重建提交只读，不 cherry-pick、不继续开发，旧计数不清零。

## 唯一可观察行为

caller 在已有 UnitOfWork callback 内调用 Contact `AppendExternalEvent`：

1. transaction advisory lock 按 `IdempotencyKey` 串行化，不另开事务。
2. 首次调用解析 effective root，同事务追加 `customer_events` 并写 R3B registry。
3. 同 key、同 type/payload/actor/occurred_at、同 effective root 返回原 EventID；
   JSON object key 顺序与 PostgreSQL 等价数字视为同一事实。
4. 同 key 异事实或异 effective root 返回 `ErrExternalEventConflict`，零新增、零覆盖。
5. 事件先写在后续被合并 customer 上时，以旧 ID 或最终 root 重放均返回原 EventID。
6. caller 后续失败时 timeline 与 registry 同时回滚，零残留。

## 边界与验收

- 只改 Contact port/store/sqlc generated runtime、PG16.14 behavior acceptance、最小
  Make/CI、card、append-only ledger 与永久 repo-contract 门。
- 不修改 migration、ownership、canonical，不写 identity/segment/event_log，不连接真实
  企微或生产数据库，不执行 live migration/cutover 或真实外发。
- 验收 transaction-bound、same key/fact/root replay、冲突、10-way concurrency、
  merge replay、rollback、focused race、sqlc 连续生成无 diff、完整 Go/Web/repo-contract。

## 修正与外部门

- `slice_induced_correction_count=0`
- `infra_induced_correction_count=0`
- `scope_induced_correction_count=0`
- `verification_induced_correction_count=0`
- `PRODUCTION_DATABASE_NOT_EXECUTED`
- `LIVE_MIGRATION_NOT_EXECUTED`
- `REAL_WECOM_NOT_EXECUTED`
- `OUTBOUND_EXTERNAL_EFFECT_NOT_EXECUTED`
