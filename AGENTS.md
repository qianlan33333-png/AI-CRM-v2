# AGENTS.md

本文件适用于整个仓库。

## 1. 权威与权限

- 优先级：用户最新指令 > 已决 ADR/canonical architecture > 详细设计 >
  执行方案 > Slice 卡。
- ADR 只能解决明确冲突，不能删减用户验收标准。
- 未明确授权时，禁止部署、修改服务器/旧系统、连接真实用户数据、执行
  真实企微写操作或运行 live migration。
- 每个 Slice 必须基于精确 `main` SHA；不得顺手扩大修改范围。

## 2. 八条架构铁律

1. 域之间只能导入 `internal/<domain>/port` 或使用领域事件；禁止导入其他
   域的 `app`、`store`、`http`、`worker`。
2. `customers`、`customer_tags`、`customer_events` 只允许 contact 写。
3. 企微写 API 只允许 outbound 调用；企微读 API 只允许 wecom 调用。
4. 任何业务状态变化必须与 `event_log` 在同一数据库事务提交或回滚。
5. 配置只能通过 config 的强类型结构读取；禁止散落 `os.Getenv` 或直查
   settings。
6. `identities`、`customer_merges`、`pending_events` 只允许 identity 写；
   外部标识只能经 identity port 解析、绑定或归因。
7. 客户定制组件不进入核心仓库，只能通过 gateway Extension API 接入；
   `/examples` 仅允许不可部署的测试参照。
8. 业务周期任务一律使用 River periodic jobs；禁止 `time.Ticker`、
   `time.AfterFunc` 和第三方 cron。

## 3. OneID 与事务

- `customers.id` 是渠道中立 OneID；customers 表不得出现 external_userid、
  unionid、openid 或手机号列。
- identity 的 `customer_id` 可空。无唯一可信归因时必须保持 floating
  identity/pending event，禁止猜测。
- 只有 verified unionid 冲突可自动合并客户；verified phone 冲突进入
  人工队列；declared phone 与跨 scope openid 不得自动桥接。
- 跨 contact/identity/events 的创建、绑定、归并通过共享 transaction-bound
  UoW 与公开 port 完成，禁止跨域直写表。

## 4. Slice 边界

- 同时只允许一个外部 Pro Slice 在执行。
- 一片只解决一个行为或状态转换、一个模块、一个 API operation 或一个
  UI flow；API 与 UI 不得同片。
- 最多 8 个手写文件、400 行手写 diff、一个预先冻结的依赖或迁移。
- `.github/**`、ADR、架构、OpenAPI、migrations、公共 ports、根依赖与
  黑盒验收夹具默认由 Codex 独占修改。
- Pro 不执行 Git、GitHub、部署、真实迁移或外部调用。

## 5. 生成、测试与证据

- oapi-codegen、sqlc、Orval 生成目录禁止手写；连续生成必须无 diff。
- `go.sum`、`package-lock.json` 缺失或 `go mod tidy`/`npm ci` 后出现未解释
  diff 都是硬失败。
- mock/synthetic/local/staging/production 证据必须分别标注；未执行写
  `NOT EXECUTED`，外部未授权门写 `PENDING_EXTERNAL_GATE`。
- 所有 PR 使用中文说明，记录命令、退出码、生成物/锁文件差异、未执行项
  和回滚方式。
