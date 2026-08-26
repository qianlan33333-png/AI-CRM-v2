# P4 B1-WC02 企微联系人资料写回效果闭环

- 精确基线：`origin/main@61c7ae22c3b1e084c069b6eeea87e56546999493`。
- 行为边界：`PUT /api/sidebar/v2/profile` 继续以 owner scope、CSRF、CAS、幂等 receipt 和 UoW 为本地事实；只有 `wecom.outbound.enabled=true` 且权限确认后，才在同一事务用 verified `WeComOutboundTargetResolver` 解析 customer_id 对应的 owner userid/external userid，并以该本地 receipt ID 接受和排队 profile effect。配置未启用时保持 local-only。
- V2 canonical 映射：企微 `remark = 更新后 SidebarProfile.Name`，`description = 更新后 SidebarProfile.Description`。不扩展 API 请求字段，也不把 source/industry/needs/pain_points 猜测映射到 Provider 字段；超过企微 effect 限制的值会使整个事务 fail closed。
- Worker：仅在相同 outbound 配置启用时注册 `wecom_contact_profile_effect` 的真实 Provider adapter；API 成功响应只可声明 `effect_queued=true`、`provider_execution_eligible=true`、`real_external_call_executed=false`，不能把排队当作企微成功。
- 外部效果：本地测试 `NOT_EXECUTED`；Provider 测试仅使用 `httptest`，无真实企微写操作。专用迁移脚本为 `acceptance/wecom/b1_wc02_profile_effect_pg16.sh`。
