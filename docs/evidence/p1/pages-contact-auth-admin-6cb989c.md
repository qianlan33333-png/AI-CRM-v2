# P1-S05 contact/auth/admin 前端行为静态盘点

- Issue: #71
- Legacy source: `origin/main@6cb989c071255437d75953dabb943318a74eb8f4`
- API fact input: `docs/evidence/p1/api-facts-contact-auth-admin-6cb989c.md`
- Route input: `docs/evidence/p1/legacy-routes-6cb989c.json`
- Route input SHA-256: `fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231`
- Evidence level: `LOCAL` / static source only
- Disposition: `UNREVIEWED`
- Signoff: `PENDING_HUMAN_SIGNOFF`

## 结果

本 Slice 首次创建 `docs/feature-matrix.csv`，共 66 个候选行为：

| 类别 | 行数 | 范围 |
|---|---:|---|
| UI/page flow | 61 | 客户列表/详情/360、标签、侧边栏、登录、配置中心、API client、发布与向导 |
| API-only/retired flow | 5 | OAuth token、admin jobs、Feishu job、订单身份修复、旧配置 API |
| 合计 | 66 | `LEGACY-S05-001` 至 `LEGACY-S05-066` |

所有行的四个状态分别为 `UNREVIEWED`、`NOT_STARTED`、`NOT_RUN`、
`PENDING_HUMAN_SIGNOFF`。决策、实现、验证与目标行证据保持为空，不把旧源码事实提前写成
`MIGRATE/MERGED/DEPRECATED/IMPLEMENTED/PASS`。

## 调查边界

Terra task `/root/p1_s02_contact_auth_admin` 使用 `gpt-5.6-terra` / `ultra` 完成分组只读
调查；Codex Sol 对 legacy SHA、66 行计数、状态枚举与高风险事实做独立复核。

调查读取 13 个已冻结 API handler、24 个直接页面模板、16 个直接引用的 JS/CSS 和
ADR-010。每行的 `source_evidence` 给出相对旧仓根目录的精确文件与行号；没有读取旧仓的
repository/store/secret 实现，没有导入 FastAPI app、启动服务、访问数据库/网络/凭据、
调用 provider 或部署。静态证据不证明部署版本、运行配置、数据或线上可达性。

## 必须保留的行为边界

- 客户列表、timeline 与素材分页仍是 OFFSET 行为；后续 v2 的 keyset cursor 是显式可见差异。
- 客户页面存在 HTML placeholder、区块级失败和 fixture/degraded 空集合；不能把 200 或空集合
  解释为真实成功或真实无数据。
- 身份冲突、owner/customer/corp scope、viewer session 与 sidebar owner token 必须 fail closed。
- sidebar 商品/素材会在 REST 读取后调用企微 JSSDK；REST 成功不等于消息送达。
- 标签同步和写操作传播幂等/actor/dry-run，但 queued/blocked 不等于企微执行成功。
- API Key 与 client Secret 只在创建或轮换时一次性显示；不得进入矩阵证据、日志或后续页面。
- OAuth callback 与 Feishu job 存在受配置门控制的外部调用路径；本 Slice 未运行这些路径。
- 固定 404/410 的旧配置与订单修复接口仍只属于源码事实，不能自动标为 `DEPRECATED`。

## G1 待签字

1. 66 行是否完整覆盖线上实际页面；页面入口、隐藏按钮和角色差异需真实浏览器逐页核对。
2. 外部 ID URL 到 OneID 的可见行为变化，以及 OFFSET 到 keyset 的交互变化。
3. sidebar OAuth/JSSDK、标签 live adapter、素材/商品发送的真实外部效果与 receipt。
4. API Key/client Secret、配置发布、push capability、管理员访问的 RBAC 与 action-token 规则。
5. API-only jobs 和固定 404/410 compatibility 的迁移、合并或废弃决定。

在 G1 之前，本 Slice 仅提供可审阅候选清单；浏览器、staging、production 与外部 provider
验证均为 `PENDING_EXTERNAL_GATE`。
