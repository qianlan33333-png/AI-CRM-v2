# V1 Group Ops 最小正式历史落表

目标只是在V2保存可人工查看的业务事实，不恢复旧群发、Webhook或执行状态。源仅为V2已有冻结加密归档；V1零修改、生产零切流。

- 12个计划复用GroupOps-owned `group_ops_plans`，固定archived/revision1、显式迁移actor；历史标记保存源ID/code/type/status/可验证owner关联和归档时间。所有原生修改入口必须继续拒绝archived计划。
- 36群记录与17群快照放入同一个GroupOps历史目录表，保留source_kind、源ID或chat_reference及原始时点；两类物理记录不冒充53个不同群，不写当前Provider目录。
- 14条计划群关系、3个旧日程节点各使用一个GroupOps-owned历史表。旧day_index/trigger_time/content_package是非执行历史事实，不改写成当前message/delay节点。
- 四个新增历史表预留迁移113，由主代理串行生成SQLc；保留NULL、原时点、原字段值，不复制tenant、Webhook/签名/运行凭据。未知owner保持NULL，不把V1 staff数值当V2外键。
- owner历史writer + journal使用caller UoW，完整实际target digest核验重放；目录可按source_kind+source key归属两张源表。五张业务表共82行；六张运行表131行只归档并独立终态，不回放。
- 页面复用群运营入口增加只读历史列表/明细；当前业务流程用另外的本地UAT数据验证，历史不冒充当前可执行配置。
- 本地/PR CI/最终候选Nightly绿→合并→exact-main绿→V2正式落表；当前只完成隔离开发与演练，未正式部署。

## 隔离验证（2026-08-28）

- V2 network-none PG16.14演练库迁移112→113成功；HistoricalStore PostgreSQL事务回滚测试通过。
- 真实冻结213来源行：82导入、131只归档、0隔离；重复导入213行全部重放。逐行源HMAC/payload与实际owner目标摘要核对213/213，重复封账通过。
- 对账摘要：`e27c59867fdbc9cf40078bbf9fab5db8dd6172dec1df75a5796ef20a081d6969`；导入二进制SHA256 `4cb1ffaba06a6587fcd56352db699ebd64ede59ff8a00dc697a9335d651b65cc`。
- external_effects/event_log/river_job/group_ops_plan_nodes/group_ops_executions始终0；12历史计划保持archived，不恢复旧任务。
- 四个GET已通过真实生产路由builder的Authenticate/AdminRead测试：admin200、匿名401、ops403、缺reader503、不要求CSRF；仅本地验证，不代表生产已部署。
- 历史页为 `/admin/groupops.html?history=1` 和 `/admin/groupopsDetail.html?history=1&id=<V2planID>`，两来源目录按本页分区展示，不伪称全局来源筛选或当前目录。
