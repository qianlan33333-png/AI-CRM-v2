# P4 Admin Config 与 Jobs A+B 本地集成收据

## 冻结基线与水位

- 本候选从 exact-green `356d5445670b91e12b6c7cf9929ac74fb8f0862c` 创建；旧 `04c7486804a6f8cc5ebcbe41b81e36991f7f9f8a` 只作为白名单与语义证据，未 cherry-pick、合并或复用其 shared 变更。
- immutable legacy 事实源为 `6cb989c071255437d75953dabb943318a74eb8f4`。当前主线水位为 42，本候选只新增 `00043_admin_ops_control_plane.sql`；所有历史 `down-to` 与 target-version anchor 保留。
- 实施仍是本地候选，未声明 PR、CLOSED、生产迁移、部署、worker、River、provider 或真实外效已执行。

## 87 条路由分母

范围从 `aicrm_next/platform/admin_config/api.py`（65 条）、`direct_api_key_api.py`（4 条）与 `admin_jobs/routes.py`（18 条）逐条读取 `docs/api-mapping.jsonl`，分母为 87。

- 已落点：77 = 已存在复用 5（`LEGACY-API-0026/0027/0253/0028/0029`）+ 本候选新增 72。新增路由复用现有 Session → Actor → Capability、CSRF、route-bound action token 与 UoW；写 API 仅以 secret reference/mask、local receipt 与 local job intent 处理。
- 本片的 26 个矩阵动作为 `LEGACY-S05-038/039/041..052/054..061/063..066`，均被迁移为 `IN_PROGRESS/NOT_RUN`；这反映已在本地候选完成，不把未 CLOSED 的工作记为 `IMPLEMENTED`。
- 无 tenant 列、复合 tenant 索引、跨域 foreign key、第二套 RBAC/配置框架、worker/River/provider/真实外发或新 UI。

### 余下 10 条（逐条事实、归属与不实施理由）

| mapping | 路由与权威输入/输出 | actor/CSRF/所有权 | 本片归属与边界 |
| --- | --- | --- | --- |
| `0030` | `GET /admin/config/login-access`；`api.py:1427`，Request → HTML（静态声明） | human_session、`manage_config`、CSRF=false | 登录访问页属于已 CLOSED 的 Auth/Session/RBAC transport；本片仅复用其 Session→Actor→Capability，不能新建 Auth page。`LEGACY-S05-040` 已有独立 owner。 |
| `0031` | `POST /admin/config/login-access/directory/refresh`；`api.py:1432`，Request → HTML | human_session、`manage_admin`、CSRF=true | Auth 目录刷新合同，映射为 `DEFERRED_POST_LAUNCH`；不创建目录/权限读写。 |
| `0032` | `POST /admin/config/login-access/save`；`api.py:1440`，Request → HTML | human_session、`manage_admin`、CSRF=true | Auth 管理权限写入，复用已关闭 Auth/RBAC owner；无本片 table/DTO 写入。 |
| `0033` | `GET /admin/config/mcp-tools`；`api.py:1294`，Request → HTML | human_session、`manage_config`、CSRF=false、external=`staging_disabled` | Integration Gateway owner；不建立 MCP 配置页或 adapter。 |
| `0034` | `POST /admin/config/mcp-tools/save`；`api.py:1299`，Request → HTML | human_session、`manage_config`、CSRF=true、external=`staging_disabled` | Integration Gateway owner，且映射标记高风险配置授权；禁止转写 secret 或执行 adapter。 |
| `0254` | `PUT /api/admin/config/app-settings`；`api.py:1398`，Request → JSON | human_session、`manage_config`、CSRF=true | Config owner 的 `A02` 已复用 page/save/GET resource；本 raw PUT 为 `DEFERRED_POST_LAUNCH`，不绕过现有 secret/mask/action-token 合同。 |
| `0265` | `GET /api/admin/config/marketing-automation/signup-conversion`；`api.py:1335`，无 input → `dict` | human_session、`manage_config`、CSRF=false | Automation schema owner；`LEGACY-T14-201/202` 为 `PENDING_TARGET_SCHEMA`，不能由 adminops 伪造读模型。 |
| `0266` | `PUT /api/admin/config/marketing-automation/signup-conversion`；`api.py:1345`，Request → JSON | human_session、`manage_config`、CSRF=true | Automation owner，标记高风险配置授权；同样等待完整 schema/ownership，禁止本片写入。 |
| `0267` | `GET /api/admin/config/mcp-tools`；`api.py:1304`，Request → `dict` | human_session、`manage_config`、CSRF=false、external=`staging_disabled` | Integration Gateway owner；`DEFERRED_POST_LAUNCH`，不复用 staging-only adapter。 |
| `0268` | `POST /api/admin/config/mcp-tools`；`api.py:1317`，Request → JSON | human_session、`manage_config`、CSRF=true、external=`staging_disabled` | Integration Gateway owner，标记高风险；本片不接收 secret/manager payload，也不发外部调用。 |

上述 10 条均有 immutable source line、静态 DTO/响应声明、actor/CSRF 与 owner 事实；未把 77 误写为完整 87，也没有以页面存在替代能力。`0030/0032` 的可见登录访问语义仍由 Auth owner 负责；`0033/0034/0267/0268` 的可见 MCP 语义由 Integration Gateway owner 负责；`0254/0265/0266` 的继承页面或目标 schema 由其现有 owner 负责。

## 11 条迁移映射重放

| mapping | legacy table | 固定处理 | 本候选行为 |
| --- | --- | --- | --- |
| `T14-001` | `admin_login_audit` | `ARCHIVE_ONLY` | 不导入、不恢复 runtime。 |
| `T14-004` | `admin_user_roles` | `MANUAL_REENTRY` | Auth owner，禁止复制权限。 |
| `T14-005` | `admin_users` | `MANUAL_REENTRY` | Auth owner，禁止复制 session/credential。 |
| `T14-024` | `app_settings` | `MANUAL_REENTRY` | 已有 A02；不迁移 legacy values/secrets。 |
| `T14-147` | `config_releases` | `MANUAL_REENTRY` | 新表只承载新本地 release lifecycle，不导入旧 release。 |
| `T14-172` | `deployment_profile_state` | `RESET_RUNTIME` | 新 profile 页面只给 local control-plane 状态。 |
| `T14-201` | `marketing_automation_configs` | `PENDING_TARGET_SCHEMA` | 不读写、不创建迁移目标。 |
| `T14-202` | `marketing_automation_question_rules` | `PENDING_TARGET_SCHEMA` | 不读写、不创建迁移目标。 |
| `T14-205` | `mcp_tool_settings` | `MANUAL_REENTRY` | Integration Gateway owner；只允许未来单独验证的 reference。 |
| `T14-225` | `outbound_tasks` | `MIGRATE_CANDIDATE` | Outbound owner；不从 legacy 复活/重试/发送，只有本片 local intent。 |
| `T14-278` | `sync_runs` | `RESET_RUNTIME` | 不恢复或继续 legacy sync。 |

所有 11 条仍保持 mapping 的 `implementation=NOT_STARTED`、`verification=NOT_RUN`；本候选没有读取 legacy 数据或伪造 migration 完成。

## 安全与运行治理

- `admin_ops_credentials` 与 `admin_ops_notification_settings` 只保存 `secret_ref`/`secret_mask`；payload、metadata、release form 均拒绝 raw `secret/password/webhook/token`。
- 所有写请求从 session principal 生成 `admin:<id>`，须现有 Capability、CSRF 与 route-bound HMAC action token；请求 ID 绑定 receipt，重复同请求同 payload 回放，冲突 payload fail-closed。
- release 状态严格为 `draft → validated → published`，rollback 创建新的 `rolled_back` 事实；Jobs 仅 `queued` local intent，cancel 有 expected-version，`outcome_unknown` 只可 worker-side 标记且不能自动重试。
- API 端不发 provider、只返回 `real_external_call_executed=false`；所有跨域执行入口为 owner-required/retired/仅 local intent。

## 待执行收据

同一锁定 staged tree 后执行：focused race/PG、`make ci-go`、真实 42→43→42→43、一次 `scripts/run_ci_acceptance_manifest.sh` 全 target 汇总、PR 四门、match-head squash 与 exact-main CLOSED。到这些事实存在前，本页不宣称上线或生产效果。
