# B1-WC01 企微标签效果运行时：后端能力收据

本业务包将既有 `LEGACY-API-0558/0559/0560/0561` 从 opaque 本地排队补齐为 typed
WeCom 标签效果：catalog sync、mark、unmark。HTTP 仍由 session、global Admin/Ops RBAC、
CSRF 与 Idempotency-Key 保护；`external_userid`、`tag_id/tag_ids` 使用严格 allowlist，CorpID
仅由服务端配置注入。

旧 Contact receipt/event/River acceptance 与 typed EER effect、accept/queue receipt 及独立
River job 共用同一个数据库事务：任一 typed 校验、EER、存储或 River 入队失败时全部回滚，
不会留下无法执行的 legacy queued receipt。相同幂等键只返回同一组持久化事实，不扫描或
重放历史 `00038` receipt；若 queued legacy receipt 没有既有 typed binding，重试会 fail closed，
不会借请求补建 EER 或 River job。`00088` 只新增 WeCom 自有的 effect 与 catalog projection
表，不回填旧命令，也不修改 Contact/Identity 数据。

worker 当前硬绑定 `DisabledProvider`，因此部署后也不会自动调用企微。真实 adapter 未获用户
授权前保持零网络；本地执行落为 `final_failed` receipt。若未来真实 adapter 返回
`outcome_unknown`，River 不自动重试，必须通过
`POST /api/admin/wecom/tag-effects/{effect_id}/reconcile` 提交 generation/fence、证据摘要及
`provider_applied/provider_not_applied`。通用 EER cancel/retry/reconcile 明确拒绝该 typed effect，
避免绕过 WeCom projection 与 reconciliation receipt。

验证目标 `make p4-b1-wc01-wecom-tag-effect-acceptance` 在隔离 PostgreSQL 16.14 上覆盖
87→88→87→88、历史数据不重放、typed 入队失败时 legacy acceptance 原子回滚、EER/River
receipt、disabled/unknown/reconcile、catalog 快照完整性与已填数据 `SQLSTATE 55000` 回滚
保护；focused/race、OpenAPI/generated、required CI 另行分层记录。

本收据只证明本地后端能力。Batch 1 exact-main Nightly、部署、企微 Provider 真实效果与
receipt reconciliation 外部证据均仍为 `NOT_EXECUTED`。
