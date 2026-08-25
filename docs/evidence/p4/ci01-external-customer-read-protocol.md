# CI01 外部客户读取协议：P4 后端能力收据

CI01 恢复 8 条外部客户/身份读取协议：

- `GET /api/customers`
- `GET /api/customers/{external_userid}`
- `GET /api/customers/{external_userid}/timeline`
- `GET /api/users/{unionid}`
- `GET /api/users/{unionid}/messages/recent`
- `GET /api/users/{unionid}/timeline`
- `GET /api/messages/{external_userid}/recent`
- `GET /api/identity/resolve`

前 7 条复用 human session 与 `customers.read`；admin/ops 为 global，sales 绑定本人
owner staff。`/api/identity/resolve` 使用独立 API-client JWT：启动时注入 32-byte
base64url HS256 secret，并要求 `admin_ops_credentials` 中同 client、同 version 的 active
credential 及精确 metadata policy：`purpose=identity`、`audience=identity`、
`capability=identity.resolve`。该协议不接受 browser cookie fallback；未配置 secret 时
fail closed。

external_userid 必须经 corp-scoped verified Identity 解析；unionid 查询只接受
`assurance=verified`，not-found/conflict 均保持闭合。客户 projection 不返回 raw identity；
timeline 不返回 payload/actor；消息只返回 chat type、message type、send time，不返回正文、
参与者或 Provider message id。所有成功响应声明 `fallback_used=false`、
`real_external_call_executed=false`。

本包无 migration。SQLC 查询增加 verified assurance 约束；OpenAPI 与合同检查登记
`LEGACY-API-0609/0619/0620/0680/0690/0743/0744/0745`。focused/race 测试覆盖
JWT 签名、时间、credential 状态/版本/policy、owner scope、PII 边界与错误映射；
`make p4-ci01-external-read-acceptance` 在隔离 PG16 上证明 declared unionid 不会进入读取结果。

该收据只证明本地后端协议与数据边界，不证明前端接入、批次 exact-main Nightly、部署或
Provider/企微真实外部效果；这些层级仍为 `NOT_EXECUTED`。
