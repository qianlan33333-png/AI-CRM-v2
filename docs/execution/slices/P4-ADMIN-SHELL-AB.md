# P4-ADMIN-SHELL-AB：后台壳与退出兼容入口

## 冻结范围与业务结果

- fresh replay Base 为实时 exact-green `ebaeaf280d84a0917ea6c5b5df5678beeb10149b`。本片只交付
  `LEGACY-API-0001 GET /admin` 与 `LEGACY-API-0053 GET /admin/logout`；A、B 路由作为同一个
  后台壳业务包，不拆分、不重复计数。
- 完整可观察行为必须同时收口 transport、Session/Actor/capability、OpenAPI、mapping、matrix、验收 manifest
  和受影响的仓库指纹守卫，共 14 个手写文件、433 行变更（测试与生成物除外）。这超过 P4 常规 12 文件
  预算但低于 15 文件/1500 行硬顶；为保持两条路由一个可验收业务包而不伪拆为半成品。
- 用户已裁定：后台管理员和运营人员可使用该入口；销售、未知、缺失或歧义身份一律拒绝。`/admin`
  仅提供既有后台的静态快捷入口，不创建新 UI，也不承诺下游链接已部署。
- `/admin/logout` 只在通过人类 Session、Actor 与 `admin.shell.read` global capability 后，以 302
  第一跳到既有 `/logout`。它不自行注销、清 Cookie 或创造 redirect；既有 `/logout` 仍是 Session
  失效、CSRF 校验和最终 redirect 的唯一 owner。

## 安全、数据与外部边界

- bearer principal、sales、非 global grant、缺失或未知角色返回 fail-closed 拒绝；无效、过期、畸形或
  OneID 歧义的浏览器 Session 只安全地回到本地登录路径。拒绝响应固定声明未发生真实外效。
- 无 schema、migration、migration mapping、水位、tenant、跨域读写、River、worker、provider 或支付变更；
  `P4ADMINSHELL_TEST_DATABASE_URL` 只指向隔离 PG16.14 `aicrm_test`，用于验证既有退出 owner 链。
- `PRODUCTION_DATABASE_NOT_EXECUTED`、`LIVE_MIGRATION_NOT_EXECUTED`、`REAL_EXTERNAL_EFFECT_NOT_EXECUTED`、
  `EDGE_OR_STAGING_DEPLOYMENT_NOT_EXECUTED`。本片的集成授权不包含生产切换。

## 本地验证收据

- `P4ADMINSHELL_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:55431/aicrm_test?sslmode=disable make p4-admin-shell-ab-acceptance`
  通过：当前 migration waterline 为 44；真实 OAuth 人类 Session 后 `/admin` 成功、`/admin/logout`
  仅 302 `/logout`，既有 `/logout` 的 CSRF 与 Session 失效链继续由同一测试覆盖。
- 通过 focused normal/boundary/error、race、vet：`./cmd/aicrm`、`./internal/auth/app`、
  `./internal/auth/port`、`tools/p1-reconciliation`、`tools/openapi-contract`，以及根目录
  `./acceptance/p1s11`。
- `feature-matrix-contract`、`p1-reconciliation`、`openapi-contract` 与 `generate-check` 已在锁 tree 前通过；
  API mapping 的 A/B 分母仍为 `501/268/12`，其中已承接 migrate 为 `502`、保留 historical deferred 为 `267`、
  稳定退役为 `12`。本收据不声明 MERGED、EDGE、STAGING、RELEASE 或 DEPLOYED。
- `verification_induced=2`：首个正式 PR 的 Web gate 发现 Orval 生成物漏同步。以 `make generate-orval`
  重建唯一生成文件后，`npm run ci` 通过 Orval contract、lint、strict typecheck、226 个 Web 测试、生产 build
  和 high-severity audit；随后发现 Candidate Merge Guard 对所有正式业务 PR 无条件要求 migration，现改为
  仅在 matrix 和 slice 同时声明 `no_schema_or_external_effect` 时允许无 migration，并以正反例覆盖。两者均未改变产品合同或扩大业务范围。

## exact-main closure addendum（2026-08-18）

- 正式 PR #236 已 MERGED/CLOSED：head `de4fa248749600dc1efc59be08f5e63912c5fa2f`，merge
  `1fd449995c2d2ee5d9954b5d10d259b6bcce86bd`，两者 tree 均为
  `1c5e3db4c0b792838a41c5a6c80f1533eb64c2a8`（match-head squash）；merge 是当前 exact main
  `53276849b11ca9b37d10673164fb2f95d3587dd5` 的祖先，历史 Required 链全绿。
- 2026-08-18 在 exact main 复验：`GET /admin` 服务端模板仍渲染快捷入口及旧链接，`GET /admin/logout`
  仍只 302 到既有 `/logout` 且不自行注销或清 Cookie；human session + `admin.shell.read` global 与
  fail-closed 拒绝链保持；`P4ADMINSHELL_TEST_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:55431/aicrm_test?sslmode=disable
  make p4-admin-shell-ab-acceptance` 退出码 0（隔离 PG16.14 `aicrm_test`，migration waterline 45）。
- `LEGACY-S07-001` 由 `IN_PROGRESS/NOT_RUN` 更新为 `IMPLEMENTED/SYNTHETIC_PASS`；仅代表仓库代码与
  synthetic/local 隔离 PG 验证闭环。`PRODUCTION_DATABASE_NOT_EXECUTED`、
  `LIVE_MIGRATION_NOT_EXECUTED`、`REAL_EXTERNAL_EFFECT_NOT_EXECUTED`、
  `EDGE_OR_STAGING_DEPLOYMENT_NOT_EXECUTED`。
