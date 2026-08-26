# P4 AI Audience 操作成员同步后端证据

## 收口范围

- `GET /api/admin/common/operation-members?scope=ai_audience` 只读取 Audience 自有的最后一次成功投影，不在读请求中调用企微。
- `POST /api/admin/common/operation-members/sync` 支持 `scope=ai_audience`，通过既有企微通讯录只读边界获取最多 100 名可操作成员。
- Provider 成功后，在同一事务内全量替换 Audience 投影、完成 `operation_members_sync` 幂等回执并写入脱敏事件。
- Provider、数据库或事件写入失败时不覆盖上一次成功投影；空 Provider 结果会明确提交为空投影。
- `page_size` 只裁剪响应，不缩小 Provider 快照，也不会把局部分页误当成删除依据。

## 上线边界

- 投影与回执属于 AI Audience，不写 Group Ops runtime、receipt 或发送事实。
- 响应使用 `provider_read_executed` 区分 GET 与成功同步；`real_external_call_executed=false` 表示没有外发业务调用、Provider 接受或送达事实。
- 同步需要全局 `operations.manage`、CSRF 与 `Idempotency-Key`；目录同步未配置时 fail closed。
- 本包不包含前端 picker 接线，不证明 sender 的企微发送授权，也不证明部署或真实生产 Provider 效果。

## 数据与生成物

- migration `00100_ai_audience_operation_member_projection.sql` 创建 Audience 自有投影并扩展既有本地配置回执 operation，包含 populated down guard。
- 新增 SQLC 查询完成投影锁定、全量替换与稳定排序读取；不新增 direct SQL baseline。
- OpenAPI 将共享同步请求扩展为 `ai_audience | group_ops`，响应按 scope 返回对应投影；Orval 生成物由规范重新生成。

## 验证

- focused/race：`./internal/segment/legacyaudience ./internal/segment/store ./internal/wecom/groupopsdirectory ./cmd/aicrm`
- PG16.14：`p4-ai-audience-operation-member-acceptance`，覆盖 Provider stub、SQLC 投影、回执/事件 replay 以及 projection/receipt down guard。
- 契约：`make generate-check orval-check openapi-p1-contract migration-validate source-policy-lint feature-matrix-contract`

## P4 口径

本包完成 S06-024 所需的后端同步与持久化能力。前端阶段尚未开始，因此不把页面 picker、按钮交互、部署或生产企微读取混称为已上线。
