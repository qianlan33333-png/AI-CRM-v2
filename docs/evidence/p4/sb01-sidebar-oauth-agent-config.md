# SB01 Sidebar OAuth 与 AgentConfig：V2 后端能力收据

SB01 为 Sidebar 增加独立的企微 OAuth 浏览器协议，以及仅用于 `agent_config` 的 JSSDK
签名读取。它不复用管理员 `/auth/wecom/callback`：启用时必须同时配置独立 CorpID、Secret、
callback URL、AgentID 与 HTTPS host allowlist；缺任一项启动即失败，未配置时路由 fail closed
且不会调用 Provider。

OAuth start 将一次性 state、corp、目标 external contact 与过期时间绑定到加密 HttpOnly/Secure
cookie。callback 先 claim state，再交换 Provider code；随后只接受同 corp 的企微员工身份，并复用
既有 browser session、RBAC、canonical customer/owner 检查。任一后续校验失败会撤销新 session；
浏览器 session cookie 写入失败也会撤销。该流程不创建或绑定 Identity，不猜测 OneID。

`GET /api/sidebar/v2/jssdk/agent-config` 只接受精确 HTTPS allowlist URL，fragment 不参与签名；
它使用独立的 `agent_config` ticket provider/cache/singleflight，响应固定
`signature_type=agent_config` 且绝不返回 ticket。Corp config ticket/signature、消息发送、前端初始化、
真实企微调用和 Provider 成功 receipt 都不属于本包。

本收据只证明 SB01 后端合同、OpenAPI/generated 与本地测试。required CI、main 合并、Batch 1
exact-main Nightly、部署、环境配置及用户授权后的真实企微效果必须分层报告。SB01 复用既有
OAuth state/session 与 Sidebar 本地模型，没有新增表或字段，因此没有 migration/SQLC 变更。
