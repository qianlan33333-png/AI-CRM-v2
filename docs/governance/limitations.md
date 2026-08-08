# Repository governance limitations

## 私有仓库 Ruleset

2026-08-08 使用当前账号调用私有仓库 Ruleset API，GitHub 返回 HTTP 403：
需要升级 GitHub Pro 或将仓库公开。用户已选择私有仓库，因此当前 `main`
没有 GitHub 平台强制的分支保护。

这意味着：

- GitHub 技术上仍可能接受直接 push；不能声称 `main` 已受保护。
- 单一账号也无法形成独立的 required approval。
- CODEOWNERS 当前只表示责任归属，不能构成强制审批证据。

## 操作性补偿控制

- `P0-B0` 后所有变更一律分支、中文 PR、CI 通过、Codex squash merge。
- 每次合并前保存 head/base SHA、检查结果和 PR URL；合并后验证远端 main。
- Actions 权限只读，禁止 `pull_request_target`、部署、Environment 和 Secret。
- CI 检查名称始终产生结果，不用 path filter 隐藏失败。
- 若 GitHub 套餐以后支持 Ruleset，立即启用：禁止 delete/force-push、要求
  PR/线性历史/会话解决/分支最新、无 bypass；单账号期间 approvals 为 0。

这些补偿控制提供可审计性，但不能等价为平台级不可绕过保护。
