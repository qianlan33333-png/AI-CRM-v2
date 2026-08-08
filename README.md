# AI-CRM v2

AI-CRM v2 是面向单企业私有化部署的全新重构仓库。当前阶段为
`P0-B0`：仓库治理、架构决策和证据基线已经建立，但应用尚不可运行、
不可部署，也未通过 P5/P6 验收。

## 当前状态

- 绿色仓库基线，不继承旧仓库 Git 历史。
- 不导入此前 ChatGPT Pro 的整包源码或伪生成物。
- 目标架构为 Go 模块化单体、PostgreSQL 16、River 与 React 18。
- 本仓库目前没有部署工作流、环境 Secret 或生产配置。

## 权威顺序

1. 用户最新明确指令；
2. 已决 ADR 与 `docs/architecture/canonical.md`；
3. `docs/spec/AI-CRM-v2-重构详细设计.md`；
4. `docs/spec/AI-CRM-v2-执行方案.md`；
5. 单个 Slice 任务卡。

ADR 只用于解决规格中的冲突，不能静默删减验收标准。产品行为、迁移
取舍和人工验收项在获得明确证据或签字前必须保持未决状态。

## 入口

- 架构总纲：`docs/architecture/canonical.md`
- ADR：`docs/adr/`
- 执行台账：`docs/execution/slice-ledger.yml`
- 验收证据：`docs/evidence/`
- 仓库限制：`docs/governance/limitations.md`

## 安全边界

不要提交 `.env`、Token、Cookie、私钥、数据库、真实用户数据、浏览器
状态或生产配置。任何真实企微调用、服务器操作、数据迁移和部署都需要
独立授权；本仓库中的本地或 synthetic 测试不能被描述为生产验证。
