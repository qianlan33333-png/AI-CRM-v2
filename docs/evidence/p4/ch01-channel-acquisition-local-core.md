# CH01 渠道获客本地核心：V2 后端能力收据

CH01 补齐两个本地后端 operation：读取渠道获客前置条件，以及以完整替换方式更新
1 至 5 名有效企微成员的分配策略。更新支持 `ratio` 与 `cap_switch`，严格校验成员唯一性、
比例合计、扫描上限、Admin/Ops global RBAC、session-bound CSRF 与 Idempotency-Key；持久化、
Channel receipt 和事件继续复用既有同一事务。

旧 `/api/admin/channels` 合同仍只投影一名 primary active assignee，避免多成员 V2 数据破坏
冻结的 legacy `maxItems: 1` 响应。旧 completed receipt 仍可精确重放，不会产生第二次写入或事件。

获客预览只读取本地 Channel projection。即使已有 legacy scene、二维码 URL 或分享链接，响应也
固定 `entrant_ready=false`、`provider_execution_eligible=false`、
`real_external_call_executed=false`，并以 `provider_asset_unverified` 明确缺少 Provider asset
receipt。CH01 不生成或下载企微二维码，不同步外部成员，不创建 entrant，不调用 Provider，也不包含前端。

本收据证明的是 CH01 本地后端能力及 OpenAPI/generated 合同。required CI、main 合并、Batch 1
exact-main Nightly、部署、企微 Provider 真实效果与 receipt/reconciliation 均须分层报告，不能由
本地测试或本文件推导为已上线。
