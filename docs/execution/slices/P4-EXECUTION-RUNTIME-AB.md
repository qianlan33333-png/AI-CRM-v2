# P4-EXECUTION-RUNTIME-AB：Execution Runtime 运行诊断

## 冻结范围与业务结果

- fresh replay base 是 exact-green `1fd449995c2d2ee5d9954b5d10d259b6bcce86bd`。本片完整交付
  `LEGACY-API-0314 GET /api/admin/execution-runtime` 与
  `LEGACY-API-0315 GET /api/admin/executions/{execution_id}`；两条只读路由为同一运行诊断包，
  不拆分、不重复计数。
- 本片锁树共 27 个文件，其中 3 个为唯一生成物、24 个为最小中央接线/测试/完整性文件。它超过通常
  P4 文件预算，是因为唯一 Integration Token 明确要求 A+B 一次同时收口 router、专用 capability、
  OpenAPI/generated、mapping/matrix、hash 与 manifest；拆开会留下可调用而未受合同保护的半包。
- immutable route facts 逐条固定为 `audience=admin`、`auth_scheme=human_session`、
  `capability=admin_read`、`access_scope=global`、`principal_types=human`，owner 是
  `platform_foundation`。实现链是外部 `admin_read` → 内部 `admin.read` → policy 的唯一
  `admin/global` grant；不是从 Admin Shell 的 `admin+ops` 选择类推，且不向 ops 扩权。
- control 缺失是成功读取：0314 返回 HTTP 200 和 `ok:false`；真实 runtime 读取失败固定
  `503 execution_runtime_unavailable`。0315 先 trim，要求 `exe_` 且不超过 100，非法或不存在
  都是 `404 execution_not_found`，读模型错误是 `503 execution_timeline_unavailable`。
- 观测时间统一 UTC Z；树深度、节点、items 上限为 12/256/1024，保留 `graph.truncated`。
  固定 redactor 清除内部 PII、token、URL query 和 message；queue、attempt、status 和 media
  状态 URL 只表示观察状态，绝不称为 provider receipt、executed 或成功。

## 安全、数据与外部边界

- 只有人类 Session 经过 Session → Actor → Capability，带 `admin.read` global grant 时可读；
  sales、ops、未知身份、bearer principal 或非 global grant 都 fail-closed。测试矩阵明确断言
  `admin` 可读、`ops` 不可读；transport 还二次验证精确内部 capability，不能替换为
  `admin.shell.read`。
- 未新增 schema、migration、migration mapping、水位、tenant、UI、worker、provider、写入或
  外部效果。空 adapter 只表达已冻结的 control 缺失；不得伪造 Channel Entry、Group Ops 或 WeCom
  media reader 的 runtime 状态。
- `PRODUCTION_DATABASE_NOT_EXECUTED`、`LIVE_MIGRATION_NOT_EXECUTED`、
  `REAL_EXTERNAL_EFFECT_NOT_EXECUTED`、`DEPLOYMENT_NOT_EXECUTED`。

## 本地验证收据

- `make p4-execution-runtime-ab-acceptance` 覆盖 app 正常/边界/错误、PII redaction、UTC、
  12/256/1024、route 404/503，以及 race；该只读包无数据库变量。
- 还必须在同一 staged tree 运行 `make generate-check`、`make p1-reconciliation-contract`、
  `make openapi-p1-contract`、`make feature-matrix-contract`、`make vet`、`make fmt-check`、
  `scripts/check_repo_contract.sh` 和 `npm run ci`。本卡在合并前不声明 MERGED、RELEASE 或 DEPLOYED。
