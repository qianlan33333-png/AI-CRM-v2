# P4-B02AB：客户标签 A+B 完整业务板块

## 接管与回放 receipt（2026-08-15）

- 新回放基线：`origin/main=4c204b0f250fe0eb9f2fc4004ec983590ef54a6a`；语义来源为干净候选 `1ed7bb82c068ef4ad2ab9ebdc5be06a1150de14c`，本次在新隔离 worktree 白名单回放。
- 已保全且未重写的来源提交：`36d58c1`（目录读取内核）、`95d5c5a`（sync 接受边界）、`f4ae45c`（live 入队状态机）、`6ffd257`（A+B 路由与安全入队）、`1ed7bb8`（OpenAPI 并行负例）。
- 读取盘点确认来源 worktree 无 staged 或未提交变更，`git diff --check` 通过，未发现指向来源 worktree 的并发写入进程；仅回放客户标签白名单，优惠券已合入内容保持实时主线版本。
- 回放开始时 main 无开放 PR；`application / go`、`application / web`、`policy / repo-contract`、`security / secret-scan` 均在 `4c204b0` 成功。该记录只确认本地候选状态，不将本地测试或队列接受状态表述为真实企微执行。

## 路由销账

- 既有 main / PR #215 保持 `LEGACY-API-0555/0552/0553/0556/0562`；本片不重复计数或重写其历史证据。
- 本地收口其余九条：`LEGACY-API-0086/0551/0554/0557/0558/0559/0560/0561/0563`。
- 0086 仅将已认证用户接到既有通用 admin shell；冻结证据仅证明 `TemplateResponse/shell_context` 调用链，未新建标签 UI 或复制旧实现（no tag page body is invented）。

## 安全边界

- Contact 仅持久化同一 UoW 的 receipt、Event 与 River insert-only job；HTTP 202 为 `queued` 接受，不是 WeCom 已尝试、已执行或已送达。
- session principal 是唯一 actor 来源；读取使用 `customers.read`，写入使用 `customers.write` 加 A01 CSRF。
- 未执行：`PRODUCTION_DATABASE_NOT_EXECUTED`、`LIVE_MIGRATION_NOT_EXECUTED`、`REAL_WECOM_NOT_EXECUTED`、真实 provider 调用、worker、外发及自动重试。`outcome_unknown` 不能自动重试，后续只能经 reconciliation。
- `LEGACY-T14-148/309/310` 不变：无旧数据读取、无 import、无 target schema 推断；feature matrix 中只属于旧浏览器 UI 的 `S05-011/012/019` 保持未实现。

## 本地验收

- PostgreSQL 16.14 gate：`37→38→37→38`，tag_groups/tags 历史记录保持；默认 gate 明确 `executed=false` 与 `real_external_call_executed=false`。
- PG black-box：并发 replay 收敛为一 receipt/Event/River job；Event 失败回滚；未完成 reserved receipt 被 deferred trigger 拒绝。
- 本地执行了 `make p4-b02ab-tag-acceptance`、tag transport 的 race 测试、生成物双次检查与完整性检查。

## 本地候选状态

- 本次授权已经覆盖原先冻结的共享收口；以接管基线的 exact-green main 完成所有本地验证后，才进入一次中文 PR、match-head squash 与 exact-main 四门关闭。
- 本地候选不等于上线：不做生产数据库、live migration、部署、真实企微或任何外发操作。
