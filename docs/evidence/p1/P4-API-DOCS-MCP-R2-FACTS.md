# P4 API Docs / MCP compatibility R2 facts — FACTS_READY_FOR_IMPLEMENTATION

> 本文是 2026-08-17 `P4-API-DOCS-MCP-FACTS`（BLOCKED）的窄 R2 决策附录，不是重新研究。
> base：`d81ba86bde4752fe6d3e402f7bf5bebf007db600`，tree：`54397c51b4faebbc8de806b9b3dbe050dcae2490`。
> base override 决策 ID：`P4-API-DOCS-0003-0033-R2-BASE-OVERRIDE-2026-08-18`（取代“必须等待 LEGACY-API-0757 merge 后再开工”的旧顺序要求；0757 改为 R2 合并后从新的 exact-main 全新重切）。
> prior facts：`docs/evidence/p1/P4-API-DOCS-MCP-FACTS.md`（2026-08-17 上传包内副本 SHA-256 `d52a5380d2b4e328fbe8e289e35373046f2576d847c669b10e4593023fac0512`）。
> immutable legacy snapshot：`6cb989c071255437d75953dabb943318a74eb8f4`（实现代理不得读取旧 Python 源码；本附录与 prior facts 是旧行为的唯一传递通道）。

## 1. Verdict

`FACTS_READY_FOR_IMPLEMENTATION`，范围严格为：

- `LEGACY-API-0003` `GET /admin/api-docs` — IMPLEMENTATION_ELIGIBLE。
- `LEGACY-API-0033` `GET /admin/config/mcp-tools` — IMPLEMENTATION_ELIGIBLE。
- `LEGACY-API-0034` `POST /admin/config/mcp-tools/save` — `DEFERRED_POST_LAUNCH`，NOT_IMPLEMENTATION_ELIGIBLE，不得注册、不得改 disposition、不得计入进度分子。

唯一等式不变：`3 = 2 × MIGRATE + 1 × DEFERRED_POST_LAUNCH`，三条 route ID 逐条销账。

## 2. B1–B5 的逐项关闭方式

| 旧 blocker | R2 关闭裁决 |
|---|---|
| B1 legacy auth/error 装配不完整 | 不复刻 FastAPI 全局默认；使用当前 v2 canonical session 与 401 `UNAUTHENTICATED` / 403 `UNAUTHORIZED` / 503 `DEPENDENCY_UNAVAILABLE` / 500 `INTERNAL_ERROR` envelope，响应只含 `code`/`message`/`request_id`/可选 `details` |
| B2 legacy base template/header 不完整 | 使用 R2 冻结合同：`200`、`text/html; charset=utf-8`、`Cache-Control: no-store`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: no-referrer`、CSP 至少 `connect-src 'none'`、`form-action 'none'`、`base-uri 'none'` |
| B3 legacy runtime route registry 未提供 | 不使用运行时 router；数据源固定为编译期嵌入的 canonical `api/openapi.yaml`，启动时解析一次生成不可变脱敏 view model |
| B4 runtime registry 与 OpenAPI 唯一真值冲突 | OpenAPI 唯一真值优先；显式字段 allowlist + fail-closed；不展示 example/examples、schema default、server URL、函数名、源文件行号、raw vendor extensions、凭据值、合成请求/响应/curl 及旧误导文案 |
| B5 owner/capability 未定 | `cmd/aicrm` composition-root compatibility adapter；现有 `config.overview.read`；global scope；admin only；不新增 capability，不修改 `internal/auth/**` |
| B6 0034 未达重新激活条件 | 继续 `DEFERRED_POST_LAUNCH`；只阻塞 0034，不阻塞 0003/0033；其延期状态由静态测试保护 |

## 3. 不可变可见合同摘要（与指挥 R2 裁决一致）

### 3.1 0003 页面

- principal：后台 human session；capability `config.overview.read`；global scope；admin only；GET 无 CSRF。
- shell：title `API 文档`；summary `查看 AI-CRM 后台和外部集成 API 文档。`；active endpoint `api.admin_api_docs`；breadcrumb `客户管理后台 → API 文档`。
- 任意 query 不改变服务端结果；query 不作为搜索词、不回显。
- 数据源：编译期嵌入 `api/openapi.yaml`；只纳入其中已存在的 operation；沿用旧页面冻结的一级 path allow-pattern（`/health`、`/mcp`、`/api/`、`/wecom/`、`/login`、`/logout`、`/auth/wecom/`、`/p/`、`/pay/`、`/s/`）与 15 组固定中文分类及相对顺序；`/admin...`、`/static...`、HEAD、OPTIONS 不进入文档列表。
- endpoint 排序：path 升序，再按 GET → POST → PUT → PATCH → DELETE（未知 method 排后）。
- fail-closed：`(method,path)` 重复、anchor/slug 碰撞、OpenAPI 解析失败均在 handler 构造阶段报错。
- 页面行为全部浏览器本地：15 组固定分组、快速索引、endpoint cards、大小写不敏感 trim 后 substring 搜索（字段仅 method+path+summary）、单 endpoint/整组/全量 Markdown 复制、hash 初始展开与 hashchange 导航；无 fetch/XHR/WebSocket/表单提交。
- 每 endpoint 允许展示：method、path、summary、description、OpenAPI 标准 security scheme、显式声明的 capability/CSRF/external-effect 元数据、parameter 的 name/location/required/type/format/enum/min/max、response status 与 schema `$ref`；Markdown 中为同一安全字段集。

### 3.2 0033 兼容跳转

- 先完成 session + `config.overview.read` 授权；成功精确返回 `302`，`Location` 精确 `/admin/api-docs`。
- 原 query 全部丢弃；request body 不解析、不记录、不回显；response body 为空；`Cache-Control: no-store`。
- GET 无 CSRF；无 session canonical 401；无 capability canonical 403；非 GET 走 method mismatch。
- 不调用 Gateway、MCP adapter、DB、worker 或 provider。

### 3.3 0034 guard

- 不得注册 `/admin/config/mcp-tools/save`；不得添加 handler、OpenAPI operation 或前端入口；不得把 disposition 改成 DEPRECATED/NOT_MIGRATED/IMPLEMENTED/CLOSED；不得为它选 capability 或新建 config/secret/event/UoW/幂等/receipt。
- 延期状态必须有静态测试保护。

### 3.4 不适用项

UoW / `event_log` / Idempotency-Key / runtime operation receipt / PostgreSQL / migration 对本包全部 NOT_APPLICABLE；实现出现 DB transaction、event append、receipt table、idempotency header 或 provider port 即判 scope violation。工程回执仍必须存在。

## 4. 最大文件白名单

未使用路径保持不动；不得为凑齐白名单创建空文件。

1. `api/openapi.yaml`
2. `api/embed.go`（只 `go:embed openapi.yaml` 并提供只读 bytes）
3. `go.mod`（仅允许把已锁定的 `gopkg.in/yaml.v3 v3.0.1` 从 indirect 调整为 direct；不得升级版本；预期 `go.sum` 无 diff）
4. `cmd/aicrm/api.go`
5. `cmd/aicrm/legacy_api_docs.go`
6. `cmd/aicrm/legacy_api_docs_test.go`
7. `cmd/aicrm/api_test.go`
8. `cmd/aicrm/templates/legacy_api_docs.html`
9. `cmd/aicrm/testdata/api_docs_browser_smoke.mjs`
10. `docs/api-mapping.jsonl`
11. `docs/feature-matrix.csv`
12. `docs/evidence/p1/P4-API-DOCS-MCP-R2-FACTS.md`（本文件）
13. `docs/evidence/slices/P4-API-DOCS-0003-0033-R2.md`
14. `docs/execution/slices/P4-API-DOCS-0003-0033-R2.md`

OpenAPI 变更引发的 generated Go/Orval 输出只能由 canonical generation 产生，禁止手改；任何非预期生成文件立即停下审计。

### 明确排除

- LEGACY-API-0034 实现；`/mcp`、0267/0268、MCP JSON-RPC、tool settings；API Client/API key 功能。
- `internal/gateway/**`、`internal/adminops/{app,store,port}/**`、`internal/auth/**` 修改。
- migrations、SQLc、schema、migration mapping；`web/src/**` React 页面。
- Provider、worker、River、outbound、企微、支付；secret storage、环境变量、credential reference。
- `.github/**`、CI selector、Makefile、ruleset；生产 DB、staging/production 部署。
- 旧 Python 源码进入 implementation worktree、Git 仓库、候选 diff 或成为 import/fixture/运行时资产。

## 5. 必跑测试与验收门

- `git diff --check <BASE>...HEAD`
- `GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go test -race -count=1 -timeout=240s ./cmd/aicrm`
- `GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go test -count=1 ./...`
- `scripts/ci/run_selected_go.sh selected composition false`
- `scripts/ci/run_api_codegen.sh`（canonical generation、OpenAPI contract、Orval；连续两次生成零漂移）
- `node cmd/aicrm/testdata/api_docs_browser_smoke.mjs`
- `scripts/check_repo_contract.sh`
- `scripts/ci/scan_changed_range.sh <BASE> <HEAD> true`（gitleaks 8.30.1）
- `git diff --exit-code -- go.sum package-lock.json`；`test -z "$(git status --porcelain)"`

browser smoke 至少断言：搜索匹配和组显隐；endpoint/group/full Markdown 复制；hash 初始化与 hashchange；HTML/JSON 特殊字符不能执行；页面加载和交互期间除 HTML 本身外无业务网络请求；clipboard failure 安全降级；不出现被禁止字段或旧误导文案。

不执行项逐项写明：PostgreSQL 16.14 `NOT_EXECUTED / NOT_APPLICABLE`；migration up/down `NOT_EXECUTED / NOT_APPLICABLE`；provider/企微/支付/外发 `NOT_EXECUTED`；staging/production `NOT_EXECUTED`；真实 secret/session `NOT_EXECUTED`。

PR 门：只基于 base `d81ba86…` 的 clean worktree；中文 PR；无无关格式化或重构；只等待 `ci / merge-gate`；成功后 match-head squash merge；merge 后重新核验 main SHA/tree 与 0003/0033 路由和 0034 guard；未产生 deploy 证据前只声明 MERGED。

## 6. 外部操作记录

legacy source、provider、企微、支付、外发、staging、production、live migration、真实数据均 `NOT_EXECUTED`；本附录不含 `.git`、DB dump、session cookie、token、生产/staging 捕获、provider 配置或任何密钥。
