# P4 后端冻结收据总账

## Radar Local Tracking Core（本地 checkpoint；不追溯改写旧冻结分母）

`00081_radar_local_tracking.sql` 恢复 enabled link 公共 302、identity-free H5 event、
管理端 events/stats/CSV 与 sidebar 安全投影。精确协议、PII 边界、PG16.14 和 CI 证据见
`docs/evidence/p4/radar-local-tracking-core.md`。该本地 receipt 不等于前端接入、main 合并、
Nightly、部署、OAuth/Provider 或真实身份归因；旧 Matrix 中含前端结果的 Radar 行不由此
伪装成整体完成。

## EER External Effects Runtime（V2-native foundation；不变更旧 Matrix 分母）

`00075_external_effects_runtime.sql` 交付 digest-only 的外部效果执行内核：领域内部
Accept/Queue 与 worker Claim/Attempt/Complete/Recover 不暴露为 HTTP；共享 Ops 面只有
`listExternalEffectsRuntime`、`getExternalEffectRuntime`、`getExternalEffectsDiagnostics`、
`cancelExternalEffectRuntime`、`retryExternalEffectRuntime`、`reconcileExternalEffectRuntime`。

- 状态机为 `accepted -> queued -> attempted -> terminal/unknown -> reconciled`；
  `outcome_unknown` 不得自动 retry，Provider I/O 不在数据库事务内。
- API 仅输出 opaque ID、closed owner/kind/state、attempt/generation 和时间；不落库、
  不返回 raw payload、recipient、credential 或 Provider body。
- 读操作为 admin/ops global `operations.read`；控制操作为 admin/ops global
  `operations.manage`，要求 session CSRF 和按 authenticated AdminUserID 绑定的
  Idempotency-Key digest。
- `make p4-external-effects-runtime-acceptance` 在隔离 PostgreSQL 16.14 上验证
  CAS generation/fence、attempt/receipt、unknown/reconcile/recovery、空库 down/up
  与 populated down guard；已登记 manifest/selected database CI。
- 这一包只建立后续 WeCom、Outbound、Webhook、Payment/Refund 所依赖的
  runtime；当前 `x-aicrm-external-effect: none`，不声明任何真实 Provider、支付、退款或企微效果。

## DM Harness（V2-native internal migration control plane；不变更旧 Matrix 分母）

`00076_data_migration_harness.sql` 交付内部数据迁移执行、断点续跑、收据与对账底座。
它不对应公网 operationId，不把历史表硬映射为旧 Matrix 能力，也不推测无法可靠
归属的数据。

- 逐表固定 source identity / schema digest / upper bound，opaque cursor 只在同一
  manifest 与 policy/mapping semantic digest 下续跑。
- generation + lease fence 保证并发单 winner；target mutation、row/result receipt 与
  checkpoint 同事务，exact replay 零重复写，payload/policy 冲突 fail-closed。
- reconcile 要求 source/result/target 全量计数与 comparison digest 一致；quarantine
  未清零时 readiness 不得通过。
- `internal/migration` 保持 operator-controlled 内部控制面；当前不新增 HTTP/OpenAPI、
  RBAC/CSRF 或前端。未来若从管理页触发，应作为独立受控 API 接入包。
- PG16.14 acceptance 由 `p4-data-migration-harness` manifest 项执行 00076 空库
  down/up、运行/续跑/对账与物化事实 down guard。本地验收不等于真实历史迁移、
  部署、Provider、企微、支付或退款效果。

## RP01 Release Plane（V2-native local package；不变更旧 Matrix 分母）

`00074_release_plane.sql` 交付一个 V2-native、本地 release attestation / journal
plane；它不对应旧 Matrix route 或 migration mapping，也不修改本总账已有的 Matrix
完成数。12 个 operationId 为：`listReleaseCandidates`、`registerReleaseCandidate`、
`getReleaseCandidate`、`recordReleasePrerequisite`、`prepareReleaseCandidate`、
`startReleaseCutover`、`restartReleaseCutover`、`completeReleaseCutoverStep`、
`activateReleaseCandidate`、`recordReleaseRollbackCheck`、`requestReleaseRollback`、
`completeReleaseRollback`。

- `getReleaseCandidate` 是唯一 detail 投影，包含 readiness、rollback eligibility、
  prerequisite exact subject tuple、无 fence 的 journal / worker projection。
- `release.read` 仅 admin/ops global，`release.manage` 仅 admin global；所有 POST
  需要 session CSRF 与 Idempotency-Key，actor 只来自 authenticated AdminUserID。
- API adapter、auth policy、final router binding、`internal/release/...` 测试与
  `acceptance/release/release_plane_pg16.sh` 覆盖本地闭环；Nightly/full regression
  由 `p4-rp01-release-plane` manifest 项登记。
- `x-aicrm-external-effect: none`：它只记录本地事实，绝不执行 deploy、backup、
  Provider、payment 或 WeCom 操作。当前记录不宣称已部署或产生任何外部效果。

## C01 Campaign Outbound Dispatch（V2-native；不变更旧 Matrix 分母）

`00078_outbound_campaign_external_dispatch.sql` 把已接受的 00068 Campaign handoff
绑定到 digest-only EER，并形成一条受控本地链路：dispatch → EER Accept/Queue →
River Claim/Attempt/Complete → `outcome_unknown` → 人工 reconcile → terminal projection。

- binding 是 `(handoff_id, customer_id, step_index)` 的 immutable 事实；00068 handoff、
  customer task link、step snapshot 均不修改。表只保存 recipient/payload digest，不保存
  `external_contact_id`、内容正文、Provider body 或凭据。
- `POST .../dispatch` 仅 admin/ops global `operations.manage`，要求 human-session CSRF 和
  actor-bound `Idempotency-Key`。`external_gate=false` 只写 blocked 本地 binding；true 仅
  创建 EER/River 本地工作，composition 的 Provider adapter 默认 disabled 且不会发起网络调用。
- `GET .../dispatch-reconciliation` 是 count-only projection；`outcome_unknown` 没有自动
  retry，只能经 EER 的人工 reconcile。fake/local receipt 和 `executed` 状态都不构成
  `delivery_proven` 或真实外部调用。
- `make p4-outbound-campaign-dispatch-acceptance` 在隔离 PostgreSQL 16.14 验收 00078
  up/down/up、populated down guard、fake receipt、unknown/manual reconcile、actor receipt，
  并断言 `delivery_proven=false`。这不是 Provider/WeCom 交付或送达证据。

## A01 Automation Rules Runtime（V2-native；不硬映射旧 Matrix）

`00080_automation_rules_runtime.sql` 交付闭合的 `customer.tag_applied` 规则、不可变版本、
唯一 enrollment、action ledger 与 EER `outbound_message` handoff。规则 mutation、tag event
消费、EER binding 与 River job 都是本地后端事实；默认 adapter 为 disabled。

- `record` action 只完成本地收据；`outbound_message` 仅接受已验证的
  `text.notice.v1` template reference，并在同一 UoW 固化 digest-only EER envelope。
- `outcome_unknown` 不得自动 retry，只能经 generation/fence/evidence-digest 的人工
  reconcile 收口；本地 `completed` 仍不构成 Provider 接受或送达证明。
- `make p4-automation-rules-runtime-acceptance` 在隔离 PG16.14 验证精确 00080、空库
  down/up、规则快照、幂等 enrollment、EER/River handoff、unknown/manual reconcile 与
  populated down guard。
- 旧 Matrix 没有与该 closed V2 rule/runtime 可靠 1:1 的条目，因此本包对
  `docs/feature-matrix.csv` 为 0 diff；它登记为 V2 后端能力，不反填或重复声明既有
  Automation Agents 与 Group Ops 行。历史 automation 表仍按 data-migration ledger 的
  原有证据状态保留，未执行迁移。
## Survey Identity-Gated External Push（V2-native；不硬映射旧 Matrix）

`00082_survey_identity_gated_external_push.sql` 将已验证的 H5 canonical customer
submission 同一 UoW 绑定到 digest-only EER `survey_webhook`。它新增的是 identity-gated
binding、PII-free detail 和 `outcome_unknown` 的人工 reconcile；不复活旧问卷 webhook
日志、重试或 Provider 调用。

- 只有已验证的 canonical customer 才可建立 binding；H5 OAuth state 一次 claim、safe next
  与 signed proof 均 fail-closed。默认真实 Provider adapter disabled，故真实 code exchange
  仍是 `EXTERNAL_AUTHORIZATION_REQUIRED`。
- binding 只保存 source/target/payload/policy digest。detail 将 `state`、`provider_accepted`
  与 `delivery_proven` 分开投影；reconcile 仅允许 verified `outcome_unknown`，要求
  generation/fence/evidence digest，且绝不自动 retry。
- `make p4-survey-identity-external-push-acceptance` 在独立 PG16.14 库验证 exact 00082、
  empty down/up、H5 fake verified identity、submission/EER binding、fake unknown、手工
  reconcile、PII-free detail 和 populated down guard（55000）。这只证明本地链路；部署、
  Provider acceptance 与 delivery 均 `NOT_EXECUTED`。
- 旧 H5 OAuth 的 GET start/callback 仅登记为窄协议路径参考；旧 `questionnaire_external_push_logs`
  与新 binding/receipt 的身份、收据和重试语义不同，Matrix 不变更状态，历史数据也不导入或重放。

## Commerce External Push 00087（V2 local acceptance；受控回填 S07-151/152/172/173）

`00087_product_external_push.sql` 把商品外推配置和测试收口为 Product-owned 本地事实：
`get/save/queue test` 分别覆盖微信支付商品和服务期商品，共六个 operationId。配置只保存
opaque reference；测试只创建 EER `accepted` receipt/binding，不创建 River job、Provider
request、自动重试或送达声明。

- 读操作为 admin/ops global `products.read`；保存和测试为 `products.write`，都经过 human
  session、CSRF、actor-bound Idempotency-Key、同一 UoW 和 Product receipt。
- 每个 `(product, kind, configuration digest)` 不可变配置只允许一个本地 `accepted` proof；
  原 key 精确重放，换 key 重复测试返回 `409`，且失败事务不留下 Product/EER receipt 或 binding。
- S07-151/152/172/173 回填的是“保存本地配置”和“返回本地测试结果”这一后端契约；`accepted`
  明确不等于 Provider accepted、delivery proven 或真实外部调用。S07-166 的复合商品表单、
  分享和支付语义仍不由本包宣称完成。
- `make p4-commerce-external-push-acceptance` 使用专属 PG16.14 临时库，验证 85 的 GroupOps
  accepted EER 可穿过 87、`87 -> 86 -> 87`、populated down guard、canonical HTTP 的
  RBAC/CSRF/幂等/无孤儿，以及微信支付/服务期两类商品的 accepted-only EER 事实。它不触碰共享验收库。
- `x-aicrm-external-effect: none`：无 Provider 配置、webhook、外部 I/O、job enqueue 或
  delivery receipt；部署及真实支付/外推效果均为 `NOT_EXECUTED`。

## 冻结口径

本收据固定的代码基线是
`origin/main@1aa864f9006576bf9d9d08bed41fe30b9c849301`
（`feat(product): 上线服务期成员网格 canonical 本地后端闭环 (#472)`）。它只盘点十个
已进入该基线、可由 OpenAPI/组合根/应用/存储/迁移/测试交叉追溯的 P4 本地后端包。

- 冻结分母：**10 packages、73 unique operationIds**，即 00054、00061、00063--00070。
  canonical member-grid schema/query 属同一个 member-grid resource family：`00054` 承载
  management DDL，canonical read 无新 migration 并复用 `00064` 的 `service_period_members`；
  因而不另增 package。
- `UNCLASSIFIED`：不在下列 manifest 的操作、前端页面、Provider/企微/支付退款效果、
  未经授权的外部运行，或无法可靠归属到一个 migration/operation 闭环的事项，均不计入
  10/73；不得把全仓遗留项或某个“剩余 116”猜测性加入分母。
- 本总账不修改也不重写 `docs/feature-matrix.csv`。旧 Matrix 只保留各旧操作的证据和
  状态；V2 difference 绝不被伪装为 legacy 1:1 完成。

## 73 operationId manifest

下列是唯一计数的机器可复核清单；每项应同时可在 `api/openapi.yaml` 和
`tools/openapi-contract/main.go` 的合同中定位。

<!-- p4-backend-freeze-operation-ids:start -->

### 00054 Service Period Member Grid Management（11）

- getServicePeriodMemberGridAccess
- getServicePeriodMemberGridSchema
- queryServicePeriodMemberGrid
- listServicePeriodMemberViews
- createServicePeriodMemberView
- updateServicePeriodMemberView
- deleteServicePeriodMemberView
- getServicePeriodMemberGridShareSettings
- createServicePeriodMemberGridCollaborator
- updateServicePeriodMemberGridCollaborator
- deleteServicePeriodMemberGridCollaborator

### 00061 Survey Operations Local Config（7）

- listSurveyExternalPushLogs
- listSurveyQuestionnaireExternalPushLogs
- getSurveyOperationsPageData
- getSurveyOperations
- saveSurveyCompletionOperations
- saveSurveyExternalPushOperations
- queueSurveyExternalPushTest

### 00063 Group Ops Local Plans（20）

- listGroupOpsPlans
- createGroupOpsPlan
- getGroupOpsPlan
- updateGroupOpsPlan
- activateGroupOpsPlan
- pauseGroupOpsPlan
- archiveGroupOpsPlan
- listGroupOpsPlanMembers
- addGroupOpsPlanMember
- removeGroupOpsPlanMember
- listGroupOpsPlanGroupAssets
- addGroupOpsPlanGroupAsset
- removeGroupOpsPlanGroupAsset
- listGroupOpsPlanNodes
- addGroupOpsPlanNode
- updateGroupOpsPlanNode
- removeGroupOpsPlanNode
- getGroupOpsWebhookDescriptor
- putGroupOpsWebhookDescriptor
- previewGroupOpsPlanContent

### 00064 Service Period Members Local Core（7）

- listServicePeriodMembers
- addServicePeriodMember
- exportServicePeriodMembers
- getServicePeriodMember
- updateServicePeriodMemberFields
- expireServicePeriodMember
- removeServicePeriodMember

### 00065 Contact Touch Policy（3）

- getCustomerContactPolicy
- putCustomerContactPolicy
- deleteCustomerContactPolicy

### 00066 Campaign Initiation Snapshots（3）

- listCloudCampaignTouchPlans
- createCloudCampaignTouchPlan
- getCloudCampaignTouchPlan

### 00067 Campaign Review/Handoff（4）

- listCloudCampaignTouchPlanRecipients
- getCloudCampaignTouchPlanRecipient
- getCloudCampaignTouchPlanReview
- mutateCloudCampaignTouchPlanReview

### 00068 Outbound Accept/Reconcile（3）

- getOutboundCampaignHandoffSummary
- acceptOutboundCampaignHandoff
- reconcileOutboundCampaignHandoff

### 00069 S05 Sidebar Local Core（9）

- mintSidebarContext
- getSidebarWorkbench
- updateSidebarProfile
- listSidebarQuestionnaires
- listSidebarOrders
- listSidebarPeriodicOrders
- updateSidebarPeriodicRemark
- listSidebarMaterials
- getSidebarMaterialThumbnailStatus

### 00070 Contact Owner Reassignment Local Core（6）

- downloadContactOwnerReassignmentTemplate
- createContactOwnerReassignmentPreview
- getContactOwnerReassignmentPreview
- executeContactOwnerReassignmentPreview
- downloadContactOwnerReassignmentErrors
- downloadContactOwnerReassignmentResults

<!-- p4-backend-freeze-operation-ids:end -->

## 包级收据

| Migration / package | Native、legacy 与 Matrix 边界 | RBAC、CSRF、幂等、UoW / receipt | Focused / PostgreSQL 证据 | 外部效果 |
| --- | --- | --- | --- | --- |
| Member Grid resource family（`00054_service_period_member_grid_management.sql` management DDL；canonical read 无新 migration、复用 `00064`） | 既有 member-view、协作者和 share-settings 是 legacy-compatible private management；`getServicePeriodMemberGridSchema` / `queryServicePeriodMemberGrid` 为 canonical local 演进。S07-153/154 保持 `IN_PROGRESS/NOT_RUN`：仅记录 canonical schema/query，旧任意排序、group、saved-view switching 与旧行语义仍不硬映射。 | admin/ops global；读为 `products.read` 或 `entitlements.read`，写为 `products.write`。写操作有 session CSRF、idempotency、CAS/version、UoW 与 operation receipt；读不带 CSRF。 | `internal/product/membergrid/*_test.go`；`acceptance/product/member_grid_integration_test.go`；`acceptance/product/member_grid_canonical_pg16.sh`。 | `x-aicrm-external-effect: none`；协作者是本地事实，不发邀请、不建 public share、不执行 Provider。 |
| `00061_survey_operations_local_config.sql` — Survey Operations Local Config | legacy admin route 的数据型兼容，但只保存本地 opaque completion / external-push configuration；test 只排入本地 queued run。Matrix 仍以原 legacy operation/action/result 独立计数，本包不把 queued 写成已投递。 | admin/ops global；`questionnaires.read` / `questionnaires.write`；三个写操作要求 CSRF + Idempotency-Key。receipt 在同一 UoW 完成，payload mismatch conflict。 | `internal/survey/app/operations_test.go`；`internal/survey/http/operations/handler_test.go`；`internal/survey/store/operations_repository_integration_test.go`；`migration-validate`。 | `none`；无 Provider client、webhook payload、River job、自动重试或外部 dispatch。 |
| `00063_group_ops_local_plans.sql` — Group Ops Local Plans | native local plan、member、asset、node、webhook descriptor 和内容预览；descriptor 只是受限 reference，不是 webhook 调用。Matrix 的旧页面/运行时语义不由此包推断完成。 | list/get/preview 为 admin global `admin.read`；写为 admin/ops global `operations.manage`。写 CSRF + Idempotency-Key；revision/CAS、单个 UoW 与 immutable completed receipt。 | `internal/groupops/app/service_test.go`；`internal/groupops/http/handler_test.go`；`internal/groupops/store/repository_integration_test.go`；`make p4-group-ops-acceptance`（PG16 migration/down guard）。 | `none`；无 group-send、runtime、provider、webhook dispatch 或 external-effect state。 |
| `00064_service_period_members.sql` — Service Period Members Local Core | canonical `service_period_members` / `member_ref` 生命周期，不复制 legacy entitlement identity。备注/联盟字段的 CAS 是本地事实；export 是白名单安全投影。Matrix legacy 语义保持独立，不能由这七项反填。 | admin/ops global，读 `entitlements.read`、写 `entitlements.write`；命令 CSRF + idempotency。state transition / version CAS、UoW、member receipt 和 event append 同事务。 | `internal/product/serviceperiodmember/app/service_test.go`；`http/handler_test.go`；`store/repository_integration_test.go`；`acceptance/product/d01_service_period_integration_test.go`。 | `none`；没有 payment、refund、Provider 或 entitlement 外部同步。 |
| `00065_customer_contact_policies.sql` — Contact Touch Policy | V2 原生 Contact policy，不和 USER OPS 或旧路由伪造 1:1。Matrix 保持不可变旧事实。 | admin/ops global；`operations.read` / `operations.manage`；PUT/DELETE 要 CSRF + idempotency、UoW、receipt 和 Contact event。 | `internal/contact/app/contact_policy_test.go`；`internal/contact/http/contact_policy_handler_test.go`；`internal/contact/store/contact_policy_repository_integration_test.go`；`make p4-contact-policy-acceptance`。 | `none`；仅本地 policy / audit facts。 |
| `00066_campaign_initiation_snapshots.sql` — Campaign Initiation Snapshots | V2 immutable local touch-plan snapshots；不把旧 Campaign 页面或发送状态计入 Matrix 完成。 | admin/ops global；`operations.read` / `operations.manage`；create CSRF + idempotency，snapshot / receipt / audit 在同一 UoW。 | `internal/campaign/app/initiation_test.go`、`initiation_race_test.go`、`http_initiation_test.go`；`acceptance/campaign/initiation_repository_integration_test.go`。 | `none`；不创建 send job、不调用 Provider。 |
| `00067_campaign_touch_plan_review_handoff.sql` — Campaign Review/Handoff | V2 review、decision、approved handoff 的本地事实；不等同旧 Campaign 操作或 delivery。 | admin/ops global；`operations.read` / `operations.manage`；mutate 要 CSRF + idempotency，review / handoff / audit 在同一 UoW。 | `internal/campaign/app/review_handoff_test.go`；`internal/campaign/http_review_handoff_test.go`；`acceptance/campaign/review_handoff_integration_test.go`。 | `none`；只产生 held handoff，不执行 dispatch。 |
| `00068_outbound_campaign_handoff_acceptance.sql` — Outbound Accept/Reconcile | V2 held local accept / reconciliation；不能把接受、task link 或 River internal event 说成送达，Matrix 不作 legacy 硬映射。 | admin/ops global；`operations.read` / `operations.manage`；accept 要 CSRF + idempotency，UoW 中完成 receipt、handoff fact 与 event link。 | `internal/outbound/app/campaign_handoff_test.go`；`http/campaign_handoff_handler_test.go`；`acceptance/outbound/campaign_handoff_integration_test.go`；`make p4-outbound-campaign-handoff-acceptance`。 | `none`；明确不创建 Outbound send job，不调用 Provider。 |
| `00069_sidebar_customer_profile_receipts.sql` — S05 Sidebar Local Core | 九项是 V2 后端投影/本地写入，不交付旧 UI debounce、JSSDK/OAuth 或真实 thumbnail。关联的 S05-022/023/024/026/030/031/032/033 均保持 `NOT_STARTED/NOT_RUN`。 | context mint 可选 session；其余 human session + admin/ops global 或 sales owner scope。两个 PUT 要 CSRF + idempotency；profile UoW/receipt/CAS，periodic remark 复用 member receipt。 | `internal/sidebar/app/service_test.go`；`internal/sidebar/http/handler_test.go`；`internal/contact/app/sidebar_profile_test.go`；`acceptance/sidebar/local_core_pg16.sh`。 | `none`；thumbnail 固定 local pending，不生成图片；无 Provider、发送或支付效果。 |
| `00070_contact_owner_reassignment.sql` — Contact Owner Reassignment Local Core | 六项为 Contact-owned CSV preview/execute/result 的 V2 本地流；S07-110..115 都保持 `NOT_STARTED/NOT_RUN`，且 S07-023 被排除。local receipt/result 不等于 WeCom transfer 或旧 XLSX 行为。 | 仅 global admin `contact.owner_reassignment`；写 CSRF + Idempotency-Key，preview 和 execute 都 actor-bound。customer/staff lock、expected version、UoW、completed receipt、`customer_events` 和 event log 同事务。 | `internal/contact/app/owner_reassignment_test.go`；`internal/contact/http/owner_reassignment_handler_test.go`；`internal/contact/store/owner_reassignment_integration_test.go`；`acceptance/contact_owner_reassignment/local_core_pg16.sh`。 | `none`；不读取 WeCom userid、不调用 Provider、不做企微转属。 |
| `00073_internal_event_safe_exports.sql` — EE01 Internal Event Safe Export (**local checkpoint; not in the frozen 10/73 denominator**) | Three Events-owned native operations freeze only local `event_log` / `event_deliveries` rows. No Radar, USER OPS, legacy Matrix, River, Outbound, Provider, or delivery claim is added. | global admin `admin.read`; create requires human session, CSRF and Idempotency-Key; actor-bound reads. Header/rows/audit/completed receipt share one UoW; versioned row/result digests are reconciled on replay/read/download and any missing or changed audit fails closed without repair. | `internal/events/app/safe_export_test.go`、`internal/events/http/safe_export_handler_test.go`; `make p4-ee01-internal-event-safe-export-acceptance` creates and cleans a PG16.14 dedicated DB and proves capacity, tamper, side-effect and down-guard negatives. | `none / NOT_EXECUTED`; no delivery enqueue, River job, outbound task or Provider call. |
| `00080_automation_rules_runtime.sql` — A01 Automation Rules Runtime (**V2 backend capability; not in the frozen 10/73 denominator**) | Closed `customer.tag_applied` rules, immutable versions, enrollments and action snapshots; no reliable legacy Matrix 1:1 mapping is claimed. | admin/ops rule read/write; mutations use CSRF + actor-bound idempotency; tag event enrollment and EER binding share one UoW. | `internal/automation/...` focused/race tests；`make p4-automation-rules-runtime-acceptance` on PG16.14. | Provider adapter disabled；`outcome_unknown` only manual reconcile；deployment and real delivery `NOT_EXECUTED`. |
| `00082_survey_identity_gated_external_push.sql` — Survey Identity-Gated External Push (**V2 backend capability; not in the frozen 10/73 denominator**) | H5 verified canonical-customer submission binding plus PII-free detail/manual reconcile; old H5 OAuth routes are only narrow protocol references and old external-push logs are not a 1:1 target. Matrix stays unchanged. | public H5 start/callback has no CSRF; admin detail requires `questionnaires.read`, reconcile requires `questionnaires.write`, session CSRF and Idempotency-Key. Submission/EER binding and reconcile receipt use one UoW; unknown has no automatic retry. | `acceptance/survey/identity_external_push_pg16_test.go`; `make p4-survey-identity-external-push-acceptance` on PG16.14. | real Provider adapter disabled; fake/local result only; deployment, Provider acceptance and delivery `NOT_EXECUTED`. |
| `00083_media_content_package_delivery.sql` — Media Content Package & Delivery Binding (**V2-native local package; not in the frozen 10/73 denominator**) | Content package reuses canonical image/PDF/miniprogram/group-invite IDs; package/CAS binding and PDF multipart are local. `outbound_media` uses an EER accepted envelope, opaque effect ID and manual reconciliation. No legacy route is guessed and no Matrix row or denominator changes. | human session; package/PDF/binding/outbound accept and reconcile require `media.library.write`, CSRF and Idempotency-Key; detail is `media.library.read` without CSRF. Package/binding mutations use CAS; accepted snapshot/binding and reconciliation receipt are immutable local facts. | `internal/media/app/*content*test.go`、`*outbound*test.go`、`internal/media/store/*content*test.go`; `make p4-media-content-delivery-acceptance` runs isolated PG16.14 exact-83, empty down/up, package/PDF/effect/reconcile detail and populated-guard checks; selected media database CI invokes it. | `none / NOT_EXECUTED`; provider adapter is disabled/fail-closed. `provider_accepted` and `delivery_proven` are separate manual facts; no send/delivery claim or automatic retry. |
| `00084_ai_audience_local_configuration_closure.sql` — AI Audience Local Configuration Closure (**V2-native typed local package; not a legacy-flow completion claim**) | Appends fixed-schema snapshots of the canonical Segment definition/package version/digest, exposes PII-free deterministic preview and receipt-backed local materialize, validates senders through the Contact-owned port, and keeps versioned Automation binding. The writerless send-record/HMAC projections are removed. `LEGACY-S06-022`/`024`/`026` remain `NOT_STARTED/NOT_RUN`: legacy catalogue/SQL semantics, directory sync and send-record detail are separate packages. | human session plus global admin/ops `segments.read` or `segments.write`; binding/senders/configuration/materialize mutations require CSRF + actor-bound Idempotency-Key and share caller UoW, completed receipt and event semantics. Preview/materialize return only count/digest lineage and never expose contact IDs or Provider facts. | focused/race tests under `internal/segment/{app,legacyaudience}`; isolated PG16 exact-84 acceptance runs HTTP → handler → service → UoW → Contact/Automation ports → Segment engine → DB/event/receipt and proves replay, changed-payload 409, no duplicate event, failure rollback, down/up and populated guards. | `none / NOT_EXECUTED`; local evaluation only. No agent start, queue/enqueue, Provider call, send, acceptance or delivery assertion. Legacy import remains unimplemented. |
| `00087_product_external_push.sql` — Commerce External Push (**V2 local acceptance; S07-151/152/172/173 backend contract**) | Six Product-owned configuration/test operations for WeChat-pay and service-period products. The test response is only local EER `accepted`; S07-166's composite product/page/share semantics stay out of scope. | admin/ops global `products.read` / `products.write`; writes require CSRF + actor-bound idempotency, receipt and one UoW. | `internal/product/{app,http,store}/external_push*`; `acceptance/product/external_push_00087_pg16.sh`; `make p4-commerce-external-push-acceptance`. | `none / NOT_EXECUTED`; no Provider request, webhook, River job, automatic retry, delivery claim or production effect. |

所有表中命令操作都遵循其 OpenAPI `x-aicrm-session-bound-csrf` 与
`x-aicrm-external-effect: none` 合同；“receipt”只证明本地数据库提交/重放，不证明
Provider 接受、送达、支付、退款或生产外部事实。

## 分层状态与当前 Nightly

| 层级 | 当前事实 | 不可推导的结论 |
| --- | --- | --- |
| 后端能力 | 基线中为 `10 packages / 73 unique operationIds` 的本地能力；各包的 API、应用/存储、migration 与上表 focused/PG 路径可追溯。 | 不等于前端流程完成。 |
| 旧 Feature Matrix | 精确基线为 **177 IMPLEMENTED / 5 IN_PROGRESS / 112 NOT_STARTED / 294 total**。本提交对 Matrix 为 0 diff。 | Matrix 状态不自动证明 V2 差异、部署或外部效果。 |
| main | 上述代码已在 `origin/main@1aa864f9006576bf9d9d08bed41fe30b9c849301`；本收据提交本身尚未进入 main。 | 不等于当前 Nightly 通过。 |
| Nightly | [run 32715607669](https://github.com/qianlan33333-png/AI-CRM-v2/actions/runs/32715607669) 对该 exact SHA 于 2026-08-24 失败；唯一首错为 `arch-import-lint: forbidden cross-module import in internal/config/http/setup_wizard_test.go: github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http`。 | 这是既有 setup-wizard 架构门，不是十包任一业务能力失败；本 docs-only 收据不扩展修复它。 |
| 部署 | `NOT_EXECUTED`。 | main、focused 或 PG16 不构成部署证明。 |
| Provider / 外部效果 | `false / NOT_EXECUTED`：73 项的 OpenAPI 合同均标记 `x-aicrm-external-effect: none`。 | 不得将本地 receipt、queued test、held handoff 或 reconciliation 当作真实外部效果。 |

## 本收据的验证

本 docs-only 提交应运行下列最小合同组合；它们验证已有 OAS、Matrix 和 migration 合同，
不借此修复任何全仓首错。

```text
make feature-matrix-contract openapi-p1-contract migration-validate
```

manifest 唯一计数可用：

```text
sed -n '/p4-backend-freeze-operation-ids:start/,/p4-backend-freeze-operation-ids:end/p' \
  docs/evidence/p4/backend-capability-ledger.md \
  | awk '$1 == "-" && $2 ~ /^[a-z]/ { print $2 }' \
  | sort -u | wc -l
# 73
```

路径存在性可由 manifest 中每个 operationId 在 `api/openapi.yaml` 与
`tools/openapi-contract/main.go` 的对应合同项复核；不将此总账扩展成 SQLC、fixture
ownership、性能、通用安全或 setup-wizard 治理修复。
