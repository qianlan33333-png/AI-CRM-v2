# G1-D02 迁移映射例外清单

> 状态：**用户已确认；G1-D02 已固化**
>
> 决策锚点：`G1-D02-2026-08-10`
>
> 基线：`main@57bb4ca4b4b8e1b46978e6e513f6d9cdf28f3af7`
>
> 冻结输入：`docs/migration-mapping.jsonl`，316 行，SHA-256 `d87e602bff788b097f6b502843b3d815419889e022da21292434670f537da28e`

## 结论

逐行扫描后的实际规模大于执行计划中的“预计几十行”：

| 集合 | 行数 | 是否计入需审阅集合 | 说明 |
|---|---:|---|---|
| 已知旧表、字段落点已完整 | 122 | 否 | 可按“全部照搬既有 mapping 语义”规则批准 |
| framework metadata | 1 | 否 | `alembic_version`，不作为业务数据迁移 |
| 已知旧表、仍缺新 schema 字段落点 | 95 | 是 | 62 行仍为 `PENDING_TARGET_SCHEMA`；33 行虽为 `MIGRATE_CANDIDATE`，但仍有字段未落位 |
| 冻结 head schema 中不存在 | 98 | 是，需旧库只读 preflight | 不能确认真实旧库是否存在、列形状如何；其中 42 行当前建议 drop、56 行仍 pending |
| MIGRATE 转换合同 | 33 | 是，但与前述 95 行重叠 | 24 行可确认具体语义转换；9 行只能确认目标 schema 未冻结，暂不能断言具体变化 |
| 已证实的旧库脏数据 | 0 | 否 | 当前证据只有 synthetic/frozen-head schema，无真实旧库行数据；不得把风险写成已证实事实 |

因此，**唯一需审阅/补证的去重集合为 193 行**：95 行目标落点未闭合 + 98 行源表存在性未闭合。其余 123 行可按规则批准。33 行 `MIGRATE_CANDIDATE` 是 95 行的子集，不重复计数；其中 24 行可由冻结文本确认具体语义转换，另 9 行只能确认存在转换合同与目标 schema 缺口。

这不是对旧能力的重新设计；它只指出现有 mapping 自己已经声明的未闭合处。对 98 行做存在性/列形状/脏数据确认需要另行授权真实旧库只读 preflight，本文件没有连接真实数据。

## 三类判定

### A. 旧字段在新 schema 无对应落点

判定口径：`source_presence=HEAD_PHYSICAL` 且 `target_schema_status=PENDING_TARGET_SCHEMA`。这些表的旧字段已经从冻结 schema 读出，但至少一个字段仍未落到冻结的新 schema 字段。共 95 行、1,421 个 field mappings：1,301 个仍是 `PENDING_TARGET_SCHEMA`，120 个只有 `planned:*` 语义落点而非冻结物理字段。

特别优先审阅：

- `LEGACY-T14-152 crm_user_identity`：30 个字段仍 pending；外部身份必须按 corp/open-platform/app/E.164 provenance 拆到 `identities`，客户字段只能在可信 Bind 后进入 `customers`。
- `LEGACY-T14-314 wecom_external_contact_identity_map`：8 个字段仍 pending；`external_userid` 缺 `corp_id`、`unionid` 缺开放平台账号、`openid` 缺 `appid` 时不得直接绑定。
- `LEGACY-T14-054/055/260/312`：旧 unionid/openid/external_userid 事件必须先经 identity Resolve；无法唯一归因时落 `pending_events`/quarantine，不能猜客户。
- `LEGACY-T14-135/136/148/242/276`：以 unionid 作为旧主键或客户关联的读模型/事件/标签，需要 import ledger + identity port 改写到 OneID，不能把 unionid 搬进 `customers`。

| mapping_id | legacy_table | 当前建议 | 未落位字段数 | 已声明候选目标 |
|---|---|---|---:|---|
| `LEGACY-T14-008` | `ai_audience_hxc_member_usage_projection_control` | `PENDING_TARGET_SCHEMA` | 11 | — |
| `LEGACY-T14-012` | `ai_audience_outbound_subscription` | `PENDING_TARGET_SCHEMA` | 14 | — |
| `LEGACY-T14-013` | `ai_audience_package` | `MIGRATE_CANDIDATE` | 20 | `planned:segments` |
| `LEGACY-T14-014` | `ai_audience_package_dependency` | `PENDING_TARGET_SCHEMA` | 7 | — |
| `LEGACY-T14-015` | `ai_audience_package_group` | `PENDING_TARGET_SCHEMA` | 4 | — |
| `LEGACY-T14-017` | `ai_audience_package_sender` | `PENDING_TARGET_SCHEMA` | 8 | — |
| `LEGACY-T14-018` | `ai_audience_package_version` | `MIGRATE_CANDIDATE` | 20 | `planned:segments` |
| `LEGACY-T14-019` | `ai_audience_refresh_intent` | `PENDING_TARGET_SCHEMA` | 30 | — |
| `LEGACY-T14-027` | `attachment_library` | `PENDING_TARGET_SCHEMA` | 13 | — |
| `LEGACY-T14-028` | `audience_rule` | `MIGRATE_CANDIDATE` | 6 | `planned:segments` |
| `LEGACY-T14-030` | `audience_rule_version` | `MIGRATE_CANDIDATE` | 9 | `planned:segments` |
| `LEGACY-T14-037` | `automation_agent_config` | `PENDING_TARGET_SCHEMA` | 8 | `planned:automations` |
| `LEGACY-T14-038` | `automation_agent_idempotency` | `PENDING_TARGET_SCHEMA` | 12 | — |
| `LEGACY-T14-045` | `automation_agent_runtime_config` | `PENDING_TARGET_SCHEMA` | 18 | — |
| `LEGACY-T14-050` | `automation_agents` | `PENDING_TARGET_SCHEMA` | 14 | `planned:automations` |
| `LEGACY-T14-052` | `automation_channel` | `MIGRATE_CANDIDATE` | 24 | `planned:channels` |
| `LEGACY-T14-054` | `automation_channel_assignment_event` | `MIGRATE_CANDIDATE` | 10 | `planned:customer_events<br>planned:pending_events` |
| `LEGACY-T14-055` | `automation_channel_contact` | `MIGRATE_CANDIDATE` | 5 | `planned:customers<br>planned:customer_events` |
| `LEGACY-T14-065` | `automation_frequency_budget` | `PENDING_TARGET_SCHEMA` | 10 | — |
| `LEGACY-T14-066` | `automation_frequency_consumption` | `PENDING_TARGET_SCHEMA` | 8 | — |
| `LEGACY-T14-067` | `automation_group_ops_effect_dependency` | `PENDING_TARGET_SCHEMA` | 11 | — |
| `LEGACY-T14-068` | `automation_group_ops_effect_graph` | `PENDING_TARGET_SCHEMA` | 16 | — |
| `LEGACY-T14-069` | `automation_group_ops_effect_material` | `PENDING_TARGET_SCHEMA` | 9 | — |
| `LEGACY-T14-071` | `automation_group_ops_plan_groups` | `PENDING_TARGET_SCHEMA` | 10 | — |
| `LEGACY-T14-072` | `automation_group_ops_plan_member` | `PENDING_TARGET_SCHEMA` | 12 | — |
| `LEGACY-T14-073` | `automation_group_ops_plan_nodes` | `PENDING_TARGET_SCHEMA` | 12 | — |
| `LEGACY-T14-074` | `automation_group_ops_plan_scope` | `PENDING_TARGET_SCHEMA` | 5 | — |
| `LEGACY-T14-075` | `automation_group_ops_plan_segmentation` | `PENDING_TARGET_SCHEMA` | 7 | — |
| `LEGACY-T14-076` | `automation_group_ops_plans` | `PENDING_TARGET_SCHEMA` | 15 | `planned:automations` |
| `LEGACY-T14-109` | `automation_sop_template` | `PENDING_TARGET_SCHEMA` | 2 | — |
| `LEGACY-T14-124` | `broadcast_job_hourly_reports` | `PENDING_TARGET_SCHEMA` | 11 | — |
| `LEGACY-T14-125` | `broadcast_jobs` | `MIGRATE_CANDIDATE` | 43 | `planned:outbound_batches<br>planned:outbound_tasks` |
| `LEGACY-T14-126` | `broadcast_queue_notification_settings` | `PENDING_TARGET_SCHEMA` | 9 | — |
| `LEGACY-T14-128` | `campaign_members` | `PENDING_TARGET_SCHEMA` | 18 | — |
| `LEGACY-T14-129` | `campaign_segments` | `PENDING_TARGET_SCHEMA` | 7 | — |
| `LEGACY-T14-130` | `campaign_steps` | `PENDING_TARGET_SCHEMA` | 14 | — |
| `LEGACY-T14-131` | `campaigns` | `PENDING_TARGET_SCHEMA` | 23 | — |
| `LEGACY-T14-132` | `channel_welcome_effect_dependency` | `PENDING_TARGET_SCHEMA` | 16 | — |
| `LEGACY-T14-133` | `channel_welcome_effect_graph` | `PENDING_TARGET_SCHEMA` | 12 | — |
| `LEGACY-T14-135` | `class_user_status_current` | `MIGRATE_CANDIDATE` | 8 | `planned:customers<br>physical:stages<br>planned:staff` |
| `LEGACY-T14-136` | `class_user_status_history` | `MIGRATE_CANDIDATE` | 11 | `planned:customer_events<br>physical:stages` |
| `LEGACY-T14-138` | `cloud_approval_tokens` | `PENDING_TARGET_SCHEMA` | 10 | — |
| `LEGACY-T14-139` | `cloud_broadcast_plan_recipient_messages` | `MIGRATE_CANDIDATE` | 10 | `planned:outbound_tasks` |
| `LEGACY-T14-140` | `cloud_broadcast_plan_recipients` | `MIGRATE_CANDIDATE` | 14 | `planned:outbound_tasks` |
| `LEGACY-T14-141` | `cloud_broadcast_plans` | `MIGRATE_CANDIDATE` | 32 | `planned:outbound_batches` |
| `LEGACY-T14-148` | `contact_tags` | `MIGRATE_CANDIDATE` | 9 | `planned:customer_tags` |
| `LEGACY-T14-152` | `crm_user_identity` | `MIGRATE_CANDIDATE` | 30 | `planned:identities<br>planned:customers` |
| `LEGACY-T14-153` | `crm_user_identity_conflicts` | `PENDING_TARGET_SCHEMA` | 17 | — |
| `LEGACY-T14-154` | `crm_user_identity_merge_audit` | `MIGRATE_CANDIDATE` | 7 | `planned:customer_merges` |
| `LEGACY-T14-155` | `crm_user_identity_resolution_queue` | `MIGRATE_CANDIDATE` | 29 | `planned:pending_events` |
| `LEGACY-T14-163` | `customer_read_model_refresh_intent` | `PENDING_TARGET_SCHEMA` | 23 | — |
| `LEGACY-T14-174` | `external_campaign_preparation_recipients` | `PENDING_TARGET_SCHEMA` | 18 | — |
| `LEGACY-T14-175` | `external_campaign_preparations` | `PENDING_TARGET_SCHEMA` | 27 | — |
| `LEGACY-T14-180` | `external_push_config` | `PENDING_TARGET_SCHEMA` | 18 | — |
| `LEGACY-T14-182` | `group_chats` | `PENDING_TARGET_SCHEMA` | 10 | — |
| `LEGACY-T14-183` | `group_invite_library` | `PENDING_TARGET_SCHEMA` | 17 | — |
| `LEGACY-T14-191` | `hxc_dashboard_broadcast_tasks` | `PENDING_TARGET_SCHEMA` | 17 | — |
| `LEGACY-T14-193` | `image_library` | `PENDING_TARGET_SCHEMA` | 17 | — |
| `LEGACY-T14-194` | `image_library_variants` | `PENDING_TARGET_SCHEMA` | 14 | — |
| `LEGACY-T14-201` | `marketing_automation_configs` | `PENDING_TARGET_SCHEMA` | 7 | `planned:automations` |
| `LEGACY-T14-202` | `marketing_automation_question_rules` | `PENDING_TARGET_SCHEMA` | 13 | `planned:automations` |
| `LEGACY-T14-208` | `miniprogram_library` | `PENDING_TARGET_SCHEMA` | 13 | — |
| `LEGACY-T14-210` | `operation_cycle_action_requests` | `PENDING_TARGET_SCHEMA` | 26 | — |
| `LEGACY-T14-211` | `operation_cycle_attempts` | `PENDING_TARGET_SCHEMA` | 12 | — |
| `LEGACY-T14-215` | `operation_cycle_runners` | `PENDING_TARGET_SCHEMA` | 13 | — |
| `LEGACY-T14-217` | `operation_cycle_snapshots` | `PENDING_TARGET_SCHEMA` | 14 | — |
| `LEGACY-T14-218` | `operation_cycle_stages` | `PENDING_TARGET_SCHEMA` | 13 | — |
| `LEGACY-T14-219` | `operation_cycle_strategies` | `PENDING_TARGET_SCHEMA` | 11 | — |
| `LEGACY-T14-220` | `operation_cycle_strategy_change_proposals` | `PENDING_TARGET_SCHEMA` | 16 | — |
| `LEGACY-T14-221` | `operation_cycle_strategy_version_documents` | `PENDING_TARGET_SCHEMA` | 18 | — |
| `LEGACY-T14-222` | `operation_cycle_strategy_versions` | `PENDING_TARGET_SCHEMA` | 15 | — |
| `LEGACY-T14-223` | `operation_cycle_system_facts` | `PENDING_TARGET_SCHEMA` | 8 | — |
| `LEGACY-T14-225` | `outbound_tasks` | `MIGRATE_CANDIDATE` | 5 | `planned:outbound_tasks` |
| `LEGACY-T14-230` | `owner_role_map` | `MIGRATE_CANDIDATE` | 3 | `planned:staff` |
| `LEGACY-T14-231` | `people` | `PENDING_TARGET_SCHEMA` | 4 | — |
| `LEGACY-T14-237` | `questionnaire_options` | `MIGRATE_CANDIDATE` | 10 | `planned:surveys` |
| `LEGACY-T14-238` | `questionnaire_questions` | `MIGRATE_CANDIDATE` | 8 | `planned:surveys` |
| `LEGACY-T14-239` | `questionnaire_score_rules` | `MIGRATE_CANDIDATE` | 6 | `planned:surveys` |
| `LEGACY-T14-241` | `questionnaire_submission_answers` | `MIGRATE_CANDIDATE` | 11 | `planned:survey_submissions` |
| `LEGACY-T14-242` | `questionnaire_submissions` | `MIGRATE_CANDIDATE` | 12 | `planned:survey_submissions` |
| `LEGACY-T14-243` | `questionnaires` | `MIGRATE_CANDIDATE` | 20 | `planned:surveys` |
| `LEGACY-T14-260` | `radar_click_events` | `MIGRATE_CANDIDATE` | 18 | `planned:customer_events<br>planned:pending_events` |
| `LEGACY-T14-261` | `radar_links` | `PENDING_TARGET_SCHEMA` | 23 | — |
| `LEGACY-T14-262` | `radar_pdf_preview_assets` | `PENDING_TARGET_SCHEMA` | 21 | — |
| `LEGACY-T14-267` | `segments` | `MIGRATE_CANDIDATE` | 17 | `planned:segments` |
| `LEGACY-T14-276` | `sidebar_customer_profile_fields` | `MIGRATE_CANDIDATE` | 6 | `planned:customers` |
| `LEGACY-T14-282` | `user_ops_do_not_disturb_next` | `PENDING_TARGET_SCHEMA` | 9 | — |
| `LEGACY-T14-287` | `user_ops_hxc_send_config` | `PENDING_TARGET_SCHEMA` | 7 | — |
| `LEGACY-T14-295` | `user_ops_send_records_next` | `MIGRATE_CANDIDATE` | 30 | `planned:outbound_tasks` |
| `LEGACY-T14-309` | `wecom_corp_tag_groups` | `MIGRATE_CANDIDATE` | 6 | `planned:tag_groups` |
| `LEGACY-T14-310` | `wecom_corp_tags` | `MIGRATE_CANDIDATE` | 6 | `planned:tags` |
| `LEGACY-T14-312` | `wecom_external_contact_event_logs` | `MIGRATE_CANDIDATE` | 16 | `planned:customer_events<br>planned:pending_events` |
| `LEGACY-T14-313` | `wecom_external_contact_follow_users` | `PENDING_TARGET_SCHEMA` | 13 | — |
| `LEGACY-T14-314` | `wecom_external_contact_identity_map` | `MIGRATE_CANDIDATE` | 8 | `planned:identities<br>planned:customers` |
| `LEGACY-T14-316` | `wecom_media_leases` | `PENDING_TARGET_SCHEMA` | 22 | — |

### B. 类型或语义发生变化

33 行 `MIGRATE_CANDIDATE` 全部包含在上一节的 95 行中，但证据强度不同：

- **24 行可确认具体语义转换**：`LEGACY-T14-013/018/028/030/052/054/125/135/136/139/140/141/148/152/154/155/225/230/243/260/267/295/312/314`。
- **9 行只能确认转换合同待冻结**：`LEGACY-T14-055/237/238/239/241/242/276/309/310`。现有 mapping 没有冻结目标字段/类型，不能把它们写成“已证实 enum、时区、精度或业务语义已变化”；它们仍因目标落点缺口留在例外清单。

| mapping_id | legacy_table | mapping 当前声明的处理合同 |
|---|---|---|
| `LEGACY-T14-013` | `ai_audience_package` | DSL 重编译并对拍 |
| `LEGACY-T14-018` | `ai_audience_package_version` | DSL 重编译并对拍 |
| `LEGACY-T14-028` | `audience_rule` | DSL 重编译并对拍 |
| `LEGACY-T14-030` | `audience_rule_version` | DSL 重编译并对拍 |
| `LEGACY-T14-052` | `automation_channel` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-054` | `automation_channel_assignment_event` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-055` | `automation_channel_contact` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-125` | `broadcast_jobs` | 仅导入只读终态历史，禁止恢复任务 |
| `LEGACY-T14-135` | `class_user_status_current` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-136` | `class_user_status_history` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-139` | `cloud_broadcast_plan_recipient_messages` | 仅导入只读终态历史，禁止恢复任务 |
| `LEGACY-T14-140` | `cloud_broadcast_plan_recipients` | 仅导入只读终态历史，禁止恢复任务 |
| `LEGACY-T14-141` | `cloud_broadcast_plans` | 仅导入只读终态历史，禁止恢复任务 |
| `LEGACY-T14-148` | `contact_tags` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-152` | `crm_user_identity` | 外部身份按 scope/provenance 拆分到 identities |
| `LEGACY-T14-154` | `crm_user_identity_merge_audit` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-155` | `crm_user_identity_resolution_queue` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-225` | `outbound_tasks` | 仅导入只读终态历史，禁止恢复任务 |
| `LEGACY-T14-230` | `owner_role_map` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-237` | `questionnaire_options` | 目标 DDL/identity ownership 冻结后转换 |
| `LEGACY-T14-238` | `questionnaire_questions` | 目标 DDL/identity ownership 冻结后转换 |
| `LEGACY-T14-239` | `questionnaire_score_rules` | 目标 DDL/identity ownership 冻结后转换 |
| `LEGACY-T14-241` | `questionnaire_submission_answers` | 目标 DDL/identity ownership 冻结后转换 |
| `LEGACY-T14-242` | `questionnaire_submissions` | 目标 DDL/identity ownership 冻结后转换 |
| `LEGACY-T14-243` | `questionnaires` | 目标 DDL/identity ownership 冻结后转换 |
| `LEGACY-T14-260` | `radar_click_events` | 先 Resolve；失败进入 pending/quarantine |
| `LEGACY-T14-267` | `segments` | DSL 重编译并对拍 |
| `LEGACY-T14-276` | `sidebar_customer_profile_fields` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-295` | `user_ops_send_records_next` | 仅导入只读终态历史，禁止恢复任务 |
| `LEGACY-T14-309` | `wecom_corp_tag_groups` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-310` | `wecom_corp_tags` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-312` | `wecom_external_contact_event_logs` | 确定性跨模型转换，JSON 需 allowlist |
| `LEGACY-T14-314` | `wecom_external_contact_identity_map` | 外部身份按 corp/app/account scope 拆分 |

其中需特别验证的类型边界：

- `LEGACY-T14-054 automation_channel_assignment_event` 的 `assigned_at/converted_at/created_at/updated_at` 是 `timestamp without time zone`；必须先冻结旧系统解释时区，不能直接按 UTC 或本地时区猜测。
- `LEGACY-T14-237/239/241` 的分数为 `double precision`；新 schema 若改为 decimal/numeric，必须冻结 scale、舍入和边界包含语义。
- 状态/枚举字段不能按同名即同义处理。尤其 outbound 的 queued/claimed/retry/provider_accepted/unknown_after_dispatch 只能迁为签字后的只读终态历史，不能恢复为可执行任务。
- `LEGACY-T14-013/018/028/030/267` 的旧筛选规则必须转为新 DSL 并用真实人群包对拍；禁止执行 legacy SQL。
- JSON/JSONB 字段只允许进入冻结 allowlist 的目标结构，未知键不得静默丢弃或直接注入新执行模型。

### C. 旧库脏数据或源 schema 漂移

**本轮没有真实旧库行级证据，因此“已证实脏数据”是 0 行。** 但有 98 行在 lifecycle manifest 中存在、在冻结 synthetic head schema 中不存在；这些行的 `legacy_columns` 与 `field_mappings` 都是 0，在旧库只读 preflight 完成前，不能把它们视为已不存在，也不能盲签它们的 drop/pending 处置。

| mapping_id | legacy_table | 当前建议 | domain |
|---|---|---|---|
| `LEGACY-T14-003` | `admin_sso_states` | `PENDING_TARGET_SCHEMA` | `legacy_auth` |
| `LEGACY-T14-041` | `automation_agent_output_export_job` | `PENDING_TARGET_SCHEMA` | `legacy_automation_agent` |
| `LEGACY-T14-042` | `automation_agent_prompt_registry` | `PENDING_TARGET_SCHEMA` | `legacy_automation_agent` |
| `LEGACY-T14-043` | `automation_agent_router_config` | `PENDING_TARGET_SCHEMA` | `legacy_automation_agent` |
| `LEGACY-T14-046` | `automation_agent_skill_call_audit` | `PENDING_TARGET_SCHEMA` | `legacy_automation_agent` |
| `LEGACY-T14-047` | `automation_agent_skill_registry` | `PENDING_TARGET_SCHEMA` | `legacy_automation_agent` |
| `LEGACY-T14-060` | `automation_event` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-061` | `automation_event_v2` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-062` | `automation_execution_trace` | `DROP_CANDIDATE` | `automation` |
| `LEGACY-T14-063` | `automation_focus_send_batch` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-064` | `automation_focus_send_batch_item` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-079` | `automation_laohuang_chat_job` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-080` | `automation_member` | `DROP_CANDIDATE` | `automation` |
| `LEGACY-T14-081` | `automation_member_audience_entry` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-082` | `automation_membership_v2` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-083` | `automation_message_activity_sync_item` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-084` | `automation_message_activity_sync_run` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-085` | `automation_operation_task` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-086` | `automation_operation_task_execution` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-087` | `automation_operation_task_execution_item` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-088` | `automation_operation_task_group` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-089` | `automation_operation_template_audit_log` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-090` | `automation_operation_template_idempotency` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-091` | `automation_operation_templates` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-092` | `automation_profile_segment_category` | `PENDING_TARGET_SCHEMA` | `legacy_automation_profile` |
| `LEGACY-T14-093` | `automation_profile_segment_option_mapping` | `PENDING_TARGET_SCHEMA` | `legacy_automation_profile` |
| `LEGACY-T14-094` | `automation_profile_segment_template` | `PENDING_TARGET_SCHEMA` | `legacy_automation_profile` |
| `LEGACY-T14-095` | `automation_profile_segment_template_audit_log` | `PENDING_TARGET_SCHEMA` | `legacy_automation_profile` |
| `LEGACY-T14-096` | `automation_profile_segment_template_idempotency` | `PENDING_TARGET_SCHEMA` | `legacy_automation_profile` |
| `LEGACY-T14-097` | `automation_program` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-098` | `automation_program_admission_attempt` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-099` | `automation_program_channel_binding` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-100` | `automation_program_config_block` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-101` | `automation_program_member` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-102` | `automation_program_member_stage_history` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-103` | `automation_reply_monitor_config` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-104` | `automation_reply_monitor_queue` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-105` | `automation_sop_batch` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-106` | `automation_sop_batch_item` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-107` | `automation_sop_pool_config` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-108` | `automation_sop_progress` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-110` | `automation_stage_entry_v2` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-111` | `automation_task_plan_v2` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-113` | `automation_workflow` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-114` | `automation_workflow_agent_binding` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-115` | `automation_workflow_audience` | `PENDING_TARGET_SCHEMA` | `legacy_automation` |
| `LEGACY-T14-116` | `automation_workflow_execution` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-117` | `automation_workflow_execution_item` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-118` | `automation_workflow_goal` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-119` | `automation_workflow_node` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-120` | `automation_workflow_node_content` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-121` | `automation_workflow_node_content_variant` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-122` | `automation_workflow_node_transition` | `DROP_CANDIDATE` | `automation_legacy` |
| `LEGACY-T14-127` | `callback_jobs` | `PENDING_TARGET_SCHEMA` | `legacy_callback` |
| `LEGACY-T14-134` | `class_term_tag_mapping` | `PENDING_TARGET_SCHEMA` | `legacy_customer_tags` |
| `LEGACY-T14-142` | `codex_schema_backups` | `PENDING_TARGET_SCHEMA` | `legacy_schema_audit` |
| `LEGACY-T14-150` | `conversion_dispatch_log` | `DROP_CANDIDATE` | `automation` |
| `LEGACY-T14-151` | `conversion_feedback` | `PENDING_TARGET_SCHEMA` | `legacy_conversion` |
| `LEGACY-T14-160` | `customer_marketing_state_current` | `PENDING_TARGET_SCHEMA` | `legacy_customer_projection` |
| `LEGACY-T14-161` | `customer_marketing_state_history` | `PENDING_TARGET_SCHEMA` | `legacy_customer_projection` |
| `LEGACY-T14-162` | `customer_read_model_current` | `PENDING_TARGET_SCHEMA` | `legacy_customer_projection` |
| `LEGACY-T14-169` | `customer_value_segment_current` | `PENDING_TARGET_SCHEMA` | `legacy_customer_projection` |
| `LEGACY-T14-170` | `customer_value_segment_history` | `PENDING_TARGET_SCHEMA` | `legacy_customer_projection` |
| `LEGACY-T14-184` | `group_ops_workspace_allowlist_snapshots` | `DROP_CANDIDATE` | `group_ops` |
| `LEGACY-T14-185` | `group_ops_workspace_draft_audit_logs` | `DROP_CANDIDATE` | `group_ops` |
| `LEGACY-T14-186` | `group_ops_workspace_draft_items` | `DROP_CANDIDATE` | `group_ops` |
| `LEGACY-T14-187` | `group_ops_workspace_drafts` | `DROP_CANDIDATE` | `group_ops` |
| `LEGACY-T14-188` | `group_ops_workspace_governance_review_steps` | `DROP_CANDIDATE` | `group_ops` |
| `LEGACY-T14-189` | `group_ops_workspace_governance_reviews` | `DROP_CANDIDATE` | `group_ops` |
| `LEGACY-T14-190` | `group_ops_workspace_gray_window_approvals` | `DROP_CANDIDATE` | `group_ops` |
| `LEGACY-T14-199` | `legacy_webhook_cleanup_audit` | `DROP_CANDIDATE` | `legacy_webhook_cleanup` |
| `LEGACY-T14-200` | `legacy_webhook_deprecation_registry` | `DROP_CANDIDATE` | `legacy_webhook_cleanup` |
| `LEGACY-T14-203` | `marketing_state_current` | `PENDING_TARGET_SCHEMA` | `legacy_customer_projection` |
| `LEGACY-T14-204` | `marketing_value_segment_current` | `PENDING_TARGET_SCHEMA` | `legacy_customer_projection` |
| `LEGACY-T14-206` | `message_batch_items` | `DROP_CANDIDATE` | `messaging` |
| `LEGACY-T14-207` | `message_batches` | `DROP_CANDIDATE` | `messaging` |
| `LEGACY-T14-232` | `prod_fix_backup_ai_audience_regression_20260624` | `PENDING_TARGET_SCHEMA` | `legacy_schema_backup` |
| `LEGACY-T14-233` | `prod_test_backup_ai_audience_20260623` | `PENDING_TARGET_SCHEMA` | `legacy_schema_backup` |
| `LEGACY-T14-234` | `prod_test_backup_ai_audience_full_20260623` | `PENDING_TARGET_SCHEMA` | `legacy_schema_backup` |
| `LEGACY-T14-235` | `questionnaire_continuation_job` | `PENDING_TARGET_SCHEMA` | `legacy_questionnaire` |
| `LEGACY-T14-240` | `questionnaire_scrm_apply_logs` | `PENDING_TARGET_SCHEMA` | `legacy_questionnaire` |
| `LEGACY-T14-263` | `routing_rule_config` | `PENDING_TARGET_SCHEMA` | `legacy_routing` |
| `LEGACY-T14-264` | `schema_migrations` | `PENDING_TARGET_SCHEMA` | `legacy_schema_audit` |
| `LEGACY-T14-277` | `signup_tag_rules` | `PENDING_TARGET_SCHEMA` | `legacy_customer_tags` |
| `LEGACY-T14-279` | `user_ops_activation_status_source` | `PENDING_TARGET_SCHEMA` | `legacy_user_ops` |
| `LEGACY-T14-280` | `user_ops_deferred_jobs` | `DROP_CANDIDATE` | `user_ops` |
| `LEGACY-T14-281` | `user_ops_do_not_disturb` | `PENDING_TARGET_SCHEMA` | `legacy_user_ops` |
| `LEGACY-T14-283` | `user_ops_experience_leads` | `PENDING_TARGET_SCHEMA` | `legacy_user_ops` |
| `LEGACY-T14-284` | `user_ops_huangxiaocan_activation_source` | `PENDING_TARGET_SCHEMA` | `legacy_user_ops` |
| `LEGACY-T14-288` | `user_ops_import_batches` | `PENDING_TARGET_SCHEMA` | `legacy_user_ops` |
| `LEGACY-T14-289` | `user_ops_lead_pool_current` | `DROP_CANDIDATE` | `user_ops` |
| `LEGACY-T14-290` | `user_ops_lead_pool_history` | `DROP_CANDIDATE` | `user_ops` |
| `LEGACY-T14-291` | `user_ops_pool_current` | `DROP_CANDIDATE` | `user_ops` |
| `LEGACY-T14-293` | `user_ops_pool_history` | `DROP_CANDIDATE` | `user_ops` |
| `LEGACY-T14-294` | `user_ops_send_records` | `DROP_CANDIDATE` | `user_ops` |
| `LEGACY-T14-298` | `wechat_pay_order_export_jobs` | `PENDING_TARGET_SCHEMA` | `legacy_commerce` |
| `LEGACY-T14-299` | `wechat_pay_order_identity_repair` | `DROP_CANDIDATE` | `commerce` |
| `LEGACY-T14-308` | `wechat_shop_tokens` | `PENDING_TARGET_SCHEMA` | `legacy_commerce` |

获授权的只读 preflight 至少要验证：

1. 98 张候选表的真实存在性、列名、类型、nullable、主键/唯一约束与行数。
2. 每个 import ledger 源主键是否为空或重复；引用的旧 FK 是否存在孤儿。
3. external_userid/corp_id、unionid/open-platform account、openid/appid、phone/provenance 是否成对完整；同 scope 是否存在冲突重复。
4. 所有状态/枚举实际 distinct values 是否在签字 allowlist 内。
5. 无时区时间列的旧系统解释时区；未来时间、零值、异常早期时间是否存在。
6. questionnaire 浮点分数是否存在 NaN/Infinity、边界舍入差异。
7. outbound/queue/runtime 行是否含非终态或不确定外部效果，确保不会生成 River 任务或重放企微调用。
8. active DND/suppress 数据是否 100% 覆盖；未覆盖时禁止切换。

## 批量批准边界

- 用户已确认本例外清单；316 行已按 G1-D02 转为终态。
- 最终分布：33 `MIGRATE`、57 `ARCHIVE_ONLY`、14 `DROP`、7 `MANUAL_REENTRY`、24 `REBUILD`、20 `RESET_RUNTIME`、160 `DEFER`、1 `NOT_APPLICABLE/NOT_REQUIRED`。
- 160 个 `DEFER` 由 118 个目标 schema 未闭合项和 42 个源表存在性未确认的 drop 候选构成；它们不得生成迁移代码。
- 98 行 source-presence preflight 仍待另行授权；本次没有执行生产数据库读取、迁移代码或 live migration。

## Terra Max 独立复核收据

- `input_sha256`: `d87e602bff788b097f6b502843b3d815419889e022da21292434670f537da28e`
- `independent_report_body_sha256`: `9e17ddf8916dad95b1fad98b29553d13ac4c55831508414e08b27889d8f4acd4`
- `file_manifest_sha256`: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`（空 manifest；Terra 未写仓库）
- 复核结论：95 / 98 / 33 三组计数成立；33 行中严格可确认具体语义转换 24 行、仅目标 schema 未冻结 9 行；98 行 absent 只作为 source-presence preflight，不写成已证实例外或脏数据。
