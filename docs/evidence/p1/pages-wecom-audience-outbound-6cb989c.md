# P1-S06 企微、人群与外发前端行为静态盘点

- Issue: #73
- Legacy source: `origin/main@6cb989c071255437d75953dabb943318a74eb8f4`
- API fact input: `docs/evidence/p1/api-facts-wecom-segment-outbound-6cb989c.md`
- Route input: `docs/evidence/p1/legacy-routes-6cb989c.json`
- Route input SHA-256: `fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231`
- Evidence level: `LOCAL` / static source only
- Disposition: `UNREVIEWED`
- Signoff: `PENDING_HUMAN_SIGNOFF`

## 结果

在 P1-S05 的 66 行之后追加 44 个候选行为：

| 范围 | 候选 ID | 主要页面/API 边界 |
|---|---|---|
| 渠道中心 | S06-001～012 | 列表、表单、二维码、获客链接、欢迎语、客服分配 |
| AI Audience | S06-013～027 | 人群包、分组、模板、绑定、成员、发送记录、webhook |
| Group Ops | S06-028～035 | 计划、群资产、节点、webhook、run-due/broadcast |
| User Ops | S06-036～042 | 运营池、详情、免打扰、预览、审核计划、发送记录 |
| 外部入口/效果 | S06-043～044 | callback/OAuth、external-effect 与 Push Center |

P1-S03 的旧 API 分区仍为 184 条；本前端调查定位 11 条 admin-page 路由、6 个可视页面组、
57 条直接页面关联 API 与 116 条未关联 API。`LEGACY-S06-008`、`009`、`010`、`019`、
`023`、`024`、`028`～`030`、`033`、`037`、`040` 是显式跨 Slice 前端依赖，不因此
重分区 API。

## 调查边界

Terra task `/root/p1_s06_matrix_reconstruct` 使用 `gpt-5.6-terra` / `ultra` 完成大型只读
调查；Codex Sol 负责候选落地、源码复核、测试和 GitHub 交付。逐行 `source_evidence`
只引用旧仓明确 handler、页面模板与直接静态资源。

调查开始时曾以一次 `find` 仅枚举文件名，结果包含范围外模板/静态路径与
`group_ops/repo.py` 文件名；没有读取该文件内容，所有最终结论均重新取自逐行列出的允许
文件。未执行广域内容搜索、旧 app import、数据库/网络/凭据访问、provider 调用或部署。

## 必须保留的行为边界

- callback 验签、耐久 inbox 与 encrypted ACK 只证明入口接收；welcome、tag、identity 与
  external effect 属于后续阶段。
- 渠道二维码/获客链接保存、生成或下载不证明企微侧可用、真实扫码或归因成功。
- Audience 定义、成员、发送人和 automation binding 都是旧读/写模型，不得把
  `external_userid` 直接迁成 OneID 或 verified identity。
- User Ops execute 的强语义只到 `pending_review/draft`；旧事实明确 broadcast/effect/sent
  为零，不能写成已经群发。
- run-due、webhook、queue command 的 `202/queued/accepted` 不证明 provider 调用；即便状态为
  `provider_accepted` 也只证明 provider 接受，不等于 receipt 或送达。`unknown_after_dispatch`
  必须进入独立 receipt/reconciliation。
- 外部 webhook URL、OAuth、企微 JSSDK 与 QR adapter 均受配置和安全门控制，本 Slice
  未执行任何真实外部链路。

## G1 待签字

1. 六个页面组与 44 行是否覆盖线上真实入口、隐藏按钮、角色和移动端行为。
2. 渠道载体、客服分配、欢迎语素材、标签依赖与真实企微 receipt。
3. 旧 Audience SQL/成员到 v2 DSL/OneID 的转换和成员 diff。
4. Group Ops webhook、run-due、token broadcast 的签名、幂等、SSRF、限流和送达语义。
5. User Ops 的 preview、免打扰、sender、审核计划与 outbound 唯一写出口。
6. `unknown_after_dispatch`、provider receipt 与人工 reconciliation/重试权限。

浏览器、staging、production 与 provider 验证均为 `PENDING_EXTERNAL_GATE`；本 Slice 不是
G1 签字结果。
