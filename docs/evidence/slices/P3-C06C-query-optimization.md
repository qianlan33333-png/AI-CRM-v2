# P3-C06C 客户列表查询优化证据

- evidence_class：`LOCAL_IMPLEMENTATION_AND_AUTHORIZED_TEST_SERVER_OBSERVATION`
- base_sha：`ea6e9771dfa02db06ec793a1aebdad89826eabcc`
- branch：`slice/p3-c06c-bounded-total-query`
- 授权测试服务器：2 CPU / `MemTotal=3813268 KiB` / `SwapTotal=4263928 KiB`
- 隔离数据库：PostgreSQL `160014`，仅 `127.0.0.1:55433/aicrm_perf`
- 隔离参数：`shared_buffers=1GB`、`effective_cache_size=2GB`、
  `work_mem=8MB`、`max_connections=40`
- synthetic 数据：200,000 customers / 600,000 customer_tags / 10,000 deleted /
  hot active 500 / hot deleted 500，generator exit 0。

## 首轮观察与根因

从 A2 merge SHA 的干净 checkout 构建的 runner 完成完整 4,096 场景：

- runner exit `1`，`passed=false`；离线 verifier exit `1` 并拒绝该收据；
- receipt SHA256：`8c15d9a5485241e0f51a0f4f94572d0e818d843b980ffaa6b35d1b693e0e8efb`；
  bytes：`60,459,321`；
- 81,920 measured calls；global P50 `16.62553ms`、global P95 `106.031866ms`、
  global max `477.05904ms`；
- 80 个单场景 P95 `>=200ms`，因此即使 global P95 低于 200ms，硬门仍按规则失败；
- 最慢场景为 selectors `17`（keyword + tag）、active、added-after、
  interact-after、next page、limit 50：P50 `323.951143ms`、
  P95 `460.673967ms`、max `477.05904ms`；
- 4,096 份 `ListCustomerIDsBounded` 计划的最高 execution time `426.547ms`，
  4,096 份 `ListCustomers` 计划最高 `97.459ms`；两类计划均无目标 Seq Scan。

旧实现每次会把最多 10,001 个 ID 传回 Go，只用 `len` 表达 bounded total。
完整门包含约十万次 repository 调用，因而该形态会造成近十亿 ID 的最坏无效
传输。该失败基线仅证明需要优化，不构成性能通过。

独立只读复核同时确认 pgx named prepared statement 在参数矩阵中可能切换为
generic plan，使 `is_deleted=false` 的 partial index 前提不可证明。当前片据此：

1. 将 bounded ID 列表改为数据库内 capped count 的单行 `bigint`，保持
   `min(real_total, 10001)` 与 10k+ 业务语义不变；
2. 仅对 contact repository 的这两条参数敏感查询使用
   `QueryExecModeCacheDescribe`，不全局修改 pool、不增加第三条 SQL；
3. runner 继续严格要求 count → page 两条生产查询、两份完整 raw EXPLAIN、无
   `customers`/`customer_tags` Seq Scan；
4. source-policy 仅豁免精确 repository 文件中 `customerQueryDBTX` 的同名
   `Query/QueryRow` 方法内由 AST 对象身份绑定的 receiver 调用
   `db.Tx.Query/QueryRow`；错误 receiver、同名 shadow receiver、复制路径与
   Exec 均永久 FAIL。
5. 强类型启动配置和 API/worker pool 构造均拒绝
   `description_cache_capacity < 1`，避免合法 DSN 在运行时禁用本片要求的
   description cache；错误保持脱敏且不改全局 query mode。

## 本地验收

- focused store/runner race：`PASS`
- focused store/runner vet：`PASS`
- PG16 `acceptance/p3c01b` on isolated local 55432：`PASS`
- sqlc 连续两次：`PASS / no diff`
- generated source + gitless generation：`PASS`
- source-policy/architecture lint 及永久负例：`PASS`
- clean `make ci-go`（含 release image 与 exact 40-character SHA query-plan gate）：`PASS`
- PostgreSQL 16.14 migration up/down/up、P3 contact acceptance、River official
  up/down/up（仅本地 55432）：`PASS`
- Web CI（Node 24.18.0 / npm 11.12.1，186 tests/build/audit）：`PASS`
- repo-contract checker + 永久负例全集 + sensitive path/gitleaks 边界：`PASS`
- A2 main SHA 的 2C4G 完整 4096 场景：`EXECUTED / HARD_GATE_FAILED`
- P3-C06C merge SHA 的 2C4G 完整 4096 场景：`NOT EXECUTED`

首轮 A2 merge SHA 的在途/失败基线不能作为优化后性能通过证据。P3-C06C 必须
先完成完整门禁、中文 PR、squash merge 与精确 main CI，再从新 main SHA 重建
runner 并从头执行完整矩阵。生产、legacy、真实企微和 live migration 均为
`NOT EXECUTED`。
