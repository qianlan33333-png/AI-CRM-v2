# Security policy

## 支持范围

当前仓库处于重构阶段，尚未发布可部署版本。安全问题仍按最高优先级处理，
但任何本地修复或 CI 通过都不代表生产系统已修复。

## 报告方式

不要在普通 Issue、PR 或聊天中粘贴密钥、Token、Cookie、用户数据、数据库
内容或未脱敏请求。使用仓库的私有 Security Advisory；若无法使用，先仅
报告受影响的文件和问题类别，不附敏感值。

## 凭据和数据

- 禁止提交 `.env*`、私钥、API key、Cookie、浏览器状态、数据库 dump、
  真实用户数据或生产配置。
- 普通设置进入强类型 config/settings；Secret 仅通过受控 env/file 注入。
- Extension API key 采用高熵随机 secret，明文只展示一次，持久化只存哈希。
- 日志、错误体和审计必须脱敏，不能记录原始手机号、unionid 或凭据。

## 供应链

- GitHub Actions 必须固定到完整 commit SHA，工作流权限默认只读。
- 禁止 `pull_request_target` 执行 PR 代码。
- 生成器和依赖版本必须锁定；生成物、`go.sum` 和 `package-lock.json` 接受
  独立复核。
- 高危漏洞不得用无期限 allowlist 绕过；任何例外必须有负责人、理由和
  失效日期。

## 外部影响

企微写操作只能经 outbound；真实外部调用、迁移和部署需要独立授权。
测试必须明确标注 LOCAL/SYNTHETIC/STAGING/PRODUCTION 证据层级。
