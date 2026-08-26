# P4 W2 Sidebar 上线闭环证据

- 范围：`LEGACY-S05-020` 至 `LEGACY-S05-035`，集中 PR #545。
- 口径：最小安全 V2 替代；复用现有 Go OpenAPI、生成客户端和 Sidebar owner/customer scope，不恢复旧 Mock/Seed 回退。
- 本地候选提交：待 Matrix 证据提交后固定最终 SHA。

## 已验证能力

- 鉴权与上下文：JSSDK 配置、OAuth 回退、CSRF、owner context token、workbench 与画像保存。
- 客户工作台：手机号绑定、问卷、时间线、来源跳转、普通/周期订单、周期备注。
- 素材与发送准备：素材搜索、真实缩略图、可分享商品、同源只读商品页、图片临时媒体准备。
- 商品卡片和图片发送仅记录 JSSDK `client_callback`；聊天送达保持 `delivery_unknown`，真实企微效果未执行。
- 手机号绑定只允许当前客户声明手机号，不实现强制接管其他客户绑定。
- 订单详情在 Sidebar 客户范围内展开，不跳转需要后台管理员权限的旧详情页。
- 其他客服聊天只读取当前客户的本地会话存档，排除当前 active owner，仅返回最近 20 条 `text/image` 脱敏内容；身份或归档异常时失败关闭。

## 可重复验证

- 后端：`go test ./...`
- 契约与迁移：`make generate-check orval-check openapi-p1-contract migration-mapping-contract migration-validate migration-guard-negative ownership-lint ownership-lint-test`
- 前端：`npm run typecheck`、`npm run transport:contract`、`npm run admin:adapter:contract`、`npm run capabilities:contract`、`npm run build`、`node scripts/e2e.mjs`
- 本地结果：Go、OpenAPI/Orval、迁移、所有权与前端构建已在分支定向通过；集成后全量计数待本文最后更新。

## 分层状态

- PR #545 最终候选 CI：待最终提交触发。
- 候选 Full Nightly：待最终提交触发；排队不阻塞后续波次开发。
- exact-main Nightly：未执行，只有合并后才能确认。
- Staging、Production、真实 WeCom Provider/JSSDK 外部效果：均未执行。
