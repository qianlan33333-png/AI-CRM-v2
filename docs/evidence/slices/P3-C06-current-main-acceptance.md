# P3-C06 当前 main 的 S 档 20 万客户性能验收

## 证据边界

- evidence class：`authorized_test_server_synthetic`
- source/main SHA：`2566eca1329f3234a6e6b9db4926ba745984dadd`
- application/go main CI：
  `https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31683860442`
- repo-contract main CI：
  `https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31683860230`
- secret-scan main CI：
  `https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/31683860427`
- 授权测试机：2 CPU、`MemTotal=3813268 KiB`、`SwapTotal=4263928 KiB`
- 隔离数据库：PostgreSQL `160014`，仅
  `127.0.0.1:55432/aicrm_perf`，容器 `aicrm-p3c6-2566-r2-pg`
- S 档参数：`shared_buffers=1GB`、`effective_cache_size=2GB`、
  `work_mem=8MB`、`max_connections=40`、`GOMEMLIMIT=768MiB`
- 未连接生产、staging、真实企微或真实用户数据；未运行 live migration、
  cutover 或真实外发。

## 初始化与可追溯构建

当前 SHA 的 S 档配置由 `cmd/aicrm-config --tier=s` 生成，首次启动前即以
`0644` 挂载；空卷通过 `POSTGRES_DB=aicrm_perf` 预建数据库，并在 goose 前机械
断言数据库存在、版本和五项 S 档参数。为适配累计 main 中新增的外键表，测试库按
`00001..00006 -> 官方生成器 -> 00007..00015` 初始化；runner 启动前再次断言
goose version `15`、数据量和 ANALYZE 统计。没有修改任何 migration 或业务表定义。

两个 Linux/amd64 静态二进制均从独立 clean clone 构建，`go version -m` 机械证明
`vcs.revision=2566eca1329f3234a6e6b9db4926ba745984dadd` 且
`vcs.modified=false`：

- generator SHA-256：
  `e8c35e7ab49ac23a71a336127805c8c982db124b0ccf9d6d29364c40b157ae24`
- runner SHA-256：
  `e92b28391d6c96de37951a1ff6b7cca7ac84a054af3b155365c5d5d360882aa3`
- S 档 `postgresql.conf` SHA-256：
  `3b22f16a5335f1fd9b6365e73953c9c2079f07fda7fac7543c04ff9f6adcd294`

固定 seed `20260812` 的 generator exit `0`、stderr `0`：200,000 customers、
600,000 customer_tags、10,000 deleted、hot active/deleted 各 500；seed receipt
SHA-256 为
`10c11ffe40d5f7b1575f1acea645bc7facda39be8863916327a825ee7bb66ee7`。

## 官方 4096 场景硬门

runner 完整执行 4,096 combinations、每场景 3 次 warmup 和 20 次 measured call，
共 81,920 measured calls。每个场景都经真实 `CustomerQueryRepository` 和 UoW，
固定执行 `CountCustomerIDsBounded -> ListCustomers`，并保存两份
`EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`：

- runner exit `0`、stderr `0`、`passed=true`
- global P50/P95/max：`27.159082 / 61.211741 / 138.412389 ms`
- 最慢单场景：
  `selectors-21-deleted-true-added-before-interact-closed-page-first-limit-50`
- 最慢单场景 P50/P95/max：`68.281939 / 125.607003 / 138.412389 ms`
- 单场景 P95 `>=200ms`：`0`
- `CountCustomerIDsBounded` / `ListCustomers` 计划数：`4096 / 4096`
- 两类计划最高 execution：`106.178 / 82.203 ms`
- `customers` / `customer_tags` forbidden Seq Scan：`0`

全部筛选、closed added/interact、first page、limit 200 的代表场景 P50/P95/max 为
`30.120840 / 51.882766 / 55.600385 ms`；其实际 custom plan 中 count/list
execution 为 `28.091 / 4.595 ms`，两者 forbidden Seq Scan 均为 `0`。

## 实际 SQL 与 generic-plan 观察

实际查询权威来源及 SHA-256：

- `internal/contact/store/queries/customers.sql`：
  `6a9971d742002aeda3719b56422c45bb809fefbaf96f7bf2dc107357f1fe6a16`
- `internal/contact/store/generated/customers.sql.go`：
  `48d489f58b6517e531eaa0c16f0d1b40f6462a8566b52b0e9429ccc62da73793`

`P3-C06-current-main-generic-plan.sql` 保留 generated `$1..$14` 参数顺序、两个
生产查询和本次官方全部筛选参数，SHA-256 为
`f9fc099e77d30a8a665eb975dbc82eeb5fb4abf7e55da459a967be7076f8d8fc`。
在同一 20 万数据、完整 `00015` schema 上设置
`plan_cache_mode=force_generic_plan` 后：

- count prepared stats：`generic_plans=1`、`custom_plans=0`；planning/execution
  `1.518 / 49.640 ms`；forbidden Seq Scan `0`
- list prepared stats：`generic_plans=1`、`custom_plans=0`；planning/execution
  `0.373 / 305.096 ms`；forbidden Seq Scan `0`
- count raw EXPLAIN SHA-256：
  `3bf42fcc79bb4a1bc7b4730879b4683b13dabdaa80821ffe2ad40c644326415d`
- list raw EXPLAIN SHA-256：
  `bb6e2a430297551ec6e5f73437fb0c40db024239a3be341fe3c0359c01833721`

generic list 的单次 execution 超过 200ms，不能冒充生产运行结果。当前 repository
正是为这两条参数敏感查询固定 `QueryExecModeCacheDescribe` custom plan；官方 P95
结论来自上述真实 repository 完整矩阵，且所有实际计划无目标 Seq Scan。本轮不需要
修改查询或索引。

## 收据、门禁与修正归因

- 完整 receipt：120,725,075 bytes，SHA-256
  `57cd92ddab3bf470006d74fd3683376e31f31092e07dc1049cf99efa2a34b09f`
- 同一 exact binary、source SHA、main CI URL 离线 verifier：exit `0`，
  `contact-perf-receipt: PASS`
- focused generator/runner/store race + vet：`PASS`
- `make ci-go`：`PASS`，含 `query-plan-gate: PASS (checked=27)`
- `npm ci --ignore-scripts --no-audit --no-fund && npm run ci`：`PASS`，
  9 files / 187 tests、build、high-level audit 0 vulnerabilities

历史失败尝试的 `infra_induced=2`（配置挂载权限、预建 `aicrm_perf`）按总指挥
纠正保留，不触发硬停。本次重放精确记录 `verification_induced=7`：测试 DSN 与新
容器密码绑定、累计 schema 的 seed 时序、两组本地完整门环境变量、Web 门命令、
receipt 汇总字段名和补丁工作目录；均未改业务语义。`slice_induced=0`，没有业务
查询、索引、migration、ownership、OpenAPI、feature matrix 或 ledger 改动。

回滚仅需 revert 本业务验收产物；远端容器和卷是隔离测试证据，可在保留收据后独立
停止/清理，不影响任何应用或生产数据。
