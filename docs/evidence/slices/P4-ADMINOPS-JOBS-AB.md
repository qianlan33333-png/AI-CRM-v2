# P4 Admin Config 与 Jobs A+B 本地集成收据

## 冻结基线与水位

- 本候选从 exact-green `356d5445670b91e12b6c7cf9929ac74fb8f0862c` 创建；旧 `04c7486804a6f8cc5ebcbe41b81e36991f7f9f8a` 只作为白名单与语义证据，未 cherry-pick、合并或复用其 shared 变更。
- immutable legacy 事实源为 `6cb989c071255437d75953dabb943318a74eb8f4`。当前主线水位为 42，本候选只新增 `00043_admin_ops_control_plane.sql`；所有历史 `down-to` 与 target-version anchor 保留。
- 实施仍是本地候选，未声明 PR、CLOSED、生产迁移、部署、worker、River、provider 或真实外效已执行。

## 87 条路由分母

范围从 `aicrm_next/platform/admin_config/api.py`（65 条）、`direct_api_key_api.py`（4 条）与 `admin_jobs/routes.py`（18 条）逐条读取 `docs/api-mapping.jsonl`，分母为 87。

- 完整销账：87 = 新增 73（原 AdminOps/Jobs 72 + 漏承接的 Admin Config JSON 写入 `0254` 1）+ 已上线能力复用 5（`LEGACY-API-0026/0027/0253/0028/0029`）+ 唯一移交 9（Auth 3、Integration Gateway 4、Automation Engine 2）。三组互斥，逐项 route ID 如下；`0254` 新增的是窄 PUT transport，业务/存储仍只复用已上线 A02 Config service，不构成第二个配置域。
- 原 72 条新增路由及 `0254` 均复用现有 Session → Actor → Capability、CSRF、route-bound action token 与 UoW；写 API 仅以 secret reference/mask、local receipt 与 local job intent 处理。没有把新增、复用或移交任一组重复相加。
- 本片的 26 个矩阵动作为 `LEGACY-S05-038/039/041..052/054..061/063..066`，均被迁移为 `IN_PROGRESS/NOT_RUN`；这反映已在本地候选完成，不把未 CLOSED 的工作记为 `IMPLEMENTED`。
- 无 tenant 列、复合 tenant 索引、跨域 foreign key、第二套 RBAC/配置框架、worker/River/provider/真实外发或新 UI。

### 10 条逐项销账（权威契约、处置与唯一归属）

2026-08-15 的全量 A+B 裁定覆盖早期的 post-launch 分类。本表只使用 immutable source `6cb989c…` 的静态路由/DTO/actor 事实，以及同一冻结源的 handler 业务语义；没有以页面存在替代能力，也没有把旧分类作为排除依据。

| mapping | 当前权威请求、响应与业务语义 | actor / RBAC / CSRF | 处置与唯一计数 |
| --- | --- | --- | --- |
| `0030` | `GET /admin/config/login-access`，保留非空 query，`302` 到 `/admin/config/detail/admin_access`；该详情读取管理员、角色与登录审计。 | human session，`manage_config`，读请求无 CSRF。 | **移交 Auth A+B**：`admin_users/admin_user_roles/admin_login_audit` 与 `LEGACY-S05-040` 的唯一 owner；Admin87 只计 1 次 Auth 移交。 |
| `0031` | `POST /admin/config/login-access/directory/refresh`，form 的 action token 无效时 `302 detail?error=…`；有效时 `302 detail?notice=通讯录刷新已跳过…`，绝不触发真实企微外呼。 | human session，`manage_admin`，CSRF + route action token。 | **移交 Auth A+B**：该受控 no-op 是管理员目录链的一部分；不在 AdminOps 建目录或任何外部调用。 |
| `0032` | `POST /admin/config/login-access/save`，form 需 `wecom_userid`，规范化 `admin_level/role_codes/login_enabled`，写管理员与角色后 `302 detail?saved=1&edit_id=…`。 | human session，`manage_admin`，CSRF + route action token；operator 来自 actor。 | **移交 Auth A+B**：管理员/角色/审计写模型唯一归 Auth，不复制 RBAC 或表。 |
| `0033` | `GET /admin/config/mcp-tools` 返回 HTML redirect 到 `/admin/api-docs`，没有 MCP 设置读写。 | human session，`manage_config`，读请求无 CSRF。 | **移交 Integration Gateway A+B**：仅计 1 次 Gateway 移交；MCP adapter 与 secret 处理不进入 AdminOps。 |
| `0034` | `POST /admin/config/mcp-tools/save` 返回 HTML redirect 到 `/admin/api-docs`，不接收或执行 adapter 配置。 | human session，`manage_config`，CSRF；高风险配置写。 | **移交 Integration Gateway A+B**：唯一 Gateway owner，禁止 secret 转写和外效。 |
| `0254` | `PUT /api/admin/config/app-settings` 接收 JSON object：`settings` object（缺省视为 `{}`）、`confirm=true`、header/body action token；成功返回 `{ok,changed,changed_count,config,source_status,fallback_used,real_external_call_executed}`，错误为有限 JSON code。 | human session，`config.settings.manage`，现有 CSRF middleware + 精确 PUT route token；actor 只能为 `admin:<id>`。 | **本板块新增**：Admin Config JSON 写入子能力的漏承接修复（`slice_induced #1`）；新增窄 PUT transport，复用 A02 Config Manager/UoW；raw secret 在 service 前拒绝，响应只含 mask/配置状态，无第二设置表。 |
| `0265` | `GET /api/admin/config/marketing-automation/signup-conversion` 无 body，返回 `{ok,config,source_status=next_read_model,fallback_used=false}`；config 是 signup-conversion、问卷/规则、阈值、时区的 Automation read model。 | human session，`manage_config`，读请求无 CSRF。 | **移交 Automation Engine A+B**：`marketing_automation_configs/question_rules` 与其问卷关系为 Automation 唯一 schema；Admin87 只计 1 次移交。 |
| `0266` | `PUT /api/admin/config/marketing-automation/signup-conversion` 接收 JSON；校验 questionnaire、choice rules、threshold/timezone，成功返回 `{ok,config,source_status=next_command,fallback_used=false,real_external_call_executed=false}`。 | human session，`manage_config`，CSRF + action token；高风险配置写。 | **移交 Automation Engine A+B**：保持其 schema、问卷关系和本地 command 的唯一 ownership；AdminOps 不伪造 read model 或写入。 |
| `0267` | `GET /api/admin/config/mcp-tools?q=&enabled_only=` 返回 `{ok,config,source_status=next_read_model,fallback_used=false}`；只读 MCP tool setting projection。 | human session，`manage_config`，读请求无 CSRF。 | **移交 Integration Gateway A+B**：同一 MCP contract 的唯一 Gateway 移交，不与 `0033` 合并计数。 |
| `0268` | `POST /api/admin/config/mcp-tools` 接收 JSON tool identity/display/enable/visibility/sample/sort fields，成功返回 `{ok,item,source_status=next_command,fallback_used=false,real_external_call_executed=false}`。 | human session，`manage_config`，CSRF + action token；高风险配置写。 | **移交 Integration Gateway A+B**：唯一 Gateway owner，禁止在本片保存 secret 或执行 adapter/provider。 |

闭合等式为 **87 = 73 新增（72 + `0254`）+ 5 复用 + 9 唯一移交**。Auth `{0030,0031,0032}`、Integration Gateway `{0033,0034,0267,0268}`、Automation Engine `{0265,0266}` 与本片 `{0254}` 两两不交；每个 route ID 恰好落入其中一组。移交的 9 条仍须由其唯一 A+B 板块取得各自的上线/CLOSED 收据；它们不因 Admin Config 与 Jobs 的关闭而被重复宣告上线，也不会从 769 总分母消失。

### 修正归因与冻结阈值

- `slice_induced=1`：`0254` 是既有 Admin Config JSON 写入口的漏承接；本轮只新增该 route-bound PUT transport，并复用 A02 service。若这个**同一 JSON 写入子能力**再出现第 2 个 `slice_induced`，立即冻结该子能力，只允许严格 repair-only，不能扩展到配置框架、UI、权限、worker 或其他路由。
- `verification_induced`（不计入上述阈值）：Automation Agents current waterline 默认值从 42 同步到 43；空库直接跑 manifest 时 `customers` 尚未由 migration runner 建立；dirty shared test DB 残留 event idempotency/River job 后，完整 manifest 需先收集 40 target 全部结果、重建指定 55431 的 `aicrm_test` 并在 `migration-integration` 的 43 前置条件后重跑。PR #227 旧 head `03487dbcef6979bcd5e77a349c77077a3e0b04e4` 的唯一业务门失败是 frozen `tools/openapi-contract` 未登记 `saveLegacyAppSettingsResource`；293 feature matrix、316 migration mapping、781 reconciliation 与 repo-contract/web/security 均通过。修复只同步该 operation 的直接消费者、allowlist、证据映射和负例，未改变 `0254` 的请求/响应或业务语义，仍不计入 `slice_induced`。这些均没有改变业务语义或扩展能力。

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

## 43 水位消费者全量审计

以 `waterline`、`expected-waterline`、`goose up`、`up-to/down-to` 和 manifest 顺序扫描 `acceptance/`、`Makefile`、`scripts/` 后，以下是与本候选相关的完整分类；没有作全局数值替换。

| 类别 | 直接或间接消费者 | 处理 |
| --- | --- | --- |
| 直接 current | `P3-O6A/O6B1/O6B2`、`P4-W0-D01/L01`、`P4-SI00B` 的最终 `goose up` 与 waterline 断言 | 已统一断言 43。 |
| 直接 current | `acceptance/automation/d01_integration_test.go`、`acceptance/stats/l01_integration_test.go`、`acceptance/operationcycle/ab_integration_test.go`、Agents storage 默认参数 | 已统一默认/断言 43；Operation target 先 `goose up`。 |
| 间接 current | `P3-W4`、`P3-S05A`、`P4-A01` 的 Make `goose up`，以及本片 control-plane compatibility 最终 `up-to 43` | 无数字硬编码，天然使用 latest；本片最终明确验证 43。 |
| 历史 target anchor | `agents_ab_migration_compatibility.sh` 的 41→42→41→42 fixture，以及它传入的 `-expected-waterline 42` | 保留 42，因为它证明 migration 42 本身；入口现在可从 43 正确先降到 41。运行在 D01 current 环境下的同一 storage 测试默认改验 43。 |
| 历史 target anchor | `order/ab_migration_compatibility.sh` 的 38→40→38→40→42 和其他 `up-to N/down-to N` compatibility fixtures | 原样保留；这些不是 current consumer，且由后续显式 `goose up` /下一兼容 fixture 归一化。 |

审计结论：唯一遗漏的 current 断言是 Automation Agents storage 的默认 42；它与其历史 42 fixture 已被明确拆分。未发现其他“latest/current=42”断言或把 43 视为未知的归一化分支。

## 本地实际收据

- 业务/契约修复锁定在 `aa00e6410fea88c7f98214a4ec3d11d879a0f591`：`make openapi-p1-contract`、`scripts/check_repo_contract.sh` 与 `scripts/test_repo_contract.sh`（含新 operation 缺失、action-token extension 缺失的负例）均 PASS；该修复只承接旧 head 的 OpenAPI 直接消费者/allowlist 漏同步。
- 同一业务/契约 HEAD 的 `make ci-go` 在指定 PG16.14 `55431/aicrm_test` 上 PASS，包含 `feature-matrix=293`、`migration-mapping=316`、`p1-reconciliation=781` 与 `p4_config_settings_operations=4`；未改动工作树。
- 经确认该指定测试库没有活跃连接后重建；`migration-integration` 实际完成 latest `43` 的 up/down/up。以该 43 前置条件，`p4-adminops-jobs-ab-acceptance` 实际 PASS：`42→43→42→43`，且保持 Auth/session/Event/Automation 历史与 secret/worker/provider 边界。
- `ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 scripts/run_ci_acceptance_manifest.sh` 在同一 43 前置条件 PASS，汇总为 `ci-acceptance-manifest: PASS entries=40`。空库缺少 `customers` 的首次现象只作为 manifest 前置条件记录，未列为 slice 问题。
- 本收据仅证明本地候选。尚未 push、PR、match-head squash、exact-main CLOSED、生产迁移、部署、worker、River、provider 或真实外效；因此不宣称上线或生产效果。
