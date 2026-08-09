# Repository governance limitations

## 仓库可见性与 Ruleset

2026-08-08 使用当前账号调用私有仓库 Ruleset API，GitHub 返回 HTTP 403：
需要升级 GitHub Pro 或将仓库公开。该结果是 P0 建仓时的历史事实。

2026-08-09 用户已把仓库切换为 public；此前受私有仓库 Actions 计费/额度影响的
检查经原 run 重跑后恢复，PR #62 及其精确 main SHA 检查均通过。公开可见性没有
自动创建治理规则：Ruleset API 当前返回空列表，`main` protection API 返回 404，
所以 `main` 仍没有 GitHub 平台强制的分支保护。

这意味着：

- GitHub 技术上仍可能接受直接 push；不能声称 `main` 已受保护。
- 单一账号也无法形成独立的 required approval。
- CODEOWNERS 当前只表示责任归属，不能构成强制审批证据。

## 操作性补偿控制

- `P0-B0` 后所有变更一律分支、中文 PR、CI 通过、Codex squash merge。
- 每次合并前保存 head/base SHA、检查结果和 PR URL；合并后验证远端 main。
- Actions 权限只读，禁止 `pull_request_target`、部署、Environment 和 Secret。
- CI 检查名称始终产生结果，不用 path filter 隐藏失败。
- 若后续获得明确授权配置 Ruleset，启用前先在独立治理 Slice 冻结 required checks，
  再禁止 delete/force-push、要求 PR/线性历史/会话解决/分支最新、无 bypass；
  单账号期间 approvals 为 0。

这些补偿控制提供可审计性，但不能等价为平台级不可绕过保护。
