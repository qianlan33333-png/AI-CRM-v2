# G1-D02 · A 档调用量 Top-20 页面行为抽查清单

> 状态：**用户已确认 G1-D02 治理签字；线上操作证据未执行**
>
> 决策锚点：`G1-D02-2026-08-10`
>
> 基线：`main@57bb4ca4b4b8e1b46978e6e513f6d9cdf28f3af7`
>
> 流量窗口：`2026-07-11T00:00:00+08:00` 至 `2026-08-10T10:15:00+08:00`

## 取样方法

从 `docs/evidence/p1/route-triage.csv` 过滤：

1. `recommended_tier=A`
2. `ui_referenced=true`
3. 按 `call_count` 数值降序
4. 保留前 20 条 route fact，不合并同一用户动作关联的多个路由

冻结输入：

- `route-triage.csv` SHA-256：`875596da33d316c31bff9a6103725affa58c44be399ec98239d9e294c34c069b`
- `feature-matrix.csv` SHA-256：`624e2f8199ece1dcb8e35b8358f26603dc037ae63c3b5d6f8c932e39d0d41b6b`

`ui_referenced=true` 不等于“都是可直接点击的 HTML 页”。原始 top-20 中包含企微 callback、OAuth 和页面自动加载 API；本清单保留原始排序，并把它们转换成可执行的线上用户动作。第 2、3、9 项涉及真实企微/鉴权边界，本轮只提供核对清单，不代用户执行。

## 抽查清单

核对结果建议填写：`一致 / 不一致 / 无法执行`，不一致时附截图或最短复现步骤。

| # | route fact / 调用量 | 线上实际操作 | 应观察到的旧行为 | feature matrix 证据 | 核对结果 |
|---:|---|---|---|---|---|
| 1 | `LEGACY-API-0739` · 4339<br>`GET /api/sidebar/v2/timeline` | 在企微侧边栏打开一个已绑定客户，进入“时间线”，执行刷新和“加载更多” | 首屏最多 20 条；响应含 `items/total/has_more/next_offset`；仅 `has_more=true` 时显示加载更多；失败显示重试 | `LEGACY-S05-027` | 治理签字通过；线上操作证据未执行 |
| 2 | `LEGACY-API-0751` · 3146<br>`GET,POST /api/wecom/events` | 使用测试企微对该回调地址做 URL 校验，再产生一条测试回调事件 | GET 校验成功；POST 持久化 ingress 后返回加密 ACK；ACK 不等于后续 identity/welcome/tag 已完成 | `LEGACY-S06-043` | 治理签字通过；线上操作证据未执行 |
| 3 | `LEGACY-API-0780` · 1717<br>`GET,POST /wecom/external-contact/callback` | 使用测试企微对该外部联系人回调地址做 URL 校验，再由测试账号触发一条外部联系人事件 | GET/POST 回调验证通过并返回 `encrypted_success_reply`；后续业务处理与 durable ACK 分阶段 | `LEGACY-S06-043` | 治理签字通过；线上操作证据未执行 |
| 4 | `LEGACY-API-0732` · 1700<br>`GET /api/sidebar/v2/other-staff-messages` | 侧边栏打开同一客户的“其他客服聊天” | 展示其他客服的 text/image 消息，最多最近 20 条；owner 尚未确认时不请求并提示 | `LEGACY-S05-035` | 治理签字通过；线上操作证据未执行 |
| 5 | `LEGACY-API-0729` · 1602<br>`GET /api/sidebar/v2/materials/image/{image_id}/thumbnail` | 侧边栏进入图片素材并滚动到出现缩略图 | 缩略图可经历 pending/302/304，最终显示 ready 或 error；429/503 可重试 | `LEGACY-S05-033` | 治理签字通过；线上操作证据未执行 |
| 6 | `LEGACY-API-0714` · 1518<br>`GET /api/sidebar/jssdk-config` | 从企微会话中打开侧边栏工作台 | 返回 JSSDK 配置及可能的 owner context；URL 错误为 400，配置错误为 502，浏览器阶段 5 秒超时 | `LEGACY-S05-020` | 治理签字通过；线上操作证据未执行 |
| 7 | `LEGACY-API-0465` · 1167<br>`POST /api/admin/send-content/preview` | 在渠道欢迎语编辑页或群运营计划节点中编辑文本/图片/小程序/附件，点击预览 | 按素材上限规范化内容包并回填预览；素材选择只在需要时同步群/ensure 邀请 | `LEGACY-S06-009`、`LEGACY-S06-033` | 治理签字通过；线上操作证据未执行 |
| 8 | `LEGACY-API-0779` · 777<br>`GET /sidebar/bind-mobile` | 从企微侧边栏进入手机号绑定工作台 | 页面正常加载并显示当前绑定状态；提交时携带 `external_userid/mobile/force_rebind`，成功更新页头状态 | `LEGACY-S05-025`（写行为） | 治理签字通过；线上操作证据未执行 |
| 9 | `LEGACY-API-0753` · 592<br>`GET /auth/wecom/start` | 在登录入口点击企微 OAuth/扫码登录链接 | 跳转企微授权；回调后建立浏览器会话；缺配置或未启用时 fail-closed 到明确错误/登录回退 | `LEGACY-S05-036`、`LEGACY-S06-043` | 治理签字通过；线上操作证据未执行 |
| 10 | `LEGACY-API-0740` · 510<br>`GET /api/sidebar/v2/workbench` | 打开已绑定客户的侧边栏工作台首页 | 渲染 `customer/profile/workflow/diagnostics`；缺 external_userid 为 400、scope 错误 403、找不到 404 | `LEGACY-S05-023` | 治理签字通过；线上操作证据未执行 |
| 11 | `LEGACY-API-0758` · 444<br>`GET /login` | 未登录状态打开后台登录页 | 显示 QR/OAuth 登录入口；已有有效 session 时 302 到安全 `next` | `LEGACY-S05-036` | 治理签字通过；线上操作证据未执行 |
| 12 | `LEGACY-API-0093` · 402<br>`GET /api/admin/ai-audience/packages` | 打开“自动化转化/人群包”，切换分组并翻页 | 渲染当前分组的人群包、总数和页码；切组重置页码；空态/错误态可见 | `LEGACY-S06-013`、`LEGACY-S06-014` | 治理签字通过；线上操作证据未执行 |
| 13 | `LEGACY-API-0007` · 398<br>`GET /admin/automation-conversion` | 直接进入“自动化转化”主页 | 页面壳正常加载，可进入人群包详情并打开人群包群发确认入口 | `LEGACY-S06-017`、`LEGACY-S06-019` | 治理签字通过；线上操作证据未执行 |
| 14 | `LEGACY-API-0555` · 359<br>`GET /api/admin/wecom/tags` | 打开“企微标签”页；观察首屏目录，可再做本地搜索/分页 | 渲染 groups、总标签数、标签上限和同步时间；失败时清空列表并显示错误 | `LEGACY-S05-010`、`LEGACY-S05-011` | 治理签字通过；线上操作证据未执行 |
| 15 | `LEGACY-API-0079` · 347<br>`GET /admin/wechat-pay/products` | 打开“微信支付商品”列表 | 服务端最多读取 100 条并渲染 list 模式；列表可继续做刷新、上/下架、复制、删除与分享 | `LEGACY-S07-036`、`LEGACY-S07-164`～`170` | 治理签字通过；线上操作证据未执行 |
| 16 | `LEGACY-API-0058` · 337<br>`GET /admin/orders` | 打开“统一订单”，执行一次筛选、刷新或翻页 | 页面以统一订单上下文渲染；客户端按 API 结果重绘订单表和分页 | `LEGACY-S07-022`、`LEGACY-S07-174` | 治理签字通过；线上操作证据未执行 |
| 17 | `LEGACY-API-0061` · 320<br>`GET /admin/questionnaires` | 打开“问卷管理”，刷新列表并打开一个问卷 | 主入口渲染问卷列表与 preflight；旧 `/ui` 别名 302；可进入编辑、复制、启停或删除流程 | `LEGACY-S07-024`、`LEGACY-S07-116`～`124` | 治理签字通过；线上操作证据未执行 |
| 18 | `LEGACY-API-0013` · 319<br>`GET /admin/channels` | 打开“渠道”列表，做关键词过滤并打开一个渠道的近期用户抽屉 | 加载最多 300 条渠道及统计；关键词仅过滤已加载行；抽屉展示最近客户和进入次数 | `LEGACY-S06-001`、`LEGACY-S06-002` | 治理签字通过；线上操作证据未执行 |
| 19 | `LEGACY-API-0010` · 299<br>`GET /admin/automation-conversion/group-ops/ui` | 进入“群运营计划”页 | 渲染计划列表、队列计数、运营成员和创建入口；队列计数不代表已发送数量 | `LEGACY-S06-028` | 治理签字通过；线上操作证据未执行 |
| 20 | `LEGACY-API-0048` · 297<br>`GET /admin/customers` | 打开“客户”，执行关键词/负责人/手机号/标签筛选，清空，再前后翻页并打开一名客户 | 服务端每页最多 50 条并规范化非负 offset；详情存在时渲染，缺失时显示 placeholder/404 | `LEGACY-S05-001`～`008` | 治理签字通过；线上操作证据未执行 |

## 固化结论

- 用户已确认本清单并批准 G1-D02 规则签字；293 条 feature matrix 全部转为 `MIGRATE/APPROVED`。
- 本次没有伪造 20 项的线上操作、浏览器或生产验证结果；所有行仍为 `implementation=NOT_STARTED`、`verification=NOT_RUN`。
- P5 人工全功能抽验仍是新旧一致性的唯一最终防线，投入不得压缩。
