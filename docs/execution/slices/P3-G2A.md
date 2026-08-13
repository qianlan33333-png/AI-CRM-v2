# P3-G2A：旧 UI 启动兼容与 Contact 真实读取

## 输入与完整可观察行为

- Base SHA：`21da77109db216acc36c7f5eadffa984e7753736`；application/go、repo-contract、
  secret-scan 三项 required CI 均为绿色。
- 旧静态资产保持零修改，以旧 cookie 名称和路径完成：已存在 v2 browser session 的确认、
  `aicrm_next_csrf` cookie 镜像、`GET /api/admin/config/overview` / `capabilities`
  的当前 permission read，以及 `GET /api/customers` 调用现有 Contact list service。
- Legacy Compatibility API 只做 cookie/session 解析、CSRF/RBAC/owner-scope middleware、
  DTO 与 error mapping；不得写业务表、跨域 SQL、调用 provider 或返回 fixture/placeholder。

## 兼容与安全边界

- 旧 session cookie `aicrm_next_admin_session` 与 v2 `aicrm_session` 均被作为 opaque
  session ref 交给现有 auth service；过期、不可用或无 cookie 均稳定 fail-closed。
- 只在存在有效格式的 v2 CSRF cookie 时镜像为旧 UI 读取的 `aicrm_next_csrf`；后续写路由
  必须经同一 v2 session-bound CSRF validator，缺失、重复或无效 header 均拒绝。
- 当前 capability read 直接调用现有 closed auth policy，不能伪造 persisted config；Contact
  list 直接调用现有 `CustomerListService`，并保留 v2 OneID、keyset 和 owner scope。
- 旧 `external_userid`、mobile、tag/status/binding filter 与 nonzero OFFSET 无法安全映射到
  现有 v2 Contact contract，本片对其返回稳定 400，而不重建身份桥接、offset pagination 或
  任意 SQL；最小启动 flow 仅支持 zero-offset 的 Contact list read。

## 排除与验收

- 不新增 migration、OpenAPI、public port、API client/key 存储、P4 平台接口、新 UI、
  Identity 语义、provider、生产数据库、live migration、真实企微或真实外发。
- 黑盒覆盖真实 v2 session/capability/Contact service 链、CSRF cookie 获取、过期 session、
  缺 CSRF、RBAC/owner mismatch 与安全不可映射 query 的 fail-closed 响应。
- 手写范围为 `cmd/aicrm/api.go` 与 composition-root `cmd/aicrm/legacy_api.go` 两个运行时文件；测试、
  slice card、ledger 和 receipt 不计入 P3 的 12 文件/1000 行预算。
- `verification_induced=4`：首轮测试把平台的稳定平铺 error envelope 误按嵌套 envelope
  解码；只修正测试断言，不改变运行时业务或兼容语义。第二阶段记录计数时曾按通用字段匹配到
  历史 P2-01，第三阶段又匹配到历史 M0-7；均已立即还原不可变历史条目，并按 P3-G2A
  上下文更新本片计数；adapter 测试并入既有 main package 后又暴露测试 helper 同名，
  仅重命名该 fixture helper；均不改变运行时语义。
- `slice_induced=2`：第二阶段自审发现零值 adapter handler 在直接调用时可能解引用未注入的
  Contact application；补齐 dependency-unavailable fail-closed guard，不改变已冻结 route 或 DTO。
  远端架构导入门禁与最终 router 自审共同定位到同一根因：adapter 被错误建成独立业务域，
  同时绕过 composition root 的 account-budget 上下文绑定与 buffered-response header 边界。
  将其整体收回既有 `cmd/aicrm` composition root、绑定现有 account ID 合同，并在 endpoint
  内写入 CSRF mirror，未放宽 checker 或跨域导入规则。达到阈值后立即冻结范围：仅完成本卡
  既定闭环，后续片降档。

## 回滚

如需撤销，revert 本 PR；不执行数据库回滚、session 数据写入、真实企微调用或任何 provider
调用。
