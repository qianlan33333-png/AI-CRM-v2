# 仓库公开切换与 Actions 恢复证据

- Issue: #63
- Evidence date: `2026-08-09`（Asia/Shanghai）
- Repository: `qianlan33333-png/AI-CRM-v2`
- Evidence level: `LOCAL` + GitHub Actions

## 状态变化

GitHub Repository API 已确认 `visibility=public`、`private=false`、默认分支为
`main`。本次变化只涉及源码仓库可见性，不改变“单企业私有化部署”的产品拓扑，
也不授权部署、真实迁移、真实企微调用或生产操作。

公开切换后，Ruleset API 返回空列表，`main` protection API 返回 HTTP 404。
因此仓库公开不等于分支已受保护；当前仍执行“分支 → 中文 PR → CI 全绿 →
squash merge → 精确 main SHA CI”的操作纪律。

## PR #62 恢复与合并

- PR: <https://github.com/qianlan33333-png/AI-CRM-v2/pull/62>
- Base: `cd7646560475aedefa897c2dfeca30744ea9396f`
- Head: `21833ebd7974e27938bd1dfbc6e387670ab1c413`
- Squash merge/main: `81b4406ad777d1f3a08ca770bb341029a5f83707`

原失败 run 在公开切换后直接 rerun，没有用新提交掩盖外部门：

- application: run `31301337127`，Go 与 Web 均 `SUCCESS`
- repo-contract: run `31301337126`，`SUCCESS`
- secret-scan: run `31301337158`，`SUCCESS`

合并后的精确 main SHA 再次产生并通过：

- application: run `31302155884`，Go/Web 与 PostgreSQL 16.14 门 `SUCCESS`
- repo-contract: run `31302155887`，`SUCCESS`
- secret-scan: run `31302155924`，`SUCCESS`

## 未执行

- 未创建或修改 Ruleset/branch protection。
- 未直接 push `main`，未绕过 PR/CI。
- 未部署，未迁移数据库，未访问真实用户数据或外部生产系统。
