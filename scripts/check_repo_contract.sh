#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
  echo "repo-contract: $*" >&2
  exit 1
}

git diff --quiet -- . ||
  fail "working tree differs from the staged index; contract would be ambiguous"

required=(
  README.md
  AGENTS.md
  CLAUDE.md
  CONTRIBUTING.md
  SECURITY.md
  NOTICE
  .tool-versions
  .gitleaks.toml
  package.json
  package-lock.json
  Makefile
  go.mod
  go.sum
  tools/go.mod
  tools/go.sum
  .github/CODEOWNERS
  .github/pull_request_template.md
  .github/workflows/application-go.yml
  .github/workflows/repo-contract.yml
  .github/workflows/secret-scan.yml
  api/openapi.yaml
  api/oapi-codegen.yaml
  api/oapi-codegen-p1-candidate.yaml
  sqlc.yaml
  migrations/00001_bootstrap.sql
  migrations/00002_event_log.sql
  migrations/00003_settings.sql
  migrations/00004_auth.sql
  migrations/00005_contact_core.sql
  migrations/00006_customer_events.sql
  migrations/00007_contact_merge_lineage.sql
  migrations/00008_segment_contract.sql
  migrations/00009_segment_query_indexes.sql
  migrations/00018_segment_crud_receipts.sql
  migrations/00020_outbound_send_attempts.sql
  migrations/00021_outbound_task_status.sql
  migrations/00022_outbound_send_attempt_history.sql
  migrations/00023_outbound_cancel_control.sql
  migrations/00024_outbound_manual_retry_control.sql
  migrations/00025_automation_tag_trigger.sql
  migrations/00028_auth_wecom_corp_id.sql
  acceptance/auth/si00b_migration_compatibility.sh
  docs/execution/slices/P4-SI00B.md
  migrations/00029_product_catalog.sql
  acceptance/product/doc.go
  acceptance/product/i01a_integration_test.go
  acceptance/product/i01a_migration_compatibility.sh
  docs/execution/slices/P4-I01A.md
  internal/product/app/catalog.go
  internal/product/app/catalog_test.go
  internal/product/http/handler.go
  internal/product/port/port.go
  internal/product/store/catalog_repository.go
  internal/product/store/queries/catalog.sql
  internal/product/store/generated/catalog.sql.go
  internal/product/store/generated/db.go
  internal/product/store/generated/models.go
  internal/product/store/generated/querier.go
  migrations/00030_media_image_upload.sql
  acceptance/media/doc.go
  acceptance/media/h01a1_integration_test.go
  acceptance/media/h01a1_migration_compatibility.sh
  docs/execution/slices/P4-H01A1.md
  internal/media/app/upload.go
  internal/media/app/upload_test.go
  internal/media/domain/image.go
  internal/media/domain/image_test.go
  internal/media/port/port.go
  internal/media/store/upload_repository.go
  internal/media/store/queries/upload.sql
  internal/media/store/generated/upload.sql.go
  internal/media/store/generated/db.go
  internal/media/store/generated/models.go
  internal/media/store/generated/querier.go
  migrations/00031_survey_questionnaire_crud.sql
  acceptance/survey/doc.go
  acceptance/survey/f01a_integration_test.go
  acceptance/survey/f01a_migration_compatibility.sh
  docs/execution/slices/P4-F01A.md
  docs/execution/slices/P4-B01.md
  internal/survey/app/service.go
  internal/survey/app/service_test.go
  internal/survey/port/port.go
  internal/survey/store/questionnaire_repository.go
  internal/survey/store/queries/questionnaires.sql
  internal/survey/store/generated/questionnaires.sql.go
  internal/survey/store/generated/db.go
  internal/survey/store/generated/models.go
  internal/survey/store/generated/querier.go
  cmd/aicrm/legacy_survey_api.go
  cmd/aicrm/legacy_survey_api_test.go
  migrations/00032_contact_channel_catalog.sql
  acceptance/contact/c01_channel_integration_test.go
  acceptance/contact/c01_migration_compatibility.sh
  docs/execution/slices/P4-C01.md
  internal/contact/app/channel_catalog.go
  internal/contact/app/channel_catalog_test.go
  internal/contact/store/channel_repository.go
  internal/contact/store/queries/channels.sql
  internal/contact/store/generated/channels.sql.go
  cmd/aicrm/legacy_channel_api.go
  cmd/aicrm/legacy_channel_api_test.go
  migrations/00033_coupon_rules.sql
  acceptance/coupon/j01_integration_test.go
  acceptance/coupon/j01_migration_compatibility.sh
  docs/execution/slices/P4-J01.md
  internal/coupon/app/service.go
  internal/coupon/port/port.go
  internal/coupon/store/repository.go
  internal/coupon/store/queries/coupons.sql
  internal/coupon/store/generated/coupons.sql.go
  internal/coupon/store/generated/db.go
  internal/coupon/store/generated/models.go
  internal/coupon/store/generated/querier.go
  cmd/aicrm/legacy_coupon_api.go
  cmd/aicrm/legacy_coupon_api_test.go
  migrations/00034_media_group_invite_library.sql
  acceptance/media/h03_integration_test.go
  acceptance/media/h03_migration_compatibility.sh
  docs/execution/slices/P4-H03.md
  internal/media/app/group_invite.go
  internal/media/app/group_invite_test.go
  internal/media/domain/group_invite.go
  internal/media/domain/group_invite_test.go
  internal/media/store/group_invite_repository.go
  internal/media/store/queries/group_invites.sql
  internal/media/store/generated/group_invites.sql.go
  cmd/aicrm/legacy_group_invite_api.go
  cmd/aicrm/legacy_group_invite_api_test.go
  migrations/00035_order_list_projection.sql
  acceptance/order/doc.go
  acceptance/order/i03_integration_test.go
  acceptance/order/i03_migration_compatibility.sh
  docs/execution/slices/P4-I03.md
  internal/order/app/list.go
  internal/order/app/list_test.go
  internal/order/port/port.go
  internal/order/store/repository.go
  internal/order/store/queries/orders.sql
  internal/order/store/generated/orders.sql.go
  internal/order/store/generated/db.go
  internal/order/store/generated/models.go
  internal/order/store/generated/querier.go
  internal/contact/store/queries/order_read_port.sql
  internal/contact/store/generated/order_read_port.sql.go
  cmd/aicrm/legacy_order_api.go
  cmd/aicrm/legacy_order_api_test.go
  cmd/aicrm/legacy_api.go
  cmd/aicrm/legacy_media_api_test.go
  cmd/aicrm/legacy_product_api_test.go
  internal/automation/store/tag_trigger.go
  internal/automation/store/queries/tag_trigger.sql
  internal/automation/store/generated/db.go
  internal/automation/store/generated/models.go
  internal/automation/store/generated/querier.go
  internal/automation/store/generated/tag_trigger.sql.go
  acceptance/automation/d01_integration_test.go
  acceptance/automation/d01_migration_compatibility.sh
  cmd/aicrm/legacy_automation_api_test.go
  docs/execution/slices/P4-W0-D01.md
  internal/outbound/app/sender.go
  internal/outbound/app/sender_test.go
  internal/outbound/store/queries/send_attempts.sql
  internal/outbound/store/sender_repository.go
  internal/outbound/worker/sender.go
  internal/outbound/worker/sender_test.go
  acceptance/outbound/o4_sender_integration_test.go
  acceptance/outbound/o5_status_integration_test.go
  acceptance/outbound/o6a_retry_integration_test.go
  acceptance/outbound/o6a_migration_compatibility.sh
  acceptance/outbound/o6b1_cancel_integration_test.go
  acceptance/outbound/o6b1_migration_compatibility.sh
  acceptance/outbound/o6b2_manual_retry_integration_test.go
  acceptance/outbound/o6b2_migration_compatibility.sh
  docs/execution/slices/P3-O4.md
  docs/execution/slices/P3-O5.md
  docs/execution/slices/P3-O6A.md
  docs/execution/slices/P3-O6B1.md
  docs/execution/slices/P3-O6B2.md
  internal/outbound/app/control.go
  internal/outbound/app/control_test.go
  internal/outbound/app/manual_retry.go
  internal/outbound/app/manual_retry_test.go
  internal/outbound/store/control_repository.go
  internal/outbound/store/queries/control.sql
  internal/outbound/store/generated/control.sql.go
  internal/platform/jobqueue/outbound_cancel.go
  internal/segment/port/port.go
  internal/segment/port/port_test.go
  internal/segment/dsl/ast.go
  internal/segment/dsl/parser.go
  internal/segment/dsl/parser_test.go
  internal/segment/compiler/compiler.go
  internal/segment/compiler/compiler_test.go
  internal/segment/compiler/executor.go
  internal/segment/compiler/executor_test.go
  internal/segment/store/queries/audience.sql
  internal/segment/store/query_set.go
  acceptance/segment/query_set_integration_test.go
  acceptance/segment/doc.go
  internal/segment/app/refresh.go
  internal/segment/app/refresh_test.go
  internal/segment/app/cron.go
  internal/segment/app/cron_test.go
  internal/segment/store/queries/refresh.sql
  internal/segment/store/refresh_repository.go
  internal/segment/store/refresh_repository_test.go
  acceptance/segment/refresh_integration_test.go
  internal/segment/app/crud.go
  internal/segment/http/crud_handler.go
  internal/segment/store/crud_repository.go
  internal/segment/store/queries/crud.sql
  acceptance/segment/crud_integration_test.go
  docs/execution/slices/P3-S05B-R2.md
  internal/platform/http/contract.go
  internal/platform/runtime/contract.go
  internal/platform/store/contract.go
  internal/platform/river/contract.go
  web/index.html
  web/src/auth.ts
  web/src/auth.test.ts
  web/src/auth-ui.tsx
  web/src/auth-ui.test.tsx
  web/src/main.tsx
  web/src/main.test.tsx
  web/src/segments.ts
  web/src/segments.test.ts
  web/src/segments-ui.tsx
  web/src/segments-ui.test.tsx
  web/src/segments.css
  web/scripts/segments-browser-smoke.mjs
  web/src/identity-reviews.ts
  web/src/identity-reviews.test.ts
  web/src/identity-reviews-ui.tsx
  web/src/identity-reviews-ui.test.tsx
  web/src/identity-reviews.css
  web/scripts/identity-reviews-browser-smoke.mjs
  web/src/customers.ts
  web/src/customers.test.ts
  web/src/customers-ui.tsx
  web/src/customers-ui.test.tsx
  web/src/customers-list.css
  web/src/shell.css
  web/src/api/generated/health.ts
  acceptance/p0s01/runtime_contract_test.go
  acceptance/p0s01/process_blackbox.sh
  acceptance/p0s01/static_contract.sh
  acceptance/p0s02/health_contract_test.go
  acceptance/p0s02/static_contract.sh
  acceptance/p0s02/test_static_contract.sh
  acceptance/p0s03/query_contract_test.go
  acceptance/p0s03/source_contract.go
  acceptance/p0s03/static_contract.sh
  acceptance/p0s03/test_contract.sh
  acceptance/p0s04/contract_test.go
  acceptance/p0s04/source_contract.go
  acceptance/p0s04/test_source_contract.sh
  acceptance/p0s04/static_contract.sh
  acceptance/p0s04/test_static_contract.sh
  acceptance/p0s10/test_snapshot_gate.sh
  acceptance/snapshots/catalog.v1.json
  tools/snapshot-gate/main.go
  tools/snapshot-gate/main_test.go
  tools/legacy-route-export/main.go
  tools/legacy-route-export/main_test.go
  docs/execution/slices/P1-S01.md
  docs/evidence/p1/legacy-routes-6cb989c.json
  docs/feature-matrix.csv
  docs/evidence/p1/feature-matrix-id-anchor.v1
  docs/execution/slices/P1-S08.md
  scripts/check_feature_matrix_contract.sh
  docs/migration-mapping.md
  docs/migration-mapping.jsonl
  docs/evidence/p1/migration-lifecycle-index-6cb989c.json
  docs/execution/slices/P1-S09.md
  docs/execution/slices/P1-C02.md
  tools/migration-mapping/main.go
  tools/migration-mapping/main_test.go
  docs/api-mapping.jsonl
  docs/evidence/p1/route-triage.csv
  docs/evidence/p1/g1-decisions.md
  docs/evidence/p1/g1-signoff-pack.md
  docs/evidence/p1/feature-matrix-top20.md
  docs/evidence/p1/migration-exceptions.md
  docs/execution/slices/G1-D01.md
  docs/execution/slices/G1-D02.md
  docs/spec/AI-CRM-v2-P2P3执行计划.md
  docs/backlog/post-launch.md
  docs/execution/slices/SEC-01.md
  tools/p1-reconciliation/main.go
  tools/p1-reconciliation/main_test.go
  docs/execution/slices/P1-C03.md
  tools/openapi-contract/main.go
  tools/openapi-contract/main_test.go
  acceptance/p1s11/contracts_test.go
  acceptance/p1s11/doc.go
  acceptance/p3c00/contracts_test.go
  acceptance/p3c00/doc.go
  docs/execution/slices/P3-C00.md
  acceptance/p3c01a/doc.go
  acceptance/p3c01a/schema_integration_test.go
  internal/contact/acceptance/doc.go
  internal/contact/acceptance/schema_contract_test.go
  internal/contact/app/customer_query_contract.go
  internal/contact/app/customer_list_service.go
  internal/contact/app/customer_list_service_test.go
  docs/execution/slices/P3-C01A.md
  internal/contact/http/customer_list_handler.go
  internal/contact/http/customer_list_handler_test.go
  docs/execution/slices/P3-C01D.md
  docs/evidence/slices/P3-C01D-customer-handler-tests.md
  docs/execution/slices/P3-C04.md
  docs/evidence/slices/P3-C04-customer-list-ui.md
  acceptance/p3c02a/doc.go
  acceptance/p3c02a/customer_mutation_integration_test.go
  internal/contact/app/customer_mutation_service.go
  internal/contact/app/customer_mutation_service_test.go
  internal/contact/store/customer_mutation_repository.go
  internal/contact/store/customer_mutation_repository_test.go
  internal/contact/store/queries/customer_mutations.sql
  internal/contact/store/generated/customer_mutations.sql.go
  docs/execution/slices/P3-C02A.md
  docs/evidence/slices/P3-C02A-sqlc-store.md
  docs/evidence/slices/P3-C02A-service-tests.md
  internal/contact/http/customer_mutation_handler.go
  internal/contact/http/customer_mutation_handler_test.go
  docs/execution/slices/P3-C02C.md
  docs/evidence/slices/P3-C02C-handler-tests.md
  docs/evidence/slices/P3-C02C-service-tests.md
  docs/evidence/slices/P3-C02C-store-tests.md
  acceptance/p3c02b/doc.go
  acceptance/p3c02b/customer_detail_integration_test.go
  internal/contact/app/customer_detail_service.go
  internal/contact/app/customer_detail_service_test.go
  internal/contact/http/customer_detail_handler.go
  internal/contact/http/customer_detail_handler_test.go
  internal/contact/store/customer_detail_repository.go
  internal/contact/store/customer_detail_repository_test.go
  internal/contact/store/queries/customer_detail.sql
  internal/contact/store/generated/customer_detail.sql.go
  docs/execution/slices/P3-C02B.md
  docs/evidence/slices/P3-C02B-sqlc-store.md
  docs/evidence/slices/P3-C02B-service-tests.md
  docs/evidence/slices/P3-C02B-handler-tests.md
  acceptance/p3c02d/doc.go
  acceptance/p3c02d/customer_event_integration_test.go
  internal/contact/app/customer_event_service.go
  internal/contact/app/customer_event_service_test.go
  internal/contact/http/customer_event_handler.go
  internal/contact/http/customer_event_handler_test.go
  internal/contact/store/customer_event_repository.go
  internal/contact/store/customer_event_repository_test.go
  internal/contact/store/queries/customer_events.sql
  internal/contact/store/generated/customer_events.sql.go
  docs/execution/slices/P3-C02D.md
  docs/evidence/slices/P3-C02D-sqlc-store.md
  docs/evidence/slices/P3-C02D-service-tests.md
  docs/evidence/slices/P3-C02D-handler-tests.md
  acceptance/p3c02e/doc.go
  acceptance/p3c02e/tag_catalog_integration_test.go
  internal/contact/app/tag_catalog_service.go
  internal/contact/app/tag_catalog_service_test.go
  internal/contact/http/tag_catalog_handler.go
  internal/contact/http/tag_catalog_handler_test.go
  internal/contact/store/tag_catalog_repository.go
  internal/contact/store/tag_catalog_repository_test.go
  internal/contact/store/queries/tags.sql
  internal/contact/store/generated/tags.sql.go
  docs/execution/slices/P3-C02E.md
  docs/evidence/slices/P3-C02E-sqlc-store.md
  docs/evidence/slices/P3-C02E-service-tests.md
  docs/evidence/slices/P3-C02E-handler-tests.md
  web/src/customer-detail.ts
  web/src/customer-detail.test.ts
  web/src/customer-detail-ui.tsx
  web/src/customer-detail-ui.test.tsx
  web/src/customer-detail.css
  docs/execution/slices/P3-C05.md
  docs/evidence/slices/P3-C05-ui.md
  docs/evidence/slices/P3-C05-route-tests.md
  cmd/aicrm-contact-perf-data/main.go
  cmd/aicrm-contact-perf-data/main_test.go
  docs/execution/slices/P3-C06.md
  docs/evidence/slices/P3-C06-synthetic-data.md
  cmd/aicrm-contact-perf/main.go
  cmd/aicrm-contact-perf/main_test.go
  docs/execution/slices/P3-C06A2.md
  docs/evidence/slices/P3-C06A2-runner.md
  docs/execution/slices/P3-C06C.md
  docs/evidence/slices/P3-C06C-query-optimization.md
  docs/execution/slices/P3-C06D.md
  docs/evidence/slices/P3-C06D-tag-count.md
  docs/execution/slices/P3-I00.md
  acceptance/fixtures/cmd/validate-database-url/main.go
  acceptance/contact/doc.go
  acceptance/contact/partition_integration_test.go
  internal/contact/store/event_partitions.go
  internal/contact/store/event_partitions_test.go
  internal/contact/store/queries/event_partitions.sql
  internal/contact/store/generated/event_partitions.sql.go
  internal/contact/worker/event_partitions.go
  internal/contact/worker/event_partitions_test.go
  docs/execution/slices/P3-C03.md
  docs/evidence/slices/P3-C03-partition-worker-tests.md
  acceptance/contact/merge_lineage_integration_test.go
  acceptance/contact/lineage_timeline_integration_test.go
  acceptance/contact/lineage_timeline_plan_integration_test.go
  internal/contact/store/merge_port_repository.go
  internal/contact/store/merge_port_repository_test.go
  internal/contact/store/external_event_repository_test.go
  internal/contact/store/queries/external_events.sql
  internal/contact/store/generated/external_events.sql.go
  docs/execution/slices/P3-C07.md
  docs/execution/slices/P3-C07A.md
  docs/execution/slices/P3-C07B1.md
  docs/execution/slices/P3-C07B2.md
  acceptance/fixtures/postgres.go
  acceptance/fixtures/postgres_test.go
  docs/execution/slices/P2-00.md
  docs/execution/slices/P2-01R.md
  docs/execution/slices/P2-02.md
  docs/execution/slices/P2-03.md
  docs/execution/slices/P2-04.md
  docs/execution/slices/P2-05.md
  docs/execution/slices/P2-07.md
  docs/evidence/slices/P2-07-dispatcher-tests.md
  docs/evidence/slices/P2-04-queue-policy-tests.md
  docs/execution/slices/P2-06.md
  docs/evidence/slices/P2-03-registry-tests.md
  docs/execution/implementation-plan.md
  docs/execution/slices/P1-S11.md
  internal/api/generated/server.gen.go
  internal/api/candidate/generated/server.gen.go
  internal/auth/port/port.go
  internal/auth/port/port_test.go
  internal/contact/port/port.go
  internal/events/port/port.go
  internal/events/port/port_test.go
  internal/events/store/appender.go
  internal/events/store/appender_test.go
  internal/events/store/queries/event_log.sql
  internal/events/store/generated/db.go
  internal/events/store/generated/event_log.sql.go
  internal/events/store/generated/models.go
  internal/events/store/generated/querier.go
  internal/identity/port/port.go
  internal/identity/port/port_test.go
  internal/platform/port/uow.go
  internal/platform/store/uow.go
  internal/platform/store/uow_test.go
  cmd/aicrm/main.go
  cmd/aicrm/components.go
  cmd/aicrm/components_test.go
  cmd/aicrm/scheduler.go
  cmd/aicrm/scheduler_test.go
  internal/config/load.go
  internal/config/schema.go
  internal/config/schema_test.go
  internal/config/port/port.go
  internal/config/registry.go
  internal/config/registry_test.go
  internal/config/app/manager.go
  internal/config/app/manager_test.go
  internal/config/store/repository.go
  internal/config/store/queries/settings.sql
  internal/config/store/generated/db.go
  internal/config/store/generated/models.go
  internal/config/store/generated/querier.go
  internal/config/store/generated/settings.sql.go
  acceptance/p2s01r/doc.go
  acceptance/p2s01r/uow_integration_test.go
  acceptance/p2s06/doc.go
  acceptance/p2s06/event_log_integration_test.go
  acceptance/p2s03/doc.go
  acceptance/p2s03/settings_integration_test.go
  acceptance/p2s04/doc.go
  acceptance/p2s04/queue_isolation_integration_test.go
  internal/platform/jobqueue/client.go
  internal/platform/jobqueue/client_test.go
  internal/platform/jobqueue/queue.go
  internal/platform/jobqueue/queue_policy_test.go
  internal/platform/scheduler/scheduler.go
  internal/platform/scheduler/scheduler_test.go
  acceptance/p2s05/doc.go
  acceptance/p2s05/scheduler_integration_test.go
  acceptance/p2s07/doc.go
  acceptance/p2s07/dispatcher_integration_test.go
  internal/events/dispatcher/dispatcher.go
  internal/events/dispatcher/dispatcher_test.go
  internal/events/dispatcher/jobs.go
  acceptance/p2s08/doc.go
  acceptance/p2s08/gateway_blackbox_test.go
  internal/platform/http/errors.go
  internal/platform/http/gateway.go
  internal/platform/http/gateway_test.go
  docs/execution/slices/P2-08.md
  docs/evidence/slices/P2-08-http-tests.md
  acceptance/p2s09/doc.go
  acceptance/p2s09/session_integration_test.go
  internal/auth/app/service.go
  internal/auth/app/service_test.go
  internal/auth/http/handler.go
  internal/auth/store/repository.go
  internal/auth/store/queries/auth.sql
  internal/auth/store/generated/auth.sql.go
  internal/auth/store/generated/db.go
  internal/auth/store/generated/models.go
  internal/auth/store/generated/querier.go
  docs/execution/slices/P2-09.md
  docs/evidence/slices/P2-09-auth-service-tests.md
  acceptance/p2s10/doc.go
  acceptance/p2s10/rbac_contract_test.go
  internal/auth/app/policy.go
  internal/auth/app/policy_test.go
  internal/auth/http/authorization.go
  docs/execution/slices/P2-10.md
  docs/evidence/slices/P2-10-rbac-tests.md
  cmd/aicrm/api.go
  cmd/aicrm/api_test.go
  acceptance/p2s11/doc.go
  acceptance/p2s11/gateway_router_test.go
  docs/execution/slices/P2-11.md
  docs/evidence/slices/P2-11-gateway-tests.md
  docs/execution/slices/P2-12.md
  docs/evidence/slices/P2-12-web-shell.md
  docs/execution/slices/P2-13.md
  docs/evidence/slices/P2-13-auth-ui.md
  docs/execution/slices/P2-14.md
  docs/evidence/slices/P2-14-stages-sqlc.md
  docs/execution/slices/P2-15.md
  docs/evidence/slices/P2-15-stage-service-tests.md
  docs/execution/slices/P2-16.md
  docs/evidence/slices/P2-16-stage-handler-tests.md
  docs/execution/slices/P2-17.md
  docs/evidence/slices/P2-17-stages-ui.md
  docs/execution/slices/P2-18.md
  docs/evidence/slices/P2-18-tier-config.md
  docs/execution/slices/G2-T03.md
  docs/evidence/g2/test-server-deployment.md
  docs/evidence/phases/P2-closeout.md
  internal/platform/deployment/tier.go
  internal/platform/deployment/tier_test.go
  cmd/aicrm-config/main.go
  cmd/aicrm-config/main_test.go
  deploy/compose.yml
  scripts/staging_deploy.sh
  acceptance/p2s18/test_tier_config.sh
  web/src/stages.ts
  web/src/stages.test.ts
  web/src/stages-ui.tsx
  web/src/stages-ui.test.tsx
  internal/auth/port/port.go
  internal/auth/http/authorization_test.go
  internal/contact/http/handler.go
  internal/contact/http/handler_test.go
  acceptance/p2s16/doc.go
  acceptance/p2s16/csrf_integration_test.go
  acceptance/p2s16/snapshot.go
  acceptance/p2s16/snapshot_test.go
  acceptance/p2s16/snapshotgen/main.go
  internal/contact/app/stage_service.go
  internal/contact/app/stage_service_test.go
  acceptance/p2s15/doc.go
  acceptance/p2s15/stage_service_integration_test.go
  internal/contact/store/queries/stages.sql
  internal/contact/store/generated/db.go
  internal/contact/store/generated/customers.sql.go
  internal/contact/store/generated/models.go
  internal/contact/store/generated/querier.go
  internal/contact/store/generated/stages.sql.go
  internal/contact/store/repository.go
  acceptance/p2s14/doc.go
  acceptance/p2s14/stages_store_integration_test.go
  acceptance/contactfixture/contactfixture.go
  acceptance/contactfixture/contactfixture_test.go
  acceptance/identity/doc.go
  acceptance/identity/contactfixture_import_test.go
  tools/query-plan-gate/main.go
  tools/query-plan-gate/main_test.go
  scripts/build_slice_bundle.sh
  scripts/check_arch_imports.go
  scripts/ownership/main.go scripts/test_ownership.sh
  scripts/sourcepolicy/main.go scripts/test_source_policy.sh
  scripts/check_slice_inputs.sh scripts/test_slice_inputs.sh
  scripts/check_slice_ledger_history.rb
  scripts/check_generated_sources.sh
  scripts/ci_promotion_decision.sh
  scripts/check_ci_promotion_smoke.sh
  scripts/test_ci_promotion_decision.sh
  scripts/check_repo_contract.sh
  scripts/verify_repo_receipts.pl
  scripts/generated-sources.sha256
  scripts/scan_sensitive_paths.sh
  scripts/test_gitleaks_config.sh
  scripts/test_build_slice_bundle.sh
  scripts/test_gitless_generated_check.sh
  scripts/test_orval_generated_check.sh
  scripts/package_release_archive.sh
  scripts/test_package_release_archive.sh
  scripts/test_arch_imports.sh
  scripts/test_query_plan_gate.sh
  scripts/test_repo_contract.sh
  docs/architecture/canonical.md
  docs/adr/ADR-001.md
  docs/adr/ADR-010.md
  docs/adr/ADR-011.md
  docs/adr/ADR-012.md
  docs/adr/ADR-013.md
  docs/architecture/port-contracts.md
  docs/architecture/table-ownership.yml
  docs/governance/agent-orchestration.md
  docs/governance/limitations.md
  docs/plans/2026-08-13-p4-legacy-backend-parity.md
  docs/execution/slice-card-template.md
  docs/execution/slice-ledger.yml
  docs/execution/slices/P3-S06.md
  docs/execution/slices/P3-I8.md
  docs/execution/slices/P0-S02.md
  docs/execution/slices/P0-S03.md
  docs/execution/slices/P0-S04.md
  docs/execution/slices/P3-S00.md
  docs/execution/slices/P3-S01.md
  docs/execution/slices/P3-S02.md
  docs/execution/slices/P3-S03.md
  docs/execution/slices/P3-S04A.md
  docs/execution/slices/P3-S04B.md
  docs/execution/slices/P3-R3A.md
  docs/execution/slices/P3-R4A.md
  docs/execution/slices/P3-R4B.md
  docs/execution/slices/P3-I3R0.md
  docs/execution/slices/P3-I4A.md
  docs/execution/slices/P3-I4B.md
  docs/execution/slices/P3-C07C-R3B.md
  docs/execution/slices/P3-C07C-R3C.md
  docs/execution/slices/M0-1.md
  docs/execution/slices/M0-2.md
  docs/execution/slices/M0-3.md
  docs/execution/slices/M0-5.md
  docs/execution/slices/M0-6.md
  docs/execution/slices/M0-7.md
  docs/spec/AI-CRM-v2-执行方案.md
  docs/spec/AI-CRM-v2-执行方案-v2-至P3.md
  docs/spec/AI-CRM-v2-重构详细设计.md
  docs/spec/SHA256SUMS
  migrations/00010_identity_storage.sql
  migrations/00011_contact_external_event_idempotency.sql
  migrations/00013_identity_receipt_completion_transaction.sql
  migrations/00015_wecom_sync_state.sql
  acceptance/identity/storage_integration_test.go
  acceptance/contact/external_event_storage_integration_test.go
  acceptance/contact/external_event_behavior_integration_test.go
  acceptance/wecom/doc.go
  acceptance/wecom/sync_resume_integration_test.go
  docs/execution/slices/P3-W4.md
  internal/wecom/app/identity_contact.go
  internal/wecom/app/identity_contact_test.go
  docs/execution/slices/P3-W5.md
)

for file_path in "${required[@]}"; do
  [[ -f "$file_path" ]] || fail "missing required file: $file_path"
done
scripts/verify_repo_receipts.pl required "${required[@]}"

mode_arguments=()
while IFS=' ' read -r expected file_path; do
  mode_arguments+=("$expected" "$file_path")
done <<'EOF'
100644 Makefile
100644 CONTRIBUTING.md
100644 go.mod
100644 go.sum
100644 tools/go.mod
100644 tools/go.sum
100644 package.json
100644 package-lock.json
100644 .gitleaks.toml
100644 web/index.html
100644 web/src/main.tsx
100644 web/src/main.test.tsx
100644 web/src/segments.ts
100644 web/src/segments.test.ts
100644 web/src/segments-ui.tsx
100644 web/src/segments-ui.test.tsx
100644 web/src/segments.css
100644 web/scripts/segments-browser-smoke.mjs
100644 web/src/identity-reviews.ts
100644 web/src/identity-reviews.test.ts
100644 web/src/identity-reviews-ui.tsx
100644 web/src/identity-reviews-ui.test.tsx
100644 web/src/identity-reviews.css
100644 web/scripts/identity-reviews-browser-smoke.mjs
100644 web/src/customers.ts
100644 web/src/customers.test.ts
100644 web/src/customers-ui.tsx
100644 web/src/customers-ui.test.tsx
100644 web/src/customers-list.css
100644 web/src/shell.css
100644 web/src/api/generated/health.ts
100644 .github/workflows/application-go.yml
100755 scripts/ci_promotion_decision.sh
100755 scripts/check_ci_promotion_smoke.sh
100755 scripts/test_ci_promotion_decision.sh
100755 scripts/check_repo_contract.sh
100755 scripts/verify_repo_receipts.pl
100644 scripts/check_arch_imports.go
100755 scripts/test_arch_imports.sh
100644 scripts/ownership/main.go
100755 scripts/test_ownership.sh
100644 scripts/sourcepolicy/main.go
100755 scripts/test_source_policy.sh
100755 scripts/test_gitless_generated_check.sh
100755 scripts/package_release_archive.sh
100755 scripts/test_package_release_archive.sh
100755 scripts/check_slice_inputs.sh
100755 scripts/test_slice_inputs.sh
100755 scripts/check_slice_ledger_history.rb
100755 acceptance/outbound/o6a_migration_compatibility.sh
100755 acceptance/outbound/o6b1_migration_compatibility.sh
100755 acceptance/outbound/o6b2_migration_compatibility.sh
100644 migrations/00025_automation_tag_trigger.sql
100644 migrations/00028_auth_wecom_corp_id.sql
100755 acceptance/auth/si00b_migration_compatibility.sh
100644 docs/execution/slices/P4-SI00B.md
100644 migrations/00029_product_catalog.sql
100755 acceptance/product/i01a_migration_compatibility.sh
100644 docs/execution/slices/P4-I01A.md
100644 migrations/00030_media_image_upload.sql
100644 acceptance/media/doc.go
100644 acceptance/media/h01a1_integration_test.go
100755 acceptance/media/h01a1_migration_compatibility.sh
100644 docs/execution/slices/P4-H01A1.md
100644 internal/media/app/upload.go
100644 internal/media/app/upload_test.go
100644 internal/media/domain/image.go
100644 internal/media/domain/image_test.go
100644 internal/media/port/port.go
100644 internal/media/store/upload_repository.go
100644 internal/media/store/queries/upload.sql
100644 internal/media/store/generated/upload.sql.go
100644 internal/media/store/generated/db.go
100644 internal/media/store/generated/models.go
100644 internal/media/store/generated/querier.go
100644 migrations/00031_survey_questionnaire_crud.sql
100644 acceptance/survey/doc.go
100644 acceptance/survey/f01a_integration_test.go
100755 acceptance/survey/f01a_migration_compatibility.sh
100644 docs/execution/slices/P4-F01A.md
100644 docs/execution/slices/P4-B01.md
100644 internal/survey/app/service.go
100644 internal/survey/app/service_test.go
100644 internal/survey/port/port.go
100644 internal/survey/store/questionnaire_repository.go
100644 internal/survey/store/queries/questionnaires.sql
100644 internal/survey/store/generated/questionnaires.sql.go
100644 internal/survey/store/generated/db.go
100644 internal/survey/store/generated/models.go
100644 internal/survey/store/generated/querier.go
100644 cmd/aicrm/legacy_survey_api.go
100644 cmd/aicrm/legacy_survey_api_test.go
100644 migrations/00032_contact_channel_catalog.sql
100644 acceptance/contact/c01_channel_integration_test.go
100755 acceptance/contact/c01_migration_compatibility.sh
100644 docs/execution/slices/P4-C01.md
100644 internal/contact/app/channel_catalog.go
100644 internal/contact/app/channel_catalog_test.go
100644 internal/contact/store/channel_repository.go
100644 internal/contact/store/queries/channels.sql
100644 internal/contact/store/generated/channels.sql.go
100644 cmd/aicrm/legacy_channel_api.go
100644 cmd/aicrm/legacy_channel_api_test.go
100644 migrations/00033_coupon_rules.sql
100644 acceptance/coupon/j01_integration_test.go
100755 acceptance/coupon/j01_migration_compatibility.sh
100644 docs/execution/slices/P4-J01.md
100644 internal/coupon/app/service.go
100644 internal/coupon/port/port.go
100644 internal/coupon/store/repository.go
100644 internal/coupon/store/queries/coupons.sql
100644 internal/coupon/store/generated/coupons.sql.go
100644 internal/coupon/store/generated/db.go
100644 internal/coupon/store/generated/models.go
100644 internal/coupon/store/generated/querier.go
100644 cmd/aicrm/legacy_coupon_api.go
100644 cmd/aicrm/legacy_coupon_api_test.go
100644 migrations/00034_media_group_invite_library.sql
100644 acceptance/media/h03_integration_test.go
100755 acceptance/media/h03_migration_compatibility.sh
100644 docs/execution/slices/P4-H03.md
100644 internal/media/app/group_invite.go
100644 internal/media/app/group_invite_test.go
100644 internal/media/domain/group_invite.go
100644 internal/media/domain/group_invite_test.go
100644 internal/media/store/group_invite_repository.go
100644 internal/media/store/queries/group_invites.sql
100644 internal/media/store/generated/group_invites.sql.go
100644 cmd/aicrm/legacy_group_invite_api.go
100644 cmd/aicrm/legacy_group_invite_api_test.go
100644 migrations/00035_order_list_projection.sql
100644 acceptance/order/doc.go
100644 acceptance/order/i03_integration_test.go
100755 acceptance/order/i03_migration_compatibility.sh
100644 docs/execution/slices/P4-I03.md
100644 internal/order/app/list.go
100644 internal/order/app/list_test.go
100644 internal/order/port/port.go
100644 internal/order/store/repository.go
100644 internal/order/store/queries/orders.sql
100644 internal/order/store/generated/orders.sql.go
100644 internal/order/store/generated/db.go
100644 internal/order/store/generated/models.go
100644 internal/order/store/generated/querier.go
100644 internal/contact/store/queries/order_read_port.sql
100644 internal/contact/store/generated/order_read_port.sql.go
100644 cmd/aicrm/legacy_order_api.go
100644 cmd/aicrm/legacy_order_api_test.go
100644 cmd/aicrm/legacy_media_api_test.go
100644 internal/automation/store/tag_trigger.go
100644 internal/automation/store/queries/tag_trigger.sql
100644 internal/automation/store/generated/db.go
100644 internal/automation/store/generated/models.go
100644 internal/automation/store/generated/querier.go
100644 internal/automation/store/generated/tag_trigger.sql.go
100644 acceptance/automation/d01_integration_test.go
100755 acceptance/automation/d01_migration_compatibility.sh
100644 cmd/aicrm/legacy_automation_api_test.go
100644 docs/execution/slices/P4-W0-D01.md
100644 tools/snapshot-gate/main.go
100644 tools/snapshot-gate/main_test.go
100644 acceptance/snapshots/catalog.v1.json
100644 tools/legacy-route-export/main.go
100644 tools/legacy-route-export/main_test.go
100644 docs/execution/slices/P1-S01.md
100644 docs/evidence/p1/legacy-routes-6cb989c.json
100644 docs/feature-matrix.csv
100644 docs/evidence/p1/feature-matrix-id-anchor.v1
100644 docs/execution/slices/P1-S08.md
100755 scripts/check_feature_matrix_contract.sh
100644 docs/migration-mapping.md
100644 docs/migration-mapping.jsonl
100644 docs/evidence/p1/migration-lifecycle-index-6cb989c.json
100644 docs/execution/slices/P1-S09.md
100644 docs/execution/slices/P1-C02.md
100644 tools/migration-mapping/main.go
100644 tools/migration-mapping/main_test.go
100644 docs/api-mapping.jsonl
100644 docs/evidence/p1/route-triage.csv
100644 docs/evidence/p1/g1-decisions.md
100644 docs/execution/slices/G1-D01.md
100644 tools/p1-reconciliation/main.go
100644 tools/p1-reconciliation/main_test.go
100644 docs/execution/slices/P1-C03.md
100644 api/openapi.yaml
100644 api/oapi-codegen.yaml
100644 api/oapi-codegen-p1-candidate.yaml
100644 internal/api/generated/server.gen.go
100644 internal/api/candidate/generated/server.gen.go
100644 tools/openapi-contract/main.go
100644 tools/openapi-contract/main_test.go
100644 acceptance/p1s11/contracts_test.go
100644 acceptance/p1s11/doc.go
100644 acceptance/p3c00/contracts_test.go
100644 acceptance/p3c00/doc.go
100644 docs/execution/slices/P3-C00.md
100644 acceptance/p3c01a/doc.go
100644 acceptance/p3c01a/schema_integration_test.go
100644 internal/contact/acceptance/doc.go
100644 internal/contact/acceptance/schema_contract_test.go
100644 internal/contact/app/customer_query_contract.go
100644 internal/contact/app/customer_list_service.go
100644 internal/contact/app/customer_list_service_test.go
100644 docs/execution/slices/P3-C01A.md
100644 internal/contact/http/customer_list_handler.go
100644 internal/contact/http/customer_list_handler_test.go
100644 docs/execution/slices/P3-C01D.md
100644 docs/evidence/slices/P3-C01D-customer-handler-tests.md
100644 docs/execution/slices/P3-C04.md
100644 docs/evidence/slices/P3-C04-customer-list-ui.md
100644 acceptance/p3c02a/doc.go
100644 acceptance/p3c02a/customer_mutation_integration_test.go
100644 internal/contact/app/customer_mutation_service.go
100644 internal/contact/app/customer_mutation_service_test.go
100644 internal/contact/store/customer_mutation_repository.go
100644 internal/contact/store/customer_mutation_repository_test.go
100644 internal/contact/store/queries/customer_mutations.sql
100644 internal/contact/store/generated/customer_mutations.sql.go
100644 docs/execution/slices/P3-C02A.md
100644 docs/evidence/slices/P3-C02A-sqlc-store.md
100644 docs/evidence/slices/P3-C02A-service-tests.md
100644 internal/contact/http/customer_mutation_handler.go
100644 internal/contact/http/customer_mutation_handler_test.go
100644 docs/execution/slices/P3-C02C.md
100644 docs/evidence/slices/P3-C02C-handler-tests.md
100644 docs/evidence/slices/P3-C02C-service-tests.md
100644 docs/evidence/slices/P3-C02C-store-tests.md
100644 acceptance/p3c02b/doc.go
100644 acceptance/p3c02b/customer_detail_integration_test.go
100644 internal/contact/app/customer_detail_service.go
100644 internal/contact/app/customer_detail_service_test.go
100644 internal/contact/http/customer_detail_handler.go
100644 internal/contact/http/customer_detail_handler_test.go
100644 internal/contact/store/customer_detail_repository.go
100644 internal/contact/store/customer_detail_repository_test.go
100644 internal/contact/store/queries/customer_detail.sql
100644 internal/contact/store/generated/customer_detail.sql.go
100644 docs/execution/slices/P3-C02B.md
100644 docs/evidence/slices/P3-C02B-sqlc-store.md
100644 docs/evidence/slices/P3-C02B-service-tests.md
100644 docs/evidence/slices/P3-C02B-handler-tests.md
100644 acceptance/p3c02d/doc.go
100644 acceptance/p3c02d/customer_event_integration_test.go
100644 internal/contact/app/customer_event_service.go
100644 internal/contact/app/customer_event_service_test.go
100644 internal/contact/http/customer_event_handler.go
100644 internal/contact/http/customer_event_handler_test.go
100644 internal/contact/store/customer_event_repository.go
100644 internal/contact/store/customer_event_repository_test.go
100644 internal/contact/store/queries/customer_events.sql
100644 internal/contact/store/generated/customer_events.sql.go
100644 docs/execution/slices/P3-C02D.md
100644 docs/evidence/slices/P3-C02D-sqlc-store.md
100644 docs/evidence/slices/P3-C02D-service-tests.md
100644 docs/evidence/slices/P3-C02D-handler-tests.md
100644 acceptance/p3c02e/doc.go
100644 acceptance/p3c02e/tag_catalog_integration_test.go
100644 internal/contact/app/tag_catalog_service.go
100644 internal/contact/app/tag_catalog_service_test.go
100644 internal/contact/http/tag_catalog_handler.go
100644 internal/contact/http/tag_catalog_handler_test.go
100644 internal/contact/store/tag_catalog_repository.go
100644 internal/contact/store/tag_catalog_repository_test.go
100644 internal/contact/store/queries/tags.sql
100644 internal/contact/store/generated/tags.sql.go
100644 docs/execution/slices/P3-C02E.md
100644 docs/evidence/slices/P3-C02E-sqlc-store.md
100644 docs/evidence/slices/P3-C02E-service-tests.md
100644 docs/evidence/slices/P3-C02E-handler-tests.md
100644 web/src/customer-detail.ts
100644 web/src/customer-detail.test.ts
100644 web/src/customer-detail-ui.tsx
100644 web/src/customer-detail-ui.test.tsx
100644 web/src/customer-detail.css
100644 docs/execution/slices/P3-C05.md
100644 docs/evidence/slices/P3-C05-ui.md
100644 docs/evidence/slices/P3-C05-route-tests.md
100644 cmd/aicrm-contact-perf-data/main.go
100644 cmd/aicrm-contact-perf-data/main_test.go
100644 docs/execution/slices/P3-C06.md
100644 docs/evidence/slices/P3-C06-synthetic-data.md
100644 cmd/aicrm-contact-perf/main.go
100644 cmd/aicrm-contact-perf/main_test.go
100644 docs/execution/slices/P3-C06A2.md
100644 docs/evidence/slices/P3-C06A2-runner.md
100644 docs/execution/slices/P3-C06C.md
100644 docs/evidence/slices/P3-C06C-query-optimization.md
100644 docs/execution/slices/P3-C06D.md
100644 docs/evidence/slices/P3-C06D-tag-count.md
100644 docs/execution/slices/P3-I00.md
100644 acceptance/fixtures/cmd/validate-database-url/main.go
100644 acceptance/contact/doc.go
100644 acceptance/contact/partition_integration_test.go
100644 internal/contact/store/event_partitions.go
100644 internal/contact/store/event_partitions_test.go
100644 internal/contact/store/queries/event_partitions.sql
100644 internal/contact/store/generated/event_partitions.sql.go
100644 internal/contact/worker/event_partitions.go
100644 internal/contact/worker/event_partitions_test.go
100644 docs/execution/slices/P3-C03.md
100644 docs/evidence/slices/P3-C03-partition-worker-tests.md
100644 acceptance/contact/merge_lineage_integration_test.go
100644 acceptance/contact/lineage_timeline_integration_test.go
100644 acceptance/contact/lineage_timeline_plan_integration_test.go
100644 internal/contact/store/merge_port_repository.go
100644 internal/contact/store/merge_port_repository_test.go
100644 internal/contact/store/external_event_repository_test.go
100644 internal/contact/store/queries/external_events.sql
100644 internal/contact/store/generated/external_events.sql.go
100644 docs/execution/slices/P3-C07.md
100644 docs/execution/slices/P3-C07A.md
100644 docs/execution/slices/P3-C07B1.md
100644 docs/execution/slices/P3-C07B2.md
100644 acceptance/fixtures/postgres.go
100644 acceptance/fixtures/postgres_test.go
100644 docs/execution/slices/P2-00.md
100644 docs/execution/slices/P2-01R.md
100644 docs/execution/slices/P2-02.md
100644 docs/execution/slices/P2-03.md
100644 docs/execution/slices/P2-04.md
100644 docs/execution/slices/P2-05.md
100644 docs/execution/slices/P2-07.md
100644 docs/evidence/slices/P2-07-dispatcher-tests.md
100644 docs/evidence/slices/P2-04-queue-policy-tests.md
100644 docs/execution/slices/P2-06.md
100644 docs/evidence/slices/P2-03-registry-tests.md
100644 docs/execution/implementation-plan.md
100644 docs/execution/slices/P1-S11.md
100644 docs/backlog/post-launch.md
100644 docs/execution/slices/SEC-01.md
100644 internal/auth/port/port.go
100644 internal/auth/port/port_test.go
100644 internal/contact/port/port.go
100644 internal/events/port/port.go
100644 internal/events/port/port_test.go
100644 internal/events/store/appender.go
100644 internal/events/store/appender_test.go
100644 internal/events/store/queries/event_log.sql
100644 internal/events/store/generated/db.go
100644 internal/events/store/generated/event_log.sql.go
100644 internal/events/store/generated/models.go
100644 internal/events/store/generated/querier.go
100644 internal/identity/port/port.go
100644 internal/identity/port/port_test.go
100644 internal/platform/port/uow.go
100644 internal/platform/store/uow.go
100644 internal/platform/store/uow_test.go
100644 cmd/aicrm/main.go
100644 cmd/aicrm/components.go
100644 cmd/aicrm/components_test.go
100644 cmd/aicrm/scheduler.go
100644 cmd/aicrm/scheduler_test.go
100644 internal/config/load.go
100644 internal/config/schema.go
100644 internal/config/schema_test.go
100644 internal/config/port/port.go
100644 internal/config/registry.go
100644 internal/config/registry_test.go
100644 internal/config/app/manager.go
100644 internal/config/app/manager_test.go
100644 internal/config/store/repository.go
100644 internal/config/store/queries/settings.sql
100644 internal/config/store/generated/db.go
100644 internal/config/store/generated/models.go
100644 internal/config/store/generated/querier.go
100644 internal/config/store/generated/settings.sql.go
100644 acceptance/p2s01r/doc.go
100644 acceptance/p2s01r/uow_integration_test.go
100644 acceptance/p2s06/doc.go
100644 acceptance/p2s06/event_log_integration_test.go
100644 acceptance/p2s03/doc.go
100644 acceptance/p2s03/settings_integration_test.go
100644 acceptance/p2s04/doc.go
100644 acceptance/p2s04/queue_isolation_integration_test.go
100644 internal/platform/jobqueue/client.go
100644 internal/platform/jobqueue/client_test.go
100644 internal/platform/jobqueue/queue.go
100644 internal/platform/jobqueue/queue_policy_test.go
100644 internal/platform/scheduler/scheduler.go
100644 internal/platform/scheduler/scheduler_test.go
100644 acceptance/p2s05/doc.go
100644 acceptance/p2s05/scheduler_integration_test.go
100644 acceptance/p2s07/doc.go
100644 acceptance/p2s07/dispatcher_integration_test.go
100644 internal/events/dispatcher/dispatcher.go
100644 internal/events/dispatcher/dispatcher_test.go
100644 internal/events/dispatcher/jobs.go
100644 acceptance/p2s08/doc.go
100644 acceptance/p2s08/gateway_blackbox_test.go
100644 internal/platform/http/errors.go
100644 internal/platform/http/gateway.go
100644 internal/platform/http/gateway_test.go
100644 docs/execution/slices/P2-08.md
100644 docs/evidence/slices/P2-08-http-tests.md
100644 acceptance/p2s09/doc.go
100644 acceptance/p2s09/session_integration_test.go
100644 internal/auth/app/service.go
100644 internal/auth/app/service_test.go
100644 internal/auth/http/handler.go
100644 internal/auth/store/repository.go
100644 internal/auth/store/queries/auth.sql
100644 internal/auth/store/generated/auth.sql.go
100644 internal/auth/store/generated/db.go
100644 internal/auth/store/generated/models.go
100644 internal/auth/store/generated/querier.go
100644 docs/execution/slices/P2-09.md
100644 docs/evidence/slices/P2-09-auth-service-tests.md
100644 acceptance/p2s10/doc.go
100644 acceptance/p2s10/rbac_contract_test.go
100644 internal/auth/app/policy.go
100644 internal/auth/app/policy_test.go
100644 internal/auth/http/authorization.go
100644 docs/execution/slices/P2-10.md
100644 docs/evidence/slices/P2-10-rbac-tests.md
100644 cmd/aicrm/api.go
100644 cmd/aicrm/api_test.go
100644 acceptance/p2s11/doc.go
100644 acceptance/p2s11/gateway_router_test.go
100644 docs/execution/slices/P2-11.md
100644 docs/evidence/slices/P2-11-gateway-tests.md
100644 docs/execution/slices/P2-12.md
100644 docs/evidence/slices/P2-12-web-shell.md
100644 docs/execution/slices/P2-13.md
100644 docs/evidence/slices/P2-13-auth-ui.md
100644 docs/execution/slices/P2-14.md
100644 docs/evidence/slices/P2-14-stages-sqlc.md
100644 docs/execution/slices/P2-15.md
100644 docs/evidence/slices/P2-15-stage-service-tests.md
100644 docs/execution/slices/P2-16.md
100644 docs/evidence/slices/P2-16-stage-handler-tests.md
100644 docs/execution/slices/P2-17.md
100644 docs/evidence/slices/P2-17-stages-ui.md
100644 docs/execution/slices/P2-18.md
100644 docs/evidence/slices/P2-18-tier-config.md
100644 docs/execution/slices/G2-T03.md
100644 docs/evidence/g2/test-server-deployment.md
100644 docs/evidence/phases/P2-closeout.md
100644 internal/platform/deployment/tier.go
100644 internal/platform/deployment/tier_test.go
100644 cmd/aicrm-config/main.go
100644 cmd/aicrm-config/main_test.go
100644 deploy/compose.yml
100755 scripts/staging_deploy.sh
100755 acceptance/p2s18/test_tier_config.sh
100644 web/src/stages.ts
100644 web/src/stages.test.ts
100644 web/src/stages-ui.tsx
100644 web/src/stages-ui.test.tsx
100644 internal/auth/port/port.go
100644 internal/auth/http/authorization_test.go
100644 internal/contact/http/handler.go
100644 internal/contact/http/handler_test.go
100644 acceptance/p2s16/doc.go
100644 acceptance/p2s16/csrf_integration_test.go
100644 acceptance/p2s16/snapshot.go
100644 acceptance/p2s16/snapshot_test.go
100644 acceptance/p2s16/snapshotgen/main.go
100644 internal/contact/app/stage_service.go
100644 internal/contact/app/stage_service_test.go
100644 acceptance/p2s15/doc.go
100644 acceptance/p2s15/stage_service_integration_test.go
100644 internal/contact/store/queries/stages.sql
100644 internal/contact/store/generated/db.go
100644 internal/contact/store/generated/customers.sql.go
100644 internal/contact/store/generated/models.go
100644 internal/contact/store/generated/querier.go
100644 internal/contact/store/generated/stages.sql.go
100644 internal/contact/store/repository.go
100644 acceptance/p2s14/doc.go
100644 acceptance/p2s14/stages_store_integration_test.go
100644 web/src/auth.ts
100644 web/src/auth.test.ts
100644 web/src/auth-ui.tsx
100644 web/src/auth-ui.test.tsx
100644 migrations/00002_event_log.sql
100644 migrations/00003_settings.sql
100644 migrations/00004_auth.sql
100644 migrations/00005_contact_core.sql
100644 migrations/00006_customer_events.sql
100644 migrations/00007_contact_merge_lineage.sql
100644 migrations/00008_segment_contract.sql
100644 migrations/00009_segment_query_indexes.sql
100644 migrations/00018_segment_crud_receipts.sql
100644 internal/segment/port/port.go
100644 internal/segment/port/port_test.go
100644 internal/segment/dsl/ast.go
100644 internal/segment/dsl/parser.go
100644 internal/segment/dsl/parser_test.go
100644 internal/segment/compiler/compiler.go
100644 internal/segment/compiler/compiler_test.go
100644 internal/segment/compiler/executor.go
100644 internal/segment/compiler/executor_test.go
100644 internal/segment/store/queries/audience.sql
100644 internal/segment/store/query_set.go
100644 acceptance/segment/query_set_integration_test.go
100644 acceptance/segment/doc.go
100644 internal/segment/app/refresh.go
100644 internal/segment/app/refresh_test.go
100644 internal/segment/app/cron.go
100644 internal/segment/app/cron_test.go
100644 internal/segment/store/queries/refresh.sql
100644 internal/segment/store/refresh_repository.go
100644 internal/segment/store/refresh_repository_test.go
100644 acceptance/segment/refresh_integration_test.go
100644 internal/segment/app/crud.go
100644 internal/segment/http/crud_handler.go
100644 internal/segment/store/crud_repository.go
100644 internal/segment/store/queries/crud.sql
100644 acceptance/segment/crud_integration_test.go
100644 docs/execution/slices/P3-S05B-R2.md
100644 docs/execution/slices/P3-S00.md
100644 docs/execution/slices/P3-S01.md
100644 docs/execution/slices/P3-S02.md
100644 docs/execution/slices/P3-S03.md
100644 docs/execution/slices/P3-S04A.md
100644 docs/execution/slices/P3-C07C-R3B.md
100644 docs/execution/slices/P3-S04B.md
100644 docs/execution/slices/P3-C07C-R3C.md
100644 docs/execution/slices/P3-C07C-R3A.md
100644 acceptance/contact/external_event_behavior_integration_test.go
100644 tools/query-plan-gate/main.go
100644 tools/query-plan-gate/main_test.go
100755 acceptance/p0s10/test_snapshot_gate.sh
100644 docs/architecture/table-ownership.yml
100755 scripts/test_orval_generated_check.sh
100755 scripts/test_gitleaks_config.sh
100755 scripts/test_repo_contract.sh
100755 acceptance/p0s01/process_blackbox.sh
100755 acceptance/p0s01/static_contract.sh
100755 scripts/test_query_plan_gate.sh
100755 acceptance/p0s02/static_contract.sh
100755 acceptance/p0s02/test_static_contract.sh
100644 internal/platform/river/contract.go
100644 acceptance/p0s04/contract_test.go
100644 acceptance/p0s04/source_contract.go
100755 acceptance/p0s04/test_source_contract.sh
100755 acceptance/p0s04/static_contract.sh
100755 acceptance/p0s04/test_static_contract.sh
100644 docs/execution/slices/P0-S04.md
100644 docs/execution/slices/M0-1.md
100644 docs/execution/slices/M0-2.md
100644 docs/execution/slices/M0-3.md
100644 docs/execution/slices/M0-5.md
100644 docs/execution/slices/M0-6.md
100644 docs/execution/slices/M0-7.md
100644 docs/adr/ADR-001.md
100644 docs/adr/ADR-010.md
100644 docs/adr/ADR-011.md
100644 docs/adr/ADR-012.md
100644 docs/adr/ADR-013.md
100644 docs/spec/AI-CRM-v2-执行方案.md
100644 docs/spec/AI-CRM-v2-执行方案-v2-至P3.md
100644 docs/spec/AI-CRM-v2-重构详细设计.md
100644 docs/spec/SHA256SUMS
EOF
scripts/verify_repo_receipts.pl modes "${mode_arguments[@]}"

receipt_arguments=()
verify_index_sha256() {
  local file_path="$1"
  local expected="$2"
  receipt_arguments+=("$expected" "$file_path")
}

verify_index_sha256 Makefile \
  691013337a41abbc4309b2b43611ca6848013ec0aa32211d4be51ddfe3cdb6ed
verify_index_sha256 CONTRIBUTING.md \
  851670c7ae917f3e7a3b03d9bec30d687afcb61ccf868fe26f6b547fc8a6273f
verify_index_sha256 .github/CODEOWNERS \
  bb2c40eaad8b8b3dd83cd2d81f58360717ab6dbaeb773afe6d65b7ae18e4f5cb
verify_index_sha256 .tool-versions \
  af006125eef7e7eb61d425c128103efb1d198a6967e4b6da25c4f7324f01e0ec
verify_index_sha256 go.mod \
  2e77bb762be00f230b38a484170d1508cf2bde4c1cbcbafaa3e5f8dda5752229
verify_index_sha256 go.sum \
  411aa7f8fff51ca54e7b0c3f84323a2bcbec8541a9a16cb01c3e46af1c24dee7
verify_index_sha256 package.json \
  a0ce7f09b7397cca843f74b00a7c1d2ee2d019bf287019490d0de09c8460f68f
verify_index_sha256 package-lock.json \
  64f32f2bc22dbde74f3e0e82fbfa91c1160621fc1a771832a0a0b06fb11e2892
verify_index_sha256 web/src/api/generated/health.ts \
  57407569893f22b7c2a3c2ae3bfe5d79960611cbc2fc8b8ccbc0b1f86337779e
verify_index_sha256 .github/workflows/application-go.yml \
  0e5dbdf0b29208555fe89b5f3effdf67f2ebc0b47a2d5908af5676bc6c7004a1
verify_index_sha256 .github/workflows/repo-contract.yml \
  d7ac507239228441b74d1cc8a71ea0f97d1edbdd5c0fdc0ab30b056287e705bf
verify_index_sha256 .github/workflows/secret-scan.yml \
  699a8058682b54611c3cccbf7cd230d647f5c4724d5165fb83f9c2c31b156056
verify_index_sha256 scripts/ci_promotion_decision.sh \
  b6029b28956d4a65f377e8a8d6d1b5566d68b71e99e6106b85bc584e58083e78
verify_index_sha256 scripts/check_ci_promotion_smoke.sh \
  72d48a5a0fbf73611f8030f102030176aeb9b95835def8dbc384a009bd1e9d2c
verify_index_sha256 scripts/test_ci_promotion_decision.sh \
  333ec5478f5aae4484aae400539b133275fec064f823ffdd2994965bcb2e9e0b
verify_index_sha256 .gitleaks.toml \
  b220c3b1e00671ed5d45f796b341a586a533659b7eecadf4906516769414ff74
verify_index_sha256 scripts/test_gitleaks_config.sh \
  2c4da4f3e1fc926910a516593513c8f1e2f51445879bd7a9a5574ce47396dcf3
verify_index_sha256 docs/execution/slices/M0-7.md \
  0b9cd7cbd3ae679b57b54361d8d7d9f0ff34e1568f55bf118505a048c9e229a4
verify_index_sha256 scripts/check_generated_sources.sh \
  20194d64deaacedff07b5a7fa401e19d8afd32244fb8370201c531a428cc3a9e
verify_index_sha256 scripts/test_gitless_generated_check.sh \
  a1c2ecdbad13520ff52d1cc5219363621529c4c74fd2ba8cd53cb3dbb6c6c9ca
verify_index_sha256 scripts/generated-sources.sha256 \
  b9687d34b92599fba2ac3855f4b0d93e7906d709cc516c88fed2caf4705a4f29
verify_index_sha256 scripts/test_orval_generated_check.sh \
  1b6690d6af1d554ccabd167cd0f7ce6d80b740015768bf2a35ca8425072d7e27
verify_index_sha256 scripts/package_release_archive.sh \
  823c105ee3255b7ac28888a00bdabd54b275dd2aeedd7e6963ead8b0c98c16d4
verify_index_sha256 scripts/test_package_release_archive.sh \
  78707542d4221d4aa477c0dd05a3b91e22ba5b729f3b8cd4f93ec5ab98746e01
verify_index_sha256 docs/execution/slices/G2-T03.md \
  50d056e21443ade6d38605b0a534d4f86a55331d8394418c179e161eccaa8f4e
verify_index_sha256 docs/evidence/g2/test-server-deployment.md \
  03ce78968aa820d18e6c4ec3539c04d34a8cfda4cc232baa7a555ae1e88992e0
verify_index_sha256 docs/evidence/phases/P2-closeout.md \
  80f355388180006365fda2c9f49b71499ad5f25ab4b8e05320d2fd0c7befae2c
verify_index_sha256 docs/backlog/post-launch.md \
  3248fa362b357e72f562531feb5ba01297f0a19b275a1e61d40108a4fe522b31
verify_index_sha256 docs/execution/implementation-plan.md \
  2aa87d7574418b1c3860b634ec5bb24e3e1533169bbf73e1036a2cc08f7fa965
verify_index_sha256 docs/execution/slices/P2-01R.md \
  15e671e6d7244993157487b674473fa65acfb26dfbb24e8903130ed9dd1ece85
verify_index_sha256 docs/execution/slices/P2-02.md \
  621075ee177a387666180a6350049e543f53f9e9c740974986c5a14ecbf1c47d
verify_index_sha256 docs/execution/slices/P2-03.md \
  4bb96186ecb58f642018d8302bdadd094a04d8c79c2b00f8e561d0848bf3b5d4
verify_index_sha256 docs/execution/slices/P2-04.md \
  d49596794caaf94e8b63b3edb4e94ebeb85e1a8c893dafc42201dc1c356cc85c
verify_index_sha256 docs/execution/slices/P2-05.md \
  5ed4da5e71bd836a0681f81c41f08f9b0e96673bafa73b42c212597186150673
verify_index_sha256 docs/evidence/slices/P2-04-queue-policy-tests.md \
  c037c37d29cac90b7a6e3eb29ccaabf6bf101720cf8ce5f635e5a9c72ed437ac
verify_index_sha256 docs/evidence/slices/P2-03-registry-tests.md \
  8e96f6cef17231c327326f01e3705c1ffa3185379d3639c1c9bf5489d0f02be9
verify_index_sha256 docs/execution/slices/P2-06.md \
  dcd53bfbd51951f9da51a3719a34835b02ecb22ac87e21667db1494e1dad456a
verify_index_sha256 sqlc.yaml \
  af7b2f322180254f08cdec49b777a69c27c5093d88362e9484eba449d8c9644c
verify_index_sha256 migrations/00002_event_log.sql \
  ffae249b7d5398d0bdacdb72078663b9646d0af908aee2c259a9d476dce73b62
verify_index_sha256 internal/events/port/port.go \
  3a6b94ceef81b0994069b077ab9d25712f69d59dbb832216f54c832c6e59870d
verify_index_sha256 internal/events/port/port_test.go \
  b05782238d51e24efdeac613f4ef9ea97d31cf98113cae3005ef9f5f04cf3341
verify_index_sha256 internal/events/store/queries/event_log.sql \
  055ee606af7b37e0bebf58180f36ab7a0700c3904ab6ae47f5e50662392f44c8
verify_index_sha256 internal/events/store/appender.go \
  9c0887f48d85f6c61e48296e40575edff93c4519d4210dfd5fda57d2baa72111
verify_index_sha256 internal/events/store/appender_test.go \
  ef4cab40e75b1630acab2c576fb8f89071bb2a9bac032fa43414286371045a4f
verify_index_sha256 internal/events/store/generated/db.go \
  08295f6e16bef8e5d3ea40cf12f296ddd5a25965c44a447a35eb6ddc9ef08ca8
verify_index_sha256 internal/events/store/generated/event_log.sql.go \
  0b815f7f800727d30c43e422a9bbc6003c19e011ba9b5e170d06755ce9fd1ab6
verify_index_sha256 internal/events/store/generated/models.go \
  f1bdaa47b54cc769e5d2911263163f58b3bfab594c189adeda5f3311853cd2e3
verify_index_sha256 internal/events/store/generated/querier.go \
  6758951a811245cef2fccb431ba1ede8727ebb7c957983a4c7a968a98773fb0e
verify_index_sha256 acceptance/p2s06/doc.go \
  3101deaa38f9aa4594cf8b94d7e7854e422dd8d9551432e978f9f3811714ba86
verify_index_sha256 acceptance/p2s06/event_log_integration_test.go \
  134c354deae4f3898540bc25ad61eac5497d0ac5a12c9a3c3be4f33027eeea82
verify_index_sha256 migrations/00003_settings.sql \
  68cf560217f40e3b9db22980e1dad23e532b83c62d5399138b99cb024961afd6
verify_index_sha256 internal/config/port/port.go \
  d3ef44bedf2b467cc98ff355961eeb8b85e556b5a47404f59664f19df6f7f9cc
verify_index_sha256 internal/config/registry.go \
  d90a3a2ffea74bbc51453132ad179bb40bd249137908b879412d0bc6f838d965
verify_index_sha256 internal/config/registry_test.go \
  fab5be640b3417346a4b3b0e5a13be288cc38c85bdb3399f2459a1c300235fdb
verify_index_sha256 internal/config/app/manager.go \
  5f8bc34ad31b6ee7435fdbc81d038fd5205e438ec4c98325de8459420d8e4231
verify_index_sha256 internal/config/app/manager_test.go \
  ddd410a63b140ff6b3b26d61089b6d6610ea61ec937b71ca35010b47f54d69e6
verify_index_sha256 internal/config/store/repository.go \
  20410ebe4e9ef49e44605d29083648cf53d2a36fb20aebf5c19d3e73a6d3cc2b
verify_index_sha256 internal/config/store/queries/settings.sql \
  7fde857302d015f24dd6824d903a0b5318a9ff3b2efd3fe5b3e019c44017ee44
verify_index_sha256 internal/config/store/generated/db.go \
  4e62e449e68451475c6fc8047f5f97703cd6bcd4ee27ee00f19775d9fac1849e
verify_index_sha256 internal/config/store/generated/models.go \
  effe9e9755ba425453e9ca96de5e536e86a747f2698c22a32f64c0655c0e168b
verify_index_sha256 internal/config/store/generated/querier.go \
  1070b6e79f04334c92389102cf15f17485c70f26797310003d910c8e387177c9
verify_index_sha256 internal/config/store/generated/settings.sql.go \
  bcd88d6a1bc79fa2cbefdf2454ea53f8649b314d53d0300c93022bebad124e1f
verify_index_sha256 acceptance/p2s03/doc.go \
  1f235f5da9ab144143ba8bc2523c1e1e0c1679a961d778b0bbf22802c5c2bb10
verify_index_sha256 acceptance/p2s03/settings_integration_test.go \
  f9d6aa321d7763f95badb4cd90188efc9119f9f27e728730240b6cc328c08abc
verify_index_sha256 docs/execution/slices/SEC-01.md \
  94947cc722e3898c156004491758fafe550bdbb3188dc69aa2a7553bfe77ab92
verify_index_sha256 scripts/check_arch_imports.go \
  f3a484f9712515894344059366c0aaecde5cf3b499f8882a452122ab7258d732
verify_index_sha256 scripts/test_arch_imports.sh \
  0164b9f845d739bf77f6187ab131ec612cc416344dea765985d29afc4b848ffa
verify_index_sha256 scripts/ownership/main.go \
  92a5275e488fb4c2c1096f67b4d581d322ee8e76d5e65defc44723fbc7e168b7
verify_index_sha256 scripts/test_ownership.sh \
  7afd2ffcf285490f45fefe43321dd1dd18d9439094470ce13fb5aa64200b4d1a
verify_index_sha256 acceptance/fixtures/postgres.go \
  e9e04301d41b57d59eb49f74d75767803a2847c41ee5fab8c383adb586de670b
verify_index_sha256 acceptance/fixtures/postgres_test.go \
  60b0d65c2fa950765166b5ad1cc65f6a5d962f496d50ce6231c34520377fecf3
verify_index_sha256 docs/execution/slices/P2-00.md \
  7f625dc6dd0017266faaf779a79ca093bf600bb4b51adc61660751be86b16022
verify_index_sha256 scripts/sourcepolicy/main.go \
  4618aa2a6f6a715ad6ccd66af85cb2dc385cafec8055d8ec6d063cb94e34ad9c
verify_index_sha256 scripts/test_source_policy.sh \
  96c0e6c0ae5074e458d44cf1ff4ad7411546fc5818930a4f0e252ed324d04e6c
verify_index_sha256 AGENTS.md \
  f223df590a995a2ccb7dcc75e82b90663c51c4802411893c730fb712333732d4
verify_index_sha256 docs/governance/agent-orchestration.md \
  acb65b41dfbb8b4afc331549494b457d2a947b0c08564a351675444a5aa4940d
verify_index_sha256 docs/plans/2026-08-13-p4-legacy-backend-parity.md \
  d403b499f12c5034a7f9e032564c8990e72581b453265f38cc49cc262507f90d
verify_index_sha256 docs/execution/slice-card-template.md \
  8ed52176806cd8a2372ee945294760293979a4e4efe00561a4b63642b889512a
verify_index_sha256 scripts/check_slice_inputs.sh \
  b7b1711da73974b0a89c79bab020e519095fc8aaf36f737b036027ec3a08cb25
verify_index_sha256 scripts/test_slice_inputs.sh \
  32ec32f2c3f0c7ec7c29aabfada4bc4c1f29dc39a37fc0aacfb51e30d304d8a7
verify_index_sha256 docs/execution/slices/M0-1.md \
  1bb201f3550ef638ea85f8f7f5de585b57825177c400a7f5cab3094ef68f6043
verify_index_sha256 docs/execution/slices/M0-2.md \
  97d9acda27150c905af7bc52eeca623034ae1c529d89aac56c253f18d426df59
verify_index_sha256 tools/go.mod \
  430d16ae44aa92c5d9754a5939f4ef0185389d9935d269e905cea662f6f66782
verify_index_sha256 tools/go.sum \
  2515f9dd3dbc17c77f98550be06a8cdf072538e6d8eb296077b6ad91120d2753
verify_index_sha256 tools/query-plan-gate/main.go \
  5e3935770be11475cbfad1e79b0ca22c984efa7ac4f90e5b174f1804c489375b
verify_index_sha256 tools/query-plan-gate/main_test.go \
  681913a250217a89836e83877fdcec5dd942b9f946fdd1dc6347f2312220f7a6
verify_index_sha256 scripts/test_query_plan_gate.sh \
  61a1ce22acc6358b697c50e191c02a6d2e8a0fe20b9d00c2070cececdc8bb497
verify_index_sha256 docs/execution/slices/M0-3.md \
  c14caaed56bc85a386ece43639053b10556d3d4eae25816fbefe334430d6ba0f
verify_index_sha256 docs/spec/AI-CRM-v2-执行方案.md \
  210f6d3c9d0434cba6426ab71fc1cc64bc3a6d3a1a184e55af5f1273c21a8099
verify_index_sha256 docs/spec/AI-CRM-v2-执行方案-v2-至P3.md \
  2cbe7cfda8388a3c122162b2a9f9d188e5428a80a54eb8de8fd362dc87c87853
verify_index_sha256 docs/spec/AI-CRM-v2-重构详细设计.md \
  cf515dd011eb00a1b48d39611773546c8d7794e5bd7f122ef5bf20c6728f82f7
verify_index_sha256 docs/spec/SHA256SUMS \
  bc4a681c8c7c5323ad1a54ab731f4b77d4d9095e3a6aa18d501c96f4e76dd673
verify_index_sha256 tools/snapshot-gate/main.go \
  425cb0ea7702d9aeb817687487f97db27b7e3c03b8a5a95df722aedd8390992c
verify_index_sha256 tools/snapshot-gate/main_test.go \
  77771f548652fc2ffe556b8f8fd31a8f394cc0e90d3e57cb7014711894a29d9b
verify_index_sha256 acceptance/snapshots/catalog.v1.json \
  b15b07867fb2f242cc02943b736e3a41d1f1f910dae8db864ec927ca8120e267
verify_index_sha256 tools/legacy-route-export/main.go \
  a7734a3207ec6e7a58c83a397dc6300fe262e5430b03c12b488262dbc126954e
verify_index_sha256 tools/legacy-route-export/main_test.go \
  1a7073ac6488e5d55873d9d2711b1096e2b08402272cd151babd9efa2c81221b
verify_index_sha256 docs/execution/slices/P1-S01.md \
  39571f878f83ed4b1537342dc49efe54bb7ea5d7720950002f12e45581a67726
verify_index_sha256 docs/evidence/p1/legacy-routes-6cb989c.json \
  fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231
verify_index_sha256 docs/evidence/p1/feature-matrix-id-anchor.v1 \
  1ab849cb10518e55f5c95716c1fab6f2c9e47477d17ad7f3f125edcc7e01ad75
verify_index_sha256 docs/execution/slices/P1-S08.md \
  707667f4058d212ed628d8b868e02225c93a3948c9d90a229856162c15b13d99
verify_index_sha256 docs/migration-mapping.jsonl \
  770fc70538cedbe14e2865e1e21001d92540981b923b201b114174d9262ca512
verify_index_sha256 docs/evidence/p1/migration-lifecycle-index-6cb989c.json \
  404083ca07522a993f349b9a53331663375b53b3344743ddcb238966e3ff2540
verify_index_sha256 docs/migration-mapping.md \
  9e3bbf63d9357291c19071b98e79f16ea76002e398e13bedb7b2ac2d89fb1e32
verify_index_sha256 tools/migration-mapping/main.go \
  fe6e133f6b6b9c746bd9e8347f8a2eb049daf45b0402fa84f5cc5cddfc3fde74
verify_index_sha256 tools/migration-mapping/main_test.go \
  6304f26af0587ebb47e622eff9afbd8cb45158a69f1e5e8befd0df5808d6d70d
verify_index_sha256 docs/execution/slices/P1-C02.md \
  c7b8c7236f20279d244286f0f3facaab28ba831a64e7f662755a6b9700abac43
verify_index_sha256 docs/api-mapping.jsonl \
  6e2e3cc5122a12d121bcc5be41d716c40e57338a7bb92a53b0b6b9d0a1254e02
verify_index_sha256 docs/evidence/p1/route-triage.csv \
  ccd7708b12481add3af74c0d6442e5f4333ee076e28110aa4b330e7181f00da4
verify_index_sha256 docs/evidence/p1/g1-decisions.md \
  90fd1c6276604085ddd5cad9f84fa07288141a410c6ea48693862cb77a80cd03
verify_index_sha256 docs/evidence/p1/g1-signoff-pack.md \
  9ae83d6b4724fd8c4698b189fc446411f895585ec07b5c1630c0f9e058d80901
verify_index_sha256 docs/evidence/p1/feature-matrix-top20.md \
  de58e5528dbfc92939f163aca2cb0ebfa38cd640d9c7b9d0f0608f173d0d91cb
verify_index_sha256 docs/evidence/p1/migration-exceptions.md \
  4e00455c4a637df40d9da41f44b4ffe0e44c948226ca766254ba83c964581c26
verify_index_sha256 docs/execution/slices/G1-D01.md \
  1d2f47dd1af0d27533184e772de2a28d9cc4a663b923bf1bd7ddd6121417be8b
verify_index_sha256 docs/execution/slices/G1-D02.md \
  3fc37264f57da5fa15d8d3765554ca3e1eea7ce8bd10865176eb9d3b537f4742
verify_index_sha256 docs/spec/AI-CRM-v2-P2P3执行计划.md \
  28273beaca7c626b86d6553ac43342c58b73056438b64d209b5b8273d4a35ef4
verify_index_sha256 tools/p1-reconciliation/main.go \
  2b1162a4a423b9f106b512d162a5ebc4d3bc5fded125caaa69bc0d7b823ade99
verify_index_sha256 tools/p1-reconciliation/main_test.go \
  ecf7d359ce9bac04949e78edc20484a2a759adcd606c8ba971e5e4976ae1f703
verify_index_sha256 docs/execution/slices/P1-C03.md \
  cd9e0441d79b9e1887030087bb4dd800a0a3ca3529275008083d00c577572ffc
verify_index_sha256 api/openapi.yaml \
  02616768e51508c9940d6c03290880993d0244d84d644ec37adf1c8359419231
verify_index_sha256 api/oapi-codegen.yaml \
  78abf754fe91788d5cbdab2286ba66dc32d5e13ed1735ffeee9119e473fd4a2b
verify_index_sha256 api/oapi-codegen-p1-candidate.yaml \
  17058e729ac6cab2e8a1e8cf0a211e6f4db3d5f36af5e75186822b18b6a33742
verify_index_sha256 internal/api/generated/server.gen.go \
  f1e66b50f9ba6722b663967ec1da44cf6bda718246d66a82aef9179c670f2e38
verify_index_sha256 internal/api/candidate/generated/server.gen.go \
  bf9c42e5f053d633ea3683c2cbce66735964fa5963f96ed31df0363105334c0e
verify_index_sha256 tools/openapi-contract/main.go \
  6cf0fd1c4d1b144acbb27703dba566953846fa592eacb5329b5ef9d5c8e98b61
verify_index_sha256 tools/openapi-contract/main_test.go \
  5e0f06df6c730dc24a4847f3a1c8242d4e98a532111de7d58bbfcc1dcf9244c6
verify_index_sha256 acceptance/p1s11/contracts_test.go \
  78ac2c4bd0555dd2d268fa8584da9291ec207aaf7b2d6c4d65b98e2b6244611f
verify_index_sha256 acceptance/p1s11/doc.go \
  8a7f18c253c7b95d9714845c8a98d548c5730bde49de5d8bae156bc3967727d9
verify_index_sha256 acceptance/p3c00/contracts_test.go \
  cff6f3a1c820feb87249bd67d4753e673cabc66faa3528b87fc13049f8f3672a
verify_index_sha256 acceptance/p3c00/doc.go \
  ac3af23bf06ff6b0a4af7eb746a276ee4939cd5085a6dae936dafae638eff87f
verify_index_sha256 docs/execution/slices/P3-C00.md \
  2143d480d2ee9d6de56e72b11e42fe41729d7c554ca55b656c6796cf6b6b02c6
verify_index_sha256 acceptance/p3c01a/doc.go \
  14f8d1d61ffae5abfab18d28fa2edc80ab11f36370e0856a8e2d12d6fd19d0a6
verify_index_sha256 acceptance/p3c01a/schema_integration_test.go \
  7b040fdcddea50f19a1646e3d3d38a998556c8fe5d40399b257cbcadb233cefd
verify_index_sha256 internal/contact/acceptance/doc.go \
  931ad07bd4ce95e1a08ca59a322bcca893f3c120fae2f81c71a0b5c53626d5f9
verify_index_sha256 internal/contact/acceptance/schema_contract_test.go \
  1490527d2e885d373b78b3123d452e7f06848c79840ecb85fd445d83d9d2d583
verify_index_sha256 internal/contact/app/customer_query_contract.go \
  9dafa3516a4acdbd2540ccf2575487c4005d321583d5ee9b10d33447c464f951
verify_index_sha256 internal/contact/app/customer_list_service.go \
  cc671eecd5efa2ae28339b2c1a43b98a903e227be5f339701b3c0119f0afa79b
verify_index_sha256 internal/contact/app/customer_list_service_test.go \
  dc72f01605ff4efc0d1becc2ba1ea60b114e383fa76a20f92116c8f097a153ef
verify_index_sha256 docs/execution/slices/P3-C01A.md \
  cb271879f61d4f901c020641faf22880b3903032252402b8b5a637c6c9aaa2d5
verify_index_sha256 internal/contact/http/customer_list_handler.go \
  9a75941667f3f6f43a9dd8ee9fbc6421c268eabd1ff04cda45af055a770cdd36
verify_index_sha256 internal/contact/http/customer_list_handler_test.go \
  09b37ca21998b11e7d540e3caf51cb8db5ccfcae963b2d71650e896ba441ccf0
verify_index_sha256 docs/execution/slices/P3-C01D.md \
  05cdc71e2649af2f481728338192dfe96ae6264aa2b39d0ec5404bf5f4b9a9a7
verify_index_sha256 docs/evidence/slices/P3-C01D-customer-handler-tests.md \
  d18b0701238ffb170a4e76441383bcb228e52e6d7ec2a6caf231927a29f1fce9
verify_index_sha256 docs/execution/slices/P3-C04.md \
  5c413a9ff757d5819a1030c5c1e9993772db8e0c6920ecd41788f09ac5fa90a9
verify_index_sha256 docs/evidence/slices/P3-C04-customer-list-ui.md \
  66ab535cf46c4935057f0f5a6b0787c3b55ac8c1d351f82aaa1839ddd5519e97
verify_index_sha256 acceptance/p3c02a/doc.go \
  100b1b52b8089d9bd3f23e9f5e3fef03dd9cf980f578ac9d1450bee21d8c9aab
verify_index_sha256 acceptance/p3c02a/customer_mutation_integration_test.go \
  288b37d4b3be5fba23dabbc0dd83b8dd471af226dbcb8bb2db500bea805ee9ec
verify_index_sha256 internal/contact/app/customer_mutation_service.go \
  4ea590158d08f7506e99e9301761be7a710eeee049f91e040f316cddd54bb404
verify_index_sha256 internal/contact/app/customer_mutation_service_test.go \
  3ba0e802a58170023fadb94cf89dcb65f436b8c961d132f846d2e3adf3d3fbfb
verify_index_sha256 internal/contact/store/customer_mutation_repository.go \
  46e1a3ae1ed655463ede371755c8f23a0387d78c15817284ca1fdf33a88b0acf
verify_index_sha256 internal/contact/store/customer_mutation_repository_test.go \
  6736dc9263dc0db994037e537be007e4684fc993b8a151ab5e3c5dd2aeaf119c
verify_index_sha256 internal/contact/store/queries/customer_mutations.sql \
  424833b87f7854f0c7251db6b68b5c490ada13ff63533d545932e7dfb194afa2
verify_index_sha256 internal/contact/store/generated/customer_mutations.sql.go \
  f693e28942b9b68161e79f6f0eac42fed170459cea69fdbf352f95bcf2e99e6a
verify_index_sha256 docs/execution/slices/P3-C02A.md \
  3b7fa0edce011243ee5983f39bb3e461ad3da84688b4139846fe682b7a03412a
verify_index_sha256 docs/evidence/slices/P3-C02A-sqlc-store.md \
  1c4c82565b6531e35bd7b657c583fa1fab4591b871550af145ecab4d966db40b
verify_index_sha256 docs/evidence/slices/P3-C02A-service-tests.md \
  019b694aa844264ea8cbe6f686060df33dc430248ddcc567dd7f02181bd21c1d
verify_index_sha256 internal/contact/http/customer_mutation_handler.go \
  8a3c6c5c31970f2c747ebff7e770cdf2e6940f9a6c20b0deae6a09a94a9a0935
verify_index_sha256 internal/contact/http/customer_mutation_handler_test.go \
  2b2c6e729bb1d5c840aabb5251cc5a013a245afb003148f22b154a3e87569266
verify_index_sha256 docs/execution/slices/P3-C02C.md \
  997680d558d59c01466a10ad48c3f2f7e8f3c261248c5665883e110cf8a845c4
verify_index_sha256 docs/evidence/slices/P3-C02C-handler-tests.md \
  c0dc2924b0880d0716275384d6f090c73572aa503eba6d4284ecf3a077d6c9a3
verify_index_sha256 docs/evidence/slices/P3-C02C-service-tests.md \
  f4f68b915cdd82cda4fe329fb4085aa26f05b2dda8628fb736092e1a8ee0b172
verify_index_sha256 docs/evidence/slices/P3-C02C-store-tests.md \
  819806fcc4183a390f4e0d00887a9588fc6c44a8488675d515f67d1542dc16a6
verify_index_sha256 acceptance/p3c02b/doc.go \
  ed42e420f6b6ce272206a24fdf97d6749db09d9785225d446e15ef4d7de58a83
verify_index_sha256 acceptance/p3c02b/customer_detail_integration_test.go \
  dcd72c1916add671497edd12fbac383adbde5973c3e4c2170e33e7947bdbbfd2
verify_index_sha256 internal/contact/app/customer_detail_service.go \
  ef4d5edce6246a8acae9ebf6f2e1eb1a8546760f6269184543b45c1e9de60486
verify_index_sha256 internal/contact/app/customer_detail_service_test.go \
  e438ac424053837cac932235185a448238275a382d79f51d4f7ded0deaf8b6d1
verify_index_sha256 internal/contact/http/customer_detail_handler.go \
  3d36dc4ace962d04f55f4778b45eb5e00466dbe3be0ee5fe515cfd79960e68ae
verify_index_sha256 internal/contact/http/customer_detail_handler_test.go \
  8f7891766463c8a27fbb5b8c7a549d8bc4590566b37b2793fc6ff82ae62e3632
verify_index_sha256 internal/contact/store/customer_detail_repository.go \
  669cc951af5b55598656583af1bc0b7949dc4760f5b24dcfa8b4e327ff224fe3
verify_index_sha256 internal/contact/store/customer_detail_repository_test.go \
  5ba2056ad4eba4777158a521e6c18632e1ef1acab92a44327694e1b843a7e3c4
verify_index_sha256 internal/contact/store/queries/customer_detail.sql \
  8e73ff0d66a5dd94223a64601d8bb3bff4befe30f80ce3a850945920c06d628f
verify_index_sha256 internal/contact/store/generated/customer_detail.sql.go \
  8dc424f267b9c7c59108aeeaf3cd4a080093434c6cf4179df8cf5191968ada67
verify_index_sha256 docs/execution/slices/P3-C02B.md \
  bb10f5e4fea5728c6b0fc0fcf8ce7beb92c0f97de38dcc4cc54bc3e61793de68
verify_index_sha256 docs/evidence/slices/P3-C02B-sqlc-store.md \
  58a8eabab52691740917ce50aba509450bd68397825bb61cbe31b703e006772f
verify_index_sha256 docs/evidence/slices/P3-C02B-service-tests.md \
  0dee3dfd2f7ba8e2ff019896dcf5cb5eeb816cbc5bdf79edd7e08d7a693b26c2
verify_index_sha256 docs/evidence/slices/P3-C02B-handler-tests.md \
  1322b436468700af2afca4a068dffe969e346a3f66fdb54d4f4b0f6232e04847
verify_index_sha256 acceptance/p3c02d/doc.go \
  0f9904b5bcbbc94986169fe7a93f51637cc731f85c07b9e7713d7ad4fe6216a7
verify_index_sha256 acceptance/p3c02d/customer_event_integration_test.go \
  f0996da91507bbf648d836cf6778574cae5e7b18aae94d0f255f73f7dbe8dd3b
verify_index_sha256 internal/contact/app/customer_event_service.go \
  cfd26bbb7fcc822202549477e3099ad7391a219dbbb286db873c37d6cabaadf3
verify_index_sha256 internal/contact/app/customer_event_service_test.go \
  58cc29829d32dca6768005f8a958eef22bccef3dfca197f085fac64b9069ec40
verify_index_sha256 internal/contact/http/customer_event_handler.go \
  f4eebe68533a4e7b45a094afcefda4a04f73f6c4d53f620648ff8d58a9d076b9
verify_index_sha256 internal/contact/http/customer_event_handler_test.go \
  2ce187a0581e11698f8be9010eeb3f6bf7ecc468b31bd4cf666158e63ea3cf8d
verify_index_sha256 internal/contact/store/customer_event_repository.go \
  3d2b91e763ffc4f419c16b3a50f8dc57df80965ea171d013b28865dcc96de834
verify_index_sha256 internal/contact/store/customer_event_repository_test.go \
  ad56c93c94901b20c67f20dbed0415f7322643de68ed700392b8d2c86421b9fd
verify_index_sha256 internal/contact/store/queries/customer_events.sql \
  93c226f0ba36f49812ab5e9fc2be6d0c98b839625b3daf34a19270bbaf5812a0
verify_index_sha256 internal/contact/store/generated/customer_events.sql.go \
  98db91969c639e0711f439cd8bf62e32c7e10f87bc9ade62d3c38f65894ea2da
verify_index_sha256 docs/execution/slices/P3-C02D.md \
  d8dffa82e7fb1a5b9a30ac8cf78a41fb1cc6e78e1744d8661092f69fff353eaf
verify_index_sha256 docs/evidence/slices/P3-C02D-sqlc-store.md \
  83a0a580784596785573793e8bdbd638ed7e6ed4dccf1d0679000ca044572ee3
verify_index_sha256 docs/evidence/slices/P3-C02D-service-tests.md \
  05aed67971f965e145441874d8f6f46f7198dd39c84b118d2b143ef1aa0482eb
verify_index_sha256 docs/evidence/slices/P3-C02D-handler-tests.md \
  3e2c3f8099cea96179c45640f62032bf6a7fcd71d3f381bf0b3bc22f459b7e58
verify_index_sha256 acceptance/p3c02e/doc.go \
  1c788e51d1792aabf8ef7c4285025aa67fe2ded7b37b6baaf2516836ba58268f
verify_index_sha256 acceptance/p3c02e/tag_catalog_integration_test.go \
  2440a19758261292c593edf3f8b733129b749b0ab0cfc035545762882fb83dce
verify_index_sha256 internal/contact/app/tag_catalog_service.go \
  70dd86af59d936d87a858a3cea0b81d3d0f9d4857ef322c7756a34b30e3b094b
verify_index_sha256 internal/contact/app/tag_catalog_service_test.go \
  4f6633c52ad64a06b250c20b4ec798928125662a20f88571e35048cec5587baa
verify_index_sha256 internal/contact/http/tag_catalog_handler.go \
  39bcafad977bb23fdbc6e7f1afeb8afa4a3d12ef35a5c7a94f63902b1548c9d6
verify_index_sha256 internal/contact/http/tag_catalog_handler_test.go \
  386a2b1913dac8924fff749430bac4f6e23b625ff175f84fec85a8b171539c07
verify_index_sha256 internal/contact/store/tag_catalog_repository.go \
  9b06834333ecb52dbe3ef0794fee95e97035e4e5b6ae412e102c03f25c8ee0dc
verify_index_sha256 internal/contact/store/tag_catalog_repository_test.go \
  d33f78ea5103f964c1c5f2eb44f4767985d24ade305fa5b587daa722963c92e3
verify_index_sha256 internal/contact/store/queries/tags.sql \
  b846791575666b704231cc1f5b6b4976d2fbc10b3e2ff28db022f0abeaacf127
verify_index_sha256 internal/contact/store/generated/tags.sql.go \
  3e6fe9a1941e90b7ddffe043f13ba6277b4a0c5be2cfd575b4dde1df350d5a03
verify_index_sha256 docs/execution/slices/P3-C02E.md \
  c12efe4184fa1bd31f614057a0b8caed2c41f5c8d4f04514f64dc513a39486ad
verify_index_sha256 docs/evidence/slices/P3-C02E-sqlc-store.md \
  d6c0e1ddb934ccf5a20eb9c14b55fbe1155bb6f0d11ee62baf87d610d1271d7e
verify_index_sha256 docs/evidence/slices/P3-C02E-service-tests.md \
  12c3191c1402359653f6b8e6736a3b10ff3f8958395339b317288fa6d69c0a9b
verify_index_sha256 docs/evidence/slices/P3-C02E-handler-tests.md \
  ecb86db382d51896cbcff6698ea0791734bfa099a69a8d1b327b06cf0acd90ca
verify_index_sha256 web/src/customer-detail.ts \
  b0cb258ae7846a6547ef479527006f2c3fe5daf69dc4a7689ee9399bc8328381
verify_index_sha256 web/src/customer-detail.test.ts \
  84b474ab8690db8d6ec5513bd20866430061bbb80018d6a043ce00cdd9fa8c44
verify_index_sha256 web/src/customer-detail-ui.tsx \
  85b0c1c24303c3157a1fef7b1d42b267240135bde781c1a73ad9cec85aace1c6
verify_index_sha256 web/src/customer-detail-ui.test.tsx \
  6d4bd7e07ae2b7c267729a5350e1d1ddf48885ea9eceecd2449aa6aca2c4ab94
verify_index_sha256 web/src/customer-detail.css \
  089c403ddcfb01a2fe12f68e5586b3ee881042c887f6ec19898a5ae5eaf62987
verify_index_sha256 docs/execution/slices/P3-C05.md \
  0616bda65c1f34a98514aadfd01d5640af1440e84e46f11f6f88be42121cfaab
verify_index_sha256 docs/evidence/slices/P3-C05-ui.md \
  7707db512250c6ac4e27ed288481159e964d84f21680f5571c31d4187e90f6a9
verify_index_sha256 docs/evidence/slices/P3-C05-route-tests.md \
  04c5e6ec1fb6751234bd58c5bc2f0e0d8235ad9fcfaa51cb95ea7fcf4608e11a
verify_index_sha256 cmd/aicrm-contact-perf-data/main.go \
  85ede9f7188fbc3508282fd1b6f9be820caab42dc38beecb5b071b98e9cc33c6
verify_index_sha256 cmd/aicrm-contact-perf-data/main_test.go \
  a66d57b33aafb173979a4b8c3c8e395b496970faaf30f0220a28bc2b7b2f1da2
verify_index_sha256 docs/execution/slices/P3-C06.md \
  e9e1da290473a886092595cfe3ff8bba6e17f48762161fceed2abff913c74c53
verify_index_sha256 docs/evidence/slices/P3-C06-synthetic-data.md \
  4ab11a23ee9b7336b05eddbbd35a2d5d0516341f3aa11610a873b06cace83a96
verify_index_sha256 cmd/aicrm-contact-perf/main.go \
  64fd38df8c9b3c84f5e4f8bec712a94a2f9092cf24d83e012b9485e19270942e
verify_index_sha256 cmd/aicrm-contact-perf/main_test.go \
  4c605278f05cd20fb35b404d5af4b802d064bbc63bdf44bd15f383b4f6132cc1
verify_index_sha256 docs/execution/slices/P3-C06A2.md \
  8fc11d267b9b208a3ff686ede72dcb627de3ec9dd0a6a1101b02d6055d08ccce
verify_index_sha256 docs/evidence/slices/P3-C06A2-runner.md \
  ca9b0a202e317be81857d423e8701e7c0208553f8771500d7a62f7b1d9a777b5
verify_index_sha256 internal/contact/store/queries/customers.sql \
  6a9971d742002aeda3719b56422c45bb809fefbaf96f7bf2dc107357f1fe6a16
verify_index_sha256 internal/contact/store/customer_query_repository.go \
  8593fa77406f052374d26d6a63725a705b27d4eb8844970291120dfa0217760a
verify_index_sha256 internal/contact/store/customer_query_repository_test.go \
  cfed0d8514c92913c093bb9e9c705b92fda81accfbe26cbfb09bfc9e03d501b3
verify_index_sha256 acceptance/p3c01b/customer_list_integration_test.go \
  5df32159ae8a9b7575f53a2d8db57ad52b8d0a2437df0df5d744d3514e857809
verify_index_sha256 docs/execution/slices/P3-C06C.md \
  4f68229a69f337c4b67c7a100f4c9874d5b9c0e2256e7c2ea711836c9144ccff
verify_index_sha256 docs/evidence/slices/P3-C06C-query-optimization.md \
  23534fbaf7a8ee100b6dd8c23e704073018eabd877f52030ba5fdc3fa1a21429
verify_index_sha256 docs/execution/slices/P3-C06D.md \
  312f73315b56243d0053f85db87d4a5907e83b7dac21b2a99c5e42ec1679b7cd
verify_index_sha256 docs/evidence/slices/P3-C06D-tag-count.md \
  6a328bb6e0d0dba614498c64fa183b230254aa06847aa2e49b2ed40189c7a0d2
verify_index_sha256 docs/execution/slices/P3-I00.md \
  3f4c4c870960c5d5afa84a40cac23f3412dd6f345d6bc6ebb6016cef353d4303
verify_index_sha256 migrations/00006_customer_events.sql \
  c95eefb3e1f6b00b663f7cd1ce39f9f2898e6ec33cc539a8a6eea36e48982445
verify_index_sha256 acceptance/fixtures/cmd/validate-database-url/main.go \
  db77a60e15d2f8e6b17a329f7c5973d00377b70e168f08c868c044acc7e6bb24
verify_index_sha256 acceptance/contact/doc.go \
  30428d601628ce702889a782f74da6ccf56a120b5c12022e3e7a5adca5ae2d05
verify_index_sha256 acceptance/contact/partition_integration_test.go \
  bec8125543430b6a4838ddc460ea7dbf39f05f0a12aff7e8deab51fe6f9237b4
verify_index_sha256 internal/contact/store/event_partitions.go \
  e5985879bc4dc0cff1681eb4a9101d00e1d3e77e55188db197bc7c663c0fbf4a
verify_index_sha256 internal/contact/store/event_partitions_test.go \
  2e03bbb83f3ee0fb816713635dc65bca0ba39643b8b86ad949c25ab933ae9a87
verify_index_sha256 internal/contact/store/queries/event_partitions.sql \
  04ffbec296243e9a210c517b012cf0323799f3f6916397350cd5d88bfabc678a
verify_index_sha256 internal/contact/store/generated/event_partitions.sql.go \
  50e45dddb59a803cd6088f0ae2b7171a987b46e179fe7e76199998ee70376941
verify_index_sha256 internal/contact/worker/event_partitions.go \
  10baa570ae166a6af519a1816b2793dedcc8c6f1107b21a69dd36081ee3cfc41
verify_index_sha256 internal/contact/worker/event_partitions_test.go \
  3227e6707531282ba1520000bdd565d41a4b479f6d4775af12155df1494b9718
verify_index_sha256 docs/execution/slices/P3-C03.md \
  8f4cde8b2d43d4e96d531734f8cdc4202c9f11f66b33f650664a42ec6c103dec
verify_index_sha256 docs/evidence/slices/P3-C03-partition-worker-tests.md \
  56b9ce2aaa579594d3b937b88a0b03994183c2d40e7690135b3dc0684293e1ec
verify_index_sha256 migrations/00007_contact_merge_lineage.sql \
  2c4a7ad698edbdb7adb8987f3b0a7b701edc2c8953e3603a914160a366899740
verify_index_sha256 acceptance/contact/merge_lineage_integration_test.go \
  915827562dca143619a686320c769ce02743313e1cbbef505d575e8a09230ca0
verify_index_sha256 internal/contact/store/merge_port_repository.go \
  1e37a639d2672a6921c040bba57608da07b66d6635a161edcdd7f0f51eb54521
verify_index_sha256 internal/contact/store/merge_port_repository_test.go \
  dc22c499ed4b125c40c044d0be335651ab885120c963ea4b5d84c405c7da69a3
verify_index_sha256 internal/contact/store/external_event_repository_test.go \
  f89c3f316ffaaea4f61f4f89b62aaecbe20b5dba51a365654cb889421b68b00b
verify_index_sha256 internal/contact/store/queries/external_events.sql \
  6db32d9fa67c1355f7d37a96e98a58b2d945ebd3cfabd6987aca047ba561ebb1
verify_index_sha256 internal/contact/store/generated/external_events.sql.go \
  644bd13fe3660efa2b810a62deb19a6a47dc34db7167f88413ce88fc2033bfaa
verify_index_sha256 docs/execution/slices/P3-C07.md \
  88e142d8f5416d1267889376d200836eefa408bbcad9475debc1421b81531a62
verify_index_sha256 docs/execution/slices/P3-C07A.md \
  0285ff8d1cf0e5f7fb6d6529053ca4a88a3d8c9103afd2bc284e27f79420ba1d
verify_index_sha256 acceptance/contact/lineage_timeline_integration_test.go \
  d53de174f0ac26c20671c2123933a92b324d8fa6fd884d4152233db816e5c6ff
verify_index_sha256 docs/execution/slices/P3-C07B1.md \
  cac2f64aedab170cb4d890a3e2a54fd972728fd20d04ba1e085483e99392edd5
verify_index_sha256 acceptance/contact/lineage_timeline_plan_integration_test.go \
  b99e932026ae26956d066953ba5f0a10108cd084155fbf2a2b4babac5906ba80
verify_index_sha256 docs/execution/slices/P3-C07B2.md \
  87f7d7da97a0e66f24cdb190358adc6337700f1e48207468626ed02fca55ce2b
verify_index_sha256 migrations/00008_segment_contract.sql \
  7c0e007360a6045b3eff5abe5b772928a5ad990950e508a50c8918007c0ea569
verify_index_sha256 internal/segment/port/port.go \
  d3ae9e5403d5b9063534ae2b8b26128c543d3cf0bb36a11d466f8883bf91380f
verify_index_sha256 internal/segment/port/port_test.go \
  01d17e1ae55a71279f0a566469ff98ac49fc2ca69df6ab9b40629a972bfc333f
verify_index_sha256 docs/execution/slices/P3-S00.md \
  ad56a1fd94bf63c184278602f834aea499c7fec41764f061b59e765e11b94d7a
verify_index_sha256 internal/segment/dsl/ast.go \
  55d49468d801192c331c25c54844376b23a5732bf517bb2f5c4bef84cd9ee131
verify_index_sha256 internal/segment/dsl/parser.go \
  1b719d9ccf5b7837f62335f850bd6709219ba794f22f57ee922cbcc02c65b79f
verify_index_sha256 internal/segment/dsl/parser_test.go \
  4133ea5ccf7337b35dd350597e6a358a18be3741fe0755414dc07e762ef85e4a
verify_index_sha256 docs/execution/slices/P3-S01.md \
  37eb186e07e5a66f21950de1beb32651a1efbb2afd92b3db84d179ec1de99f77
verify_index_sha256 internal/segment/compiler/compiler.go \
  516b4ef6d0131f4c4a2d186b05caddba02d3ab60e0804ac99206895625a9e40e
verify_index_sha256 internal/segment/compiler/compiler_test.go \
  5e936728f73f7a5be2f755ec0f2e4158dd40ee5b35754301ab6502e07a73a889
verify_index_sha256 docs/execution/slices/P3-S02.md \
  f9b5f078beedfbdb30c703bca22672bf80ccd45abf121270e700b48040dbf1f7
verify_index_sha256 migrations/00009_segment_query_indexes.sql \
  31de3210612f7b895fd9feed6da2f168b68f50c2e7c6a825bb40644f9370293b
verify_index_sha256 internal/segment/compiler/executor.go \
  d3af0156b74bca647d97f144b01b6e6e6fe97d38bb9f51f847a2eaf8e1b93c6c
verify_index_sha256 internal/segment/compiler/executor_test.go \
  7980d7da1203eb89c870c89959085428bfff0a07c2dd78a840c8509950061e4a
verify_index_sha256 internal/segment/store/queries/audience.sql \
  863560b8c36362d3e5b5b7dfd2a0fd034e050c7c94b20a612b49f096ab926aad
verify_index_sha256 internal/segment/store/query_set.go \
  35badea18e3f1fa46ed2784d83234e1166331cc80d85a3b7dc0dd2e03c33536f
verify_index_sha256 acceptance/segment/query_set_integration_test.go \
  748e9ebe2a11b87c2ce14f2f7844b13aa2bf3ded0bd18dfe3c7124d79d72c931
verify_index_sha256 acceptance/segment/doc.go \
  89fb7cae27317abcd01789dd4cff4cbeddd74b34db7564bfa427aa35987b2421
verify_index_sha256 docs/execution/slices/P3-S03.md \
  8f2934cc4664b91566a92bc0b267c96c6c507f7592a29441f27a021a935716c7
verify_index_sha256 internal/segment/app/refresh.go \
  c4584b4be60ddf1366c450f4357635b145a57ba4e6a5fabde892bfdfbd2fdef7
verify_index_sha256 internal/segment/app/refresh_test.go \
  8d9036916b25a07cbba2c2ff828d22c003ece84028c962cd7207db12139de6d3
verify_index_sha256 internal/segment/store/queries/refresh.sql \
  e5cacabce048f85816e1d626b4c01a2069bef69f3357173a6c3004385f9f96d0
verify_index_sha256 internal/segment/store/refresh_repository.go \
  33e464d6cf73ab14f440103dbe6301192a4eefa0bc2053dc334b6c715965cf70
verify_index_sha256 internal/segment/store/refresh_repository_test.go \
  1c75c707d310ffdc6bad192fbdfe92b40f273208f3e0ef3d75fa7aec46925952
verify_index_sha256 acceptance/segment/refresh_integration_test.go \
  b6e79e1564497ae1b8c91434ba6fc250a3982a39016695298e9e754a13d92f63
verify_index_sha256 docs/execution/slices/P3-S04A.md \
  63cf6550d1d374e05f85965f8a559ec9a4a3a1f634701c3ee5d4c78ba8a01b7b
verify_index_sha256 internal/segment/app/cron.go \
  cb18a115c288304a1be0773ed8e746a62cb3b5e2f55bdf598ded4e5317e4134d
verify_index_sha256 internal/segment/app/cron_test.go \
  8b628737f31b402a47d09a89ad512242f9066d76c0a9337346b65f47c9831b2c
verify_index_sha256 docs/execution/slices/P3-S04B.md \
  9525e0caf7fc1d3244537e39f7d703dfe95426aa375737d0989bcec6a6f0e61a
verify_index_sha256 docs/architecture/port-contracts.md \
  4952f77f8fd461573c2b46f7cbddc0fcc80892debc2e9b9298a23e1012420cf4
verify_index_sha256 docs/execution/slice-ledger.yml \
  90eadff6db9212a4b582e08544b47971380e6c5712ecf3cfdbcd003a79ecf8a5
verify_index_sha256 docs/execution/slices/P3-S06.md \
  9acfa58b69a3ee8395a574023c7ad68049cfbb1f68d38cfb88a89e80ed9abda9
verify_index_sha256 docs/execution/slices/P3-I8.md \
  52756abe8f84b30dbc7df1604b18748af4b51e470c08c6e2bd98a6afff570119
verify_index_sha256 web/src/segments.ts \
  d44e630fc6045ce4ee0adb01f285d0f2fbdf43904339987035d20139834dbef6
verify_index_sha256 web/src/segments.test.ts \
  fae3b571c659d7dda6415bd7433e8127498450f9333e00ea636a108008872985
verify_index_sha256 web/src/segments-ui.tsx \
  24fb00c24c30bc66da5025d831b6dcb23d576078cc5ef3b85fd84b24632ab0c8
verify_index_sha256 web/src/segments-ui.test.tsx \
  1f14826d392cc1644577a029cb2b5cc5070d6044bd560f3653f0777985c0603f
verify_index_sha256 web/src/segments.css \
  98c8e5e7b499df9fab0b0c762e2da1f75303a6fa7f1096d8c77f15d998f88fed
verify_index_sha256 web/scripts/segments-browser-smoke.mjs \
  88c47fa84d4b120588f8c698964c65af2ae43f155da9df31c0858ce3ebf6d3ea
verify_index_sha256 web/src/identity-reviews.ts \
  275ef82a1a8f8d3f94e1cadf54e7d8f04046f1888b07da3507b5674fae194b16
verify_index_sha256 web/src/identity-reviews.test.ts \
  d0df6fab898af86d6b936a343fd695e88b70537091a6da48497a15f9b7b10f32
verify_index_sha256 web/src/identity-reviews-ui.tsx \
  0ca4facfc4ca2b0371daa931d43ac5cdce6048d972d50f8bdb8eca2314addd9d
verify_index_sha256 web/src/identity-reviews-ui.test.tsx \
  64cd31780800622634557eb42e0497f068694f653423f552296c707935d271ec
verify_index_sha256 web/src/identity-reviews.css \
  2d355f444cc59a065686afc78c970eb61acce7ec92d8cb5bb89e92cb4fdebafd
verify_index_sha256 web/scripts/identity-reviews-browser-smoke.mjs \
  78b7fd2624f3aa7503a856f0d15351c3d6d8a053b67a8347dc2e20eac599b29f
verify_index_sha256 docs/execution/slices/P1-S11.md \
  5866fe52a0039f310c10add3d8cfa77eaba9d748dcf518d71df04dac2354a872
verify_index_sha256 internal/auth/port/port.go \
  33058cef7510921e29546259cdeff79cb0d95a2722254c7f784305faf03cd04f
verify_index_sha256 internal/contact/port/port.go \
  181f4ea2ab9c140314dda3e110eb9616a678fe4872d8e8c956750aa4673da9e5
verify_index_sha256 internal/identity/port/port.go \
  bc3f25a61d71865c511fb41bb34871a03b223508fccdf1f33586a8193850ff13
verify_index_sha256 internal/identity/port/port_test.go \
  91e5f66dc7b240334826ad6dd5212685e1fb9b1d8d677c7dea1a27dbb824cb27
verify_index_sha256 internal/platform/port/uow.go \
  f8f9b381c9cdbcabbeea9403e8379c33464b7356522abdb383d0e09a6f5996c1
verify_index_sha256 internal/platform/store/uow.go \
  46591bbf2833b97ce06d9cc1513ca2aadc70f21de05544e836b453d35da51b7e
verify_index_sha256 internal/platform/store/uow_test.go \
  4524e09f7200b7b445cdab73be7ee921d619adc624de3b790f4fcd395017b0d7
verify_index_sha256 cmd/aicrm/main.go \
  52fe62cdda6653e597ca338c4cb9a47605b47fb15c21410f6156f6d05691d180
verify_index_sha256 cmd/aicrm/components.go \
  46387b23db9b21c13806ea3440a6de35a86d65d1497eb9489e29c2cfc3e55085
verify_index_sha256 cmd/aicrm/components_test.go \
  b81bf5c6370a3e89dbd99308d7ad31cdb03e716e76c77f412544ab32318a56e0
verify_index_sha256 cmd/aicrm/scheduler.go \
  4e64f1baadef3111255bdde2b8263e03a1184ae6204fd1b0096ecc72129cc5e1
verify_index_sha256 cmd/aicrm/scheduler_test.go \
  21a7441fcfd1faef6a54636dc937e0b949be262f513b841bd7de62bedac54d36
verify_index_sha256 internal/config/load.go \
  3df220675a71df7c798681c43e0ebc300342b7396fb60a5b867faed787c81b84
verify_index_sha256 internal/config/schema.go \
  ae88cf2e3fc3c90db427d6a5f1586d40296e82796dd9db1d91e08a2ca11271fb
verify_index_sha256 internal/config/schema_test.go \
  20939e0f978d8221ba034919e2bc61a954d415091447ab99c3b5a1def3cbd081
verify_index_sha256 acceptance/p0s01/process_blackbox.sh \
  2b6376c3e061d074d8a94db6d5dc0876ec5a2084978eab0e348b028a381b6889
verify_index_sha256 acceptance/p0s01/static_contract.sh \
  da67ff4f8a2e96b649cfe7940d9c0b96f1b11463f39e5152b8796073200d325a
verify_index_sha256 acceptance/p2s04/doc.go \
  475ea9f04231b77cb8a7400395055f75f9bb80711729b8203393dfba0e817d3a
verify_index_sha256 acceptance/p2s04/queue_isolation_integration_test.go \
  a3190dafafeade5f3583bb3183ea70bb8390dfa2dafcb2789e831609d8bca3a7
verify_index_sha256 internal/platform/jobqueue/client.go \
  dc4aeccc44e26c3ac981a4888538532531d92bb37a14cdff3e4ab780af9749d6
verify_index_sha256 internal/platform/jobqueue/client_test.go \
  e5f0931c6caca82fe125aada38fe5bbc3dd5348eeee22bcd2fe67ee5de568da1
verify_index_sha256 internal/platform/jobqueue/queue.go \
  472ca471345e5455edc29eb863b5c61b6d3377d9c642b06255bbd84d1d453d32
verify_index_sha256 internal/platform/jobqueue/queue_policy_test.go \
  12f2041543ad7da6af1d068067184e50d20c36732ee7e6a39423cc0da7053cf1
verify_index_sha256 internal/platform/scheduler/scheduler.go \
  f0e7d1de129ed19f3ce861475ac641285f2d2c91ad6df3edac32dcdd792969c4
verify_index_sha256 internal/platform/scheduler/scheduler_test.go \
  3da95a71a1d17a89a76521fe76f537ca412645b41f6c264df806ab054dfe3564
verify_index_sha256 acceptance/p2s05/doc.go \
  2f876c4d57b8df9ab8228a55f6329ef14d75cc86f93035ee364cef256a08be73
verify_index_sha256 acceptance/p2s05/scheduler_integration_test.go \
  f5a6b0d41d420a3085529c4ea333afd21ac3367a7d4f7a08ace1273541c36271
verify_index_sha256 acceptance/p2s07/doc.go \
  c9c184fc05b5f37a83bf1bf03497c6d39a77440e22825515dc235cd684709741
verify_index_sha256 acceptance/p2s07/dispatcher_integration_test.go \
  99228a216fb404853419715de60804737c4552726d15a89b6f0c70972b776ec7
verify_index_sha256 internal/events/dispatcher/dispatcher.go \
  da3291b6344074358016e9d03b38651a8652d883caf20e2eb392829b102f58ab
verify_index_sha256 internal/events/dispatcher/dispatcher_test.go \
  0357827909be23f731ff5e3ba13e2e361ce0d7d95df39abb07e5c9a7aa79c63f
verify_index_sha256 internal/events/dispatcher/jobs.go \
  de8a10de9e438a57f00ffd0e904e25e32c37002603b42e75b83d3fdf6eb85262
verify_index_sha256 acceptance/p2s08/doc.go \
  8c7efd59df7d54ec7c2f032beb4da78baa87ddccf874c852a22a9d21218ccb50
verify_index_sha256 acceptance/p2s08/gateway_blackbox_test.go \
  e522f404c6dde64fc0f339af29e92e1352090b914912c8443e2eb5daa1c99956
verify_index_sha256 internal/platform/http/errors.go \
  42ada93ddaa105c3e9d1eb30b0209e151e4c54d421081af998fdb89848821c98
verify_index_sha256 internal/platform/http/gateway.go \
  150b89ff6761bc3584ad58d5c74895209f0a1b303147b2538efa9fa545609522
verify_index_sha256 internal/platform/http/gateway_test.go \
  3668ef9ced90f5bc2e859b2936a0219b1d02779dbcbc7dda1146de2546dec6f7
verify_index_sha256 docs/execution/slices/P2-08.md \
  f9c10d171a4256764945f333ba2c591aca6c3824938f1980d849403b15b9e7f4
verify_index_sha256 docs/evidence/slices/P2-08-http-tests.md \
  a61b50a0d040373515216d96fe1707a14be7a9852e8a1b9e64ba94c07ae35fcc
verify_index_sha256 migrations/00004_auth.sql \
  777e6634e63db30a4f0ab2e3c17afd0cc98235863753a74efc4871c379e797c3
verify_index_sha256 migrations/00005_contact_core.sql \
  226c660db0a572a97c23322b6f1dd0e694f210033d7ab4992b59bd8349ef0432
verify_index_sha256 internal/auth/app/service.go \
  b4af7c3a3a3592c5eb0f98ab34951f5f9b56155e1991ee0166635ab908fc4eff
verify_index_sha256 internal/auth/app/service_test.go \
  05aba70a7f1f0299fb6a3a16f859cdc7243a4b09c4869029e5d6d1141a634439
verify_index_sha256 internal/auth/http/handler.go \
  2859f6e0215a486ae4c0e66551f6a2096e1e6385b56fd9c4ff99b00d2feb611f
verify_index_sha256 internal/auth/store/repository.go \
  9b84ab75217f3ae6c1b1c69d9a54e889742a7fffd074c42237d47774cdbbb966
verify_index_sha256 internal/auth/store/queries/auth.sql \
  e1297b75be5300fc4d096c05f8f0fa995013093d03243f5c9746e6cffd1487c1
verify_index_sha256 internal/auth/store/generated/auth.sql.go \
  fd2ba3dcf9fb1cbd3475197b08753e5050c5beddffe777c3bf4f29dcf6c2d112
verify_index_sha256 internal/auth/store/generated/db.go \
  121194f70f0c6bd5ff393988502dac16cb3ebd421032d65a6de2d8f176c2f832
verify_index_sha256 internal/auth/store/generated/models.go \
  8ad356407329c9294431184c7e4f75c836c185912bbb7fc90b61c49009ab853e
verify_index_sha256 internal/auth/store/generated/querier.go \
  6640f435a626fe69a363ea4fa95aef8cd1dba8698aa7ad235d603e4465ee2bdb
verify_index_sha256 acceptance/p2s09/doc.go \
  719f525dc416a2d9c7e6360a2446519b641b8f17cd2a56d521642dac0a67da0e
verify_index_sha256 acceptance/p2s09/session_integration_test.go \
  b619aacb5c9d08f63779cc78a3e662f9f27e0ce5478d97cd0b4b8eca0531a445
verify_index_sha256 docs/execution/slices/P2-09.md \
  bd75e25e956d1f1abaccf5e1e4b4e705bee5981f2908c33626193d7a86794160
verify_index_sha256 docs/evidence/slices/P2-09-auth-service-tests.md \
  5edbbf1d8c4d10761a4a91bf2e2c8cf7206be786226fdb889ce481e049199f36
verify_index_sha256 internal/auth/port/port_test.go \
  620ec5bf1ea4d8a6cf2eebade0b31871976962888aea1bdb6f0571aacec27272
verify_index_sha256 internal/auth/app/policy.go \
  1773a811136e7bc61bf82619c7a54d7cf71f5e0bc4a7de87464b26ae4241e69b
verify_index_sha256 internal/auth/app/policy_test.go \
  aa048c777a2d088c0e40fc577a69ffafbf0032838b133c0ab291ca6570d15868
verify_index_sha256 internal/auth/http/authorization.go \
  acd3c1c15a5361c1023115ea339be68636c15911832a13f711765e4b0768452f
verify_index_sha256 acceptance/p2s10/doc.go \
  23f3f43f387ff3fc929b5bec675c667c81c3ab77ca6397a39733fe8b403747c0
verify_index_sha256 acceptance/p2s10/rbac_contract_test.go \
  af33a1f7423c177865b25fc45b960a01a8e074b7f03ae08f7194bd3eaff7dcdf
verify_index_sha256 docs/execution/slices/P2-10.md \
  0858a75f669f890e77cfd6e4aeb9a7218ab5e2f980f6370a9f00283296d63e54
verify_index_sha256 docs/evidence/slices/P2-10-rbac-tests.md \
  be0c22686771222bdcdc3350760365a30397350915806f900e212829eca2cab8
verify_index_sha256 cmd/aicrm/api.go \
  963256cea0c5268b207c08457fee3a3ad49d28f1354dd7d45316122147b2ca38
verify_index_sha256 cmd/aicrm/api_test.go \
  c8e0ed59f3758867a869f08a7b4cf36f766cf24831da23a1ba3b137985244ab5
verify_index_sha256 acceptance/p2s11/doc.go \
  735a2c1eb929a5046d53d60a522b9b46f9c822dc20c85846eb358d2b80f15a5d
verify_index_sha256 acceptance/p2s11/gateway_router_test.go \
  b06debad2b4cf25446d25e389f0ebc58e02e8cc7678c090a5829db4633f29956
verify_index_sha256 docs/execution/slices/P2-11.md \
  621f0fa454d672f0bab5f8589d737db91e1b3a7137407a680cc141fccaf7f34b
verify_index_sha256 docs/evidence/slices/P2-11-gateway-tests.md \
  db10c68cc987690f3a812ce5966c499597fc90aaafef9b4b04cdd0dd6eba1be6
verify_index_sha256 web/src/main.tsx \
  7e4f63facdcd6427fb294c704215c8419d4d0e92adb77b04a652e569d99c69d1
verify_index_sha256 web/src/main.test.tsx \
  d1f1198b5a173eb51982c5a9c84d473507c41d63755b2c84e0f4853236924a18
verify_index_sha256 web/src/customers.ts \
  d5171da808f93014f46f4bc374b8960a3879ebf2f44ece604fa3b95907726c23
verify_index_sha256 web/src/customers.test.ts \
  920448d8b3c3b68280fe7fe5f723da2832a78e373f9cb5546b4e3071467d12d4
verify_index_sha256 web/src/customers-ui.tsx \
  90946468256ffda9423deeeda06cada0f671f4018ea4445690c59585170815b5
verify_index_sha256 web/src/customers-ui.test.tsx \
  6a3b4b10d5dc21ea9beedb540de3c87e479a27c2b8a044ed0b754bc9afcffebf
verify_index_sha256 web/src/customers-list.css \
  d9a64cbdb9c2d1a10de699e28fdb00515a73c38f9d52d254e37a16fc89d2632c
verify_index_sha256 web/src/shell.css \
  993a7d533476836bab13f051f2a063d60e4f513224d355af485616de04bb033e
verify_index_sha256 docs/execution/slices/P2-12.md \
  3eba447aab3854dd1ae2b27df5e2167de9b38e7c5d0996aa6a64a1d524b5c8f7
verify_index_sha256 docs/evidence/slices/P2-12-web-shell.md \
  5d84f724094413672de6f6861e45542bf3b6d12eb39a04cbe06846e09eba0f17
verify_index_sha256 docs/execution/slices/P2-13.md \
  e649a864beeed46b2ba604c33e2ae70bb48f083fdc39c62c7ab472c00e02e007
verify_index_sha256 docs/evidence/slices/P2-13-auth-ui.md \
  d647a78aa84bddbeb6416cd2b92ed6dc5548b1c3e6e0d357153f2868e2000c3f
verify_index_sha256 docs/execution/slices/P2-14.md \
  48c6fa497cb10990fd3198843dd202c5d1b6acb704ae6da5a63bb747c43f852c
verify_index_sha256 docs/evidence/slices/P2-14-stages-sqlc.md \
  54aa76745fe22675191cc6df9bb669e8230a8976f007ce1c1059189918dd99c1
verify_index_sha256 docs/execution/slices/P2-15.md \
  82a047eb827f129d01eb329bfd95ea751a30c3526e6247698189f4af047e6386
verify_index_sha256 docs/evidence/slices/P2-15-stage-service-tests.md \
  7c086a4d95da8440a60970d1e174ee5f82053e60a9d7f9c51aa79ec3172a0bd9
verify_index_sha256 docs/execution/slices/P2-16.md \
  7ec39c6175b3eae4a047653d367d4b247276bc558c052bcbfc03d92b0d15e092
verify_index_sha256 docs/evidence/slices/P2-16-stage-handler-tests.md \
  0a69becbba6446898f62ada1815b9f29d73d869fd18f12467f637c0ccf07d70b
verify_index_sha256 docs/execution/slices/P2-17.md \
  e264c335226ac1b883c9d0c02098521a31ad3742c8e7649f5c0ae99136982adc
verify_index_sha256 docs/evidence/slices/P2-17-stages-ui.md \
  244927f511b6d2bff674ac8fa806c21c51ce8ae67d55716f4f07241ae2f0e19f
verify_index_sha256 docs/execution/slices/P2-18.md \
  12d12a92f60ee6b4bde7db7eccd0c41d1b000f57058c44b97db511b2763e487b
verify_index_sha256 docs/evidence/slices/P2-18-tier-config.md \
  51b53dc5e09aec8693f61dac4263993e5e1a8bb7cc48f9886c35d8c86217464e
verify_index_sha256 internal/platform/deployment/tier.go \
  a179baf803251c646a43119526eb7de4288aeb72511d0ca0aadf96c4e55436e8
verify_index_sha256 internal/platform/deployment/tier_test.go \
  f84f48aeefbaad25fbc4abaf99e5b41a6ebbaa7a05623b4b3dfda317e0467686
verify_index_sha256 cmd/aicrm-config/main.go \
  ab12c6c131a67bbfd2b651d1cd815124eaef09508bc32787e42369824d782ed8
verify_index_sha256 cmd/aicrm-config/main_test.go \
  56580426c014a8ed62cb9774629ae3468f55b5ed941df3f7600e892df1e32283
verify_index_sha256 deploy/compose.yml \
  1a5c68290299aa87ddbab9485e293c21b5883b120f621051132a39204b2fd9fb
verify_index_sha256 scripts/staging_deploy.sh \
  6cc71be7a0dfd8362ba25fe9bfc9ed8b68bacb896093476a75d846c6c3fbf880
verify_index_sha256 acceptance/p2s18/test_tier_config.sh \
  ab73b731c77453c6188d6e2544d20be0e9563c67eb4746eb52ab7de5e20fafd7
verify_index_sha256 web/src/stages.ts \
  3c161326e176da892546b860d027bd86aca5743ab3c680666f4697645030569b
verify_index_sha256 web/src/stages.test.ts \
  f0eeee59c943e35c19e85d049573f3ff72f96d3e7e3cea841c5ef379c2a53f86
verify_index_sha256 web/src/stages-ui.tsx \
  56883ff280af8a19ffa3e69e79e34972ac640dee2dfd2eba04afc132cceb5513
verify_index_sha256 web/src/stages-ui.test.tsx \
  27a5be752ac1312c866f88ff5482aafb7033373f3a5ba7c26324f8036c076af9
verify_index_sha256 internal/auth/http/authorization_test.go \
  f7512b38ffec491002262026b971d87dbae994b9e9b374b1ee699c7385aab9fa
verify_index_sha256 internal/contact/http/handler.go \
  63e1808d7e5e72022f0b7ec6a9a50537ad5533e37629b31abb5e68c3d589b45c
verify_index_sha256 internal/contact/http/handler_test.go \
  b7fc528e0203ede77e44926cc98b0c2620da1443e0da580ec1c6547231b9d5c5
verify_index_sha256 acceptance/p2s16/doc.go \
  990e3e9960d3f5c6433c5924b58883df7dc6cbec8593d0dee61098d207004fff
verify_index_sha256 acceptance/p2s16/csrf_integration_test.go \
  56e176d23dfcbfeb4d46ebfd036e188d92019c5b2aadffbbe0e4e11dfb9e2cca
verify_index_sha256 acceptance/p2s16/snapshot.go \
  4269cb5cf53237180cd6db8241b2a389b12e0a094349a5300372189ee56e1291
verify_index_sha256 acceptance/p2s16/snapshot_test.go \
  8fa2a8feda9ebda657e0e72ba7e0cc1bfaec56b8a6348d7f5751d5e59d28659a
verify_index_sha256 acceptance/p2s16/snapshotgen/main.go \
  701d151613f783b461d801ccb1a34824b84fb6f701e1b7d190c00bed56707fe5
verify_index_sha256 internal/contact/app/stage_service.go \
  9c5ba8595f64ae5b5a32f583338d58a6bed6cf1ec3504ba8ec26744e65f1b570
verify_index_sha256 internal/contact/app/stage_service_test.go \
  37e8ed0280ca4b1d1e92a21ec519ba7f3d0bd798f6fb2ff2c0797dcf98aec6a8
verify_index_sha256 acceptance/p2s15/doc.go \
  abbcb37df7d455dee12597ec062edee974a15e90d6eafd44624cbf2f56873802
verify_index_sha256 acceptance/p2s15/stage_service_integration_test.go \
  feb2004d7cead51fe5016d19c87af1c4439bef11f0bdd8c105191711f4471962
verify_index_sha256 internal/contact/store/queries/stages.sql \
  0a0ecd3338cabecf50261d114a6036727e29dd87de01019ff7513f8b002162ca
verify_index_sha256 internal/contact/store/generated/db.go \
  e3ef23479f44c12b0c868db745a22448a5d14cc7e4311ef4b2d2652bd1aca0a0
verify_index_sha256 internal/contact/store/generated/customers.sql.go \
  48d489f58b6517e531eaa0c16f0d1b40f6462a8566b52b0e9429ccc62da73793
verify_index_sha256 internal/contact/store/generated/models.go \
  66dff9cb34c2a7838f15407019e583e70ddac87a9c0ba233fb746d7d830e638d
verify_index_sha256 internal/contact/store/generated/querier.go \
  c73e6425f6b34935f02d01abff6653b31da5f828bc79939bf2a984c77084207f
verify_index_sha256 internal/contact/store/generated/stages.sql.go \
  24abe8b30311c9a7134c8daab59b487caae03f72a6d1ab50d587c536eb5046f5
verify_index_sha256 internal/contact/store/repository.go \
  e3ca8720cf6789a7b6aab4603513622b7feff27a42fb2f86ca394250951c7bab
verify_index_sha256 acceptance/p2s14/doc.go \
  372bee4c9610e5cd4ab9696e2efe65fed8fa42868e3e86ab632a8883d7f09869
verify_index_sha256 acceptance/p2s14/stages_store_integration_test.go \
  c9768333556d987e7e01c522d2229362438114900af5191420415ca8b9d40143
verify_index_sha256 web/src/auth.ts \
  61a038e04583826f405dd5cad2b4fc43e4dc9fab9317a1bf8e5c58cb14a3da11
verify_index_sha256 web/src/auth.test.ts \
  9fae694ec613b82253c3f573867d9d38086638f6061797b3604ccffe91501f4a
verify_index_sha256 web/src/auth-ui.tsx \
  cd44efe0eb1216921df18f4f508088a12004a97eb4f148698e46b776f9b6cde6
verify_index_sha256 web/src/auth-ui.test.tsx \
  a45b33d9bcc123347b5b6878b16c79fbc8b390798fc423efb82fd0b9582b1ddd
verify_index_sha256 docs/execution/slices/P2-07.md \
  4788b3c37af60d5b95704b0e8981a86904575112a635106821a1d180ef8f0c91
verify_index_sha256 docs/evidence/slices/P2-07-dispatcher-tests.md \
  81000988e5933b412931f9797949823afc8d5855cf3a76263ce7ef443dd092d7
verify_index_sha256 acceptance/p2s01r/doc.go \
  34dbf4e86ad0f890aa503a84b8c429c692b15678f8df20adebde86a698d4a12b
verify_index_sha256 acceptance/p2s01r/uow_integration_test.go \
  15522642151bc0e7e8f4b45985066269b0a8a66d3401662c2cde86edadbc33df
verify_index_sha256 scripts/check_feature_matrix_contract.sh \
  6aeeed7538c430b1d18861fade94ba0428fac5b3c2c2f6493e51bea08e3d178e
verify_index_sha256 acceptance/p0s10/test_snapshot_gate.sh \
  955d136c6cb6bf873127e78dcd4d137ecd19e11972fffb30af85260150ac7e3b
verify_index_sha256 docs/adr/ADR-001.md \
  dbf5a99d7e5ccc0740e8a32a9945a355ae64a629ca1d4699fb3393890a4bff6b
verify_index_sha256 docs/adr/ADR-010.md \
  5fdbd62214938e6485322a84858c488a8bac812e00f312318186a5b3ec9b72dc
verify_index_sha256 docs/adr/ADR-011.md \
  3fb1954942b0de9da1989276d535af090c0dcd22841437dbb7d6e49e54b7f92d
verify_index_sha256 docs/adr/ADR-012.md \
  a4943e7a665309a388ce4ffad7d4a7610f2038191b9d5d9483b2370d5df0dd4b
verify_index_sha256 docs/adr/ADR-013.md \
  3924512aa2718298d2013387979397b5bb72a4ed0dee520fd0e2030490a1de2d
verify_index_sha256 docs/execution/slices/P3-R3A.md \
  cac248d380f467833560cbd3992a86100c9f56594380bc0fd013be241c6614db
verify_index_sha256 docs/execution/slices/P3-R4A.md \
  8b0cf9837e00532321228d45e7ab872501f97f23109335e776e4b33ae300246a
verify_index_sha256 docs/execution/slices/P3-R4B.md \
  3ee411d7d57ba4001dadffdf7fe617c1bc91400341286c44173db6d82ad0297a
verify_index_sha256 docs/execution/slices/P3-C07C-R3B.md \
  271a5dc048cdf85699464af6ab0f570aa64b5f122fa9e46ba1ce472b042542e1
verify_index_sha256 migrations/00011_contact_external_event_idempotency.sql \
  eb9212ffabda2e1537809c5db0b2ff61721d6a970a5f24aa913302d45478ec77
verify_index_sha256 acceptance/contact/external_event_storage_integration_test.go \
  85f80070808498213d0f6dd07dd7a8039693c56d6e348ac9dee33ac30c712216
verify_index_sha256 docs/execution/slices/P3-C07C-R3C.md \
  fe3ceda4239f18f3007a26e1f499284a01c5be5793fb999159e14a3927e0189c
verify_index_sha256 acceptance/contact/external_event_behavior_integration_test.go \
  bc3761383b4d3614c1aa5d9c44b01597fa578b4bd0b5abc2e556661eef3e6c13
verify_index_sha256 docs/execution/slices/P3-C07C-R3A.md \
  2f802992b78426238bfd41d3946daae550923279a93f67f89a91ec1d6a9eeb0c
verify_index_sha256 scripts/check_slice_ledger_history.rb \
  1fe3b93ce6b021a9e760956324bed957bd3635662fb686cecba59851a6ecc582
verify_index_sha256 migrations/00010_identity_storage.sql \
  bc72451450a9efff3435c17fd17d2f457ef909a7c7390e7468fddc7befe68aab
verify_index_sha256 docs/execution/slices/P3-I3R0.md \
  98bd9763ae225b74ab215032e9d433611fadb6f7453c64149beb6a90d747684b
verify_index_sha256 docs/execution/slices/P3-I4A.md \
  67b4d5ef685fc15104edaf40767664f1c441ae82b38e778f47e6762371bc3478
verify_index_sha256 docs/execution/slices/P3-I4B.md \
  eab53de36d28d54fecd09cb284074d5665903883a00a4f259f8ffdbe4900664e
verify_index_sha256 docs/execution/slices/P3-I5.md \
  0bae282fec31140c4840908e21310288b93fa0f9e1c1540ce6a7e0e9868ebb5a
verify_index_sha256 docs/execution/slices/P3-I6.md \
  30eec7572c47b0d7190f4c8259ca8090d6d5030916c343045072e6df5c3e5a86
verify_index_sha256 internal/identity/app/ingest.go \
  a65530f14115676e0d5fb498af4f8683f1cd370389badfc6902eeff7d88290ec
verify_index_sha256 internal/identity/app/ingest_test.go \
  b134c1f41c8ecd051ae879f3e87bb7cc24cdfb8f64e922077641553cf7d60d32
verify_index_sha256 internal/identity/app/replay.go \
  4908d38aafe6bc609896df1145dcb6798a4dbf29d2996f4d43f54282fdaf9f77
verify_index_sha256 internal/identity/app/replay_test.go \
  fe5e2579ba8027053b3bb8210b9a40a6b37e3987742336c9f571d7cc1797472b
verify_index_sha256 internal/identity/store/queries/identities.sql \
  fe60a77a68c3213c1d218e156d3724e8011519517f4cfb9c937088046de62078
verify_index_sha256 internal/identity/store/repository.go \
  fbecf892a9dd9b963bd5823c44ecc2fc7431df312bae56f1475d7459ac5688c6
verify_index_sha256 acceptance/identity/replay_integration_test.go \
  1443486053341115224de0ad7d231d1ba900a0a5927d64a1d35ac4681dffe5f8
verify_index_sha256 migrations/00017_identity_pending_event_payload.sql \
  c5de70f3060c42535c1c8ee851c0cd82122d9f2636ff1271e4bb7679b41d877b
verify_index_sha256 migrations/00013_identity_receipt_completion_transaction.sql \
  9199103badc244cc03e850ae2786ec94e8b9def50bdf22a7ff62cfb0c9322091
verify_index_sha256 acceptance/identity/storage_integration_test.go \
  7d1ee4dff2f5722348df60b3bb5c02e58bbfdc2fd2943fd4a14bd527ed684851
verify_index_sha256 acceptance/identity/ingest_integration_test.go \
  2db7cd28bbb86dbbe7d3d3cb91b80cee4d49f42c3d8148ff11c50c71a7f1ef3f
verify_index_sha256 acceptance/outbound/o1a_r3_integration_test.go \
  7e148c8b86169c82cbb41fcb9d8fe6738b36da3f5ace3d80923e9a63b9f1bb28
verify_index_sha256 migrations/00020_outbound_send_attempts.sql \
  f0e1c607b8fd91894ed9ca17c35dad540b41e9efca86f469e5a9131cdeb94e6f
verify_index_sha256 migrations/00021_outbound_task_status.sql \
  6bd8b3578bdfe189e7c2578a0b76fe3a8b4db542fd2b2e4cdbe749c8c1b06c1d
verify_index_sha256 internal/outbound/app/sender.go \
  7abaf623430c7e588363c336e5193f45ab2d1f2bef42acd1383afc0416392aec
verify_index_sha256 internal/outbound/app/sender_test.go \
  73f713313eebc00958fe9a6a820e2faa536f51d7ab0774f85ce91046c972a9af
verify_index_sha256 internal/outbound/store/queries/send_attempts.sql \
  5e46a7088c0783721934926751c18ed1c4500f8f2f59c2ee278d761b7e93ba10
verify_index_sha256 internal/outbound/store/sender_repository.go \
  c62b609810b498ef7b234b38acf1b3a013b6e9b4daa879c5105930447dd973d1
verify_index_sha256 internal/outbound/worker/sender.go \
  a0ba54bf303a67d4e828c9b3391a46a04b3c4582562bd552a357b95a8b69ae1a
verify_index_sha256 internal/outbound/worker/sender_test.go \
  5165472be9b62647dd2a2101215bc7e875ef043539650bcfd2b9cecf28b649bc
verify_index_sha256 acceptance/outbound/o4_sender_integration_test.go \
  b2ef6401040730d09023d9eaaa4fa9f408079159bd25b4639194341c4a68c747
verify_index_sha256 docs/execution/slices/P3-O4.md \
  545e6a8b95f27d26c0dcc03022b4799ad80173c157fb65f1aba69ba447e83f89
verify_index_sha256 acceptance/outbound/o5_status_integration_test.go \
  2fe6cdd0301bfe969330b86c0abcf1ae04fa2bf77450de245fa121b11044f004
verify_index_sha256 docs/execution/slices/P3-O5.md \
  73aa5438e583b2fcd1328bdcd51df034f609a2ab15ac16638d25fd776c22e992
verify_index_sha256 migrations/00022_outbound_send_attempt_history.sql \
  9de1d35b524539e5b3206b6eda53d52e56cb0e0bce0f9a52d1a4429061225c9f
verify_index_sha256 acceptance/outbound/o6a_retry_integration_test.go \
  8f2edb3f7c233eefa408ba0d3a496c68cf2a3b23ad4a5894a28b8efeffc670f5
verify_index_sha256 acceptance/outbound/o6a_migration_compatibility.sh \
  8bc0c28e8fcaa8ac46d911976c37efc50cd0e66423c493eac9a8376228ec5586
verify_index_sha256 docs/execution/slices/P3-O6A.md \
  45285f6d0764dd978eda613167dba6cb498cf796031664f4d0a72eb9474847be
verify_index_sha256 acceptance/outbound/o3_integration_test.go \
  b1478b702e9766fa67117c26bfd345f4d139257c70b800651e7103fcc007c70d
verify_index_sha256 acceptance/outbound/o6b1_cancel_integration_test.go \
  27b8cf4f6103fe0f51750122ed471355d6d1e85a441c71d6e7b8ae298f29f315
verify_index_sha256 acceptance/outbound/o6b1_migration_compatibility.sh \
  34a2b3b2a3799689a9b8708866ef1e2542a0ea32e54af5b13ddd68fb608d3012
verify_index_sha256 acceptance/outbound/o6b2_manual_retry_integration_test.go \
  248042036f6e91eaa03202b45dc5677cb2f2cbc26b74d1996a39d3993fa1f52a
verify_index_sha256 acceptance/outbound/o6b2_migration_compatibility.sh \
  b0bd2b2457d29af18ea5dad25f0ee759beccd6048a9196c43fd417f756c7fd1a
verify_index_sha256 docs/execution/slices/P3-O6B1.md \
  5036062b03bd5858c060fb3a2da4b11a73cf15201368fe471cbfb86067d5fc16
verify_index_sha256 internal/outbound/app/control.go \
  c35da34aaad2981ab4a80fd2033290b03f1b02f905fed4988339beb658f4e841
verify_index_sha256 internal/outbound/app/control_test.go \
  f2e00376654afd349b7823d458c1edca6a85d023c175b381041c0959fc3d795f
verify_index_sha256 internal/outbound/store/control_repository.go \
  21f56563e320af9f2ccbe6ac2be44cc2e1f0e6fd344d27180054d8d85bd12003
verify_index_sha256 internal/outbound/store/enqueue_batch_repository.go \
  d0d2e18b1312b2aed636fee4c5f6edcf0274159e64d4dabb147a4844c0db2153
verify_index_sha256 internal/outbound/store/enqueue_one_repository.go \
  d65b0edce44263d656e8a7e2449022aebcb20bc0a345c25d8467cd28f5cd3bd5
verify_index_sha256 internal/outbound/store/generated/control.sql.go \
  8b63c592c3680cd2a71f64c3465282a5d88288e128c586bb2d7360e07fce0d43
verify_index_sha256 internal/outbound/store/generated/querier.go \
  32331850f514219e3623eb7ad25176efe5720cd4893cd5e96168ab408f9e3261
verify_index_sha256 internal/outbound/store/queries/control.sql \
  73c90eceda5179641ccbd34cd471394daabf760576c319af3046e513550c2c8c
verify_index_sha256 internal/platform/jobqueue/outbound_cancel.go \
  738bb5032dacfd142bf26aae6ce9234b65d741d57c631947e003233d8dd591d3
verify_index_sha256 migrations/00023_outbound_cancel_control.sql \
  4c498d745892f102678fff2315a935a020531edff949c71bd58e69fb4044bf22
verify_index_sha256 migrations/00024_outbound_manual_retry_control.sql \
  ae1939b3612fb69ab591f11c3889ad7fe77db35cc1bd98352cb1e92b87627879
verify_index_sha256 internal/outbound/app/manual_retry.go \
  5c9d4870c2cceeec02eb7026844f77beca2923f1c5e6096fb6ed503e7a3a2b66
verify_index_sha256 internal/outbound/app/manual_retry_test.go \
  f9693b4918740591f37d02f9bf69e727b87580e734d9c6d158b24be51c11941f
verify_index_sha256 docs/execution/slices/P3-O6B2.md \
  3e88d624ca8091c2c862daeef6a81852cc7f379d5036dd96068adbf004efc66e
verify_index_sha256 acceptance/contactfixture/contactfixture.go \
  ff13575d17ec3f882256b328f111ad41683e4d012e10e626794760f188e8d990
verify_index_sha256 acceptance/contactfixture/contactfixture_test.go \
  3d3ce197ee6cae2b9ac7a7c19f25e0d8f3f6f78d7f8a6cf8676253e1bde68358
verify_index_sha256 acceptance/identity/doc.go \
  efc1a6ec165ac8844b0af5446db2da87a46e9506761aec24827ae8efd700b3bf
verify_index_sha256 acceptance/identity/contactfixture_import_test.go \
  e550f00216351ab1dc162484d69c65b58c9945d05a42313a1153bfceb326a91f
verify_index_sha256 docs/execution/slices/P3-I7.md \
  9d42808542676d39bb6880a8de37bd86edd2e0aa82cdc71fad9daca7247632a3
verify_index_sha256 internal/identity/app/review.go \
  593721b7dfed15d0a5a7ec33d6302e2a61cf0440c6aceb9b1d2c51e964a8d2bc
verify_index_sha256 internal/identity/app/review_test.go \
  5a20a68b462343945cdbaa7c76dd6e5b3855a7086e4c0e374f3f2afe47b2b06d
verify_index_sha256 internal/identity/http/review_handler.go \
  6d35b0ee53f0fc737977f43ee8274ed47d35f5043aa0a1766fac17dba8886a7d
verify_index_sha256 internal/identity/http/review_handler_test.go \
  114752fad95c3503243e95c1f7f490ef599fc56916890cee5f4b8e92a95544c0
verify_index_sha256 acceptance/identity/review_integration_test.go \
  962260908d20ba2d2f8cd346a7a128f7a8414bf2582b99c0d64571a6f86e67be
verify_index_sha256 cmd/aicrm-river-migrate/main_test.go \
  b5ee3df7e6ab50818ed80d77782c3e89b78d310fdf9479824bb34d6d25a476ff
verify_index_sha256 docs/execution/slices/M0-5.md \
  c5a4f1991b8f3ecbb1a3a024c6131aea5fc3d6813ddeb34b323faf2948229609
verify_index_sha256 docs/execution/slices/M0-6.md \
  96f5131c60d2eec508557f03ba1322af88c2002a259ec8d455569024d2013125
verify_index_sha256 docs/architecture/canonical.md \
  3f1a95bd511402b7c08c35c395bd0e5e60bfcb66c007aa0708750fea0ef4c568
verify_index_sha256 docs/architecture/table-ownership.yml \
  37bf353ca92330154417d99f743328240878036f17d7454a942113b9d40f8407
verify_index_sha256 scripts/test_repo_contract.sh \
  9c19b0f34cabea978fa8e0db9cf03832862200b38c095ebb4f73249d52a53839
verify_index_sha256 migrations/00018_segment_crud_receipts.sql \
  da96a6be5c431220d4f117405839f2d69ba682a34df14c2dc7f5a41b7b1fb5e0
verify_index_sha256 internal/segment/app/crud.go \
  32c71c027868e01b003366e7a30e2e52ff0ba5e0c059cac8074947917adca395
verify_index_sha256 internal/segment/http/crud_handler.go \
  ac2a8fedf6390c31baa7b79f7e51cc9beaa9508b9f02128b0158faf865c11415
verify_index_sha256 internal/segment/store/crud_repository.go \
  d1270756aca4f67930f476480f5a921ad7384354ed3df83f1a3e9fcd001f9d64
verify_index_sha256 internal/segment/store/queries/crud.sql \
  6444d97a56c3a3813d3f18ca1fd27b90283ee87a0c06334f98c39827173b26cd
verify_index_sha256 acceptance/segment/crud_integration_test.go \
  3817464e34cad386bf841793529b5a9c8e17f2d6835857f1d8312bae4f6068d4
verify_index_sha256 docs/execution/slices/P3-S05B-R2.md \
  1637cd5a6490c64f30734c42988921655e093c40f4633c181082d2728f6c7e73
verify_index_sha256 acceptance/p0s02/static_contract.sh \
  8acee6eaa7950a0d8c315f7eebf4b4d17f09adf7f75f883514cebefdb99b38a6
verify_index_sha256 acceptance/p0s02/test_static_contract.sh \
  1d3c89bdb0dffabf298777965896128c2fa4116b134deacf8c1499b18a45c7a2
verify_index_sha256 internal/platform/store/contract.go \
  747683b0f430da2ee29f001abaebe5fe621561aa3dd99b5b9db6b7d871895165
verify_index_sha256 acceptance/p0s03/query_contract_test.go \
  44184be1c39ffff0e825d0888dcc9d62e5993cfe1958861d2ddb50323ce536da
verify_index_sha256 acceptance/p0s03/source_contract.go \
  239802f1fea13e0640ca4e3d1eda8f8428f8393f2cd4919deecc0d6ab311cd79
verify_index_sha256 acceptance/p0s03/static_contract.sh \
  313bc75b5730733b0cb5654a669a5130c7dd36c8cc3a5de99d14b124f7741100
verify_index_sha256 acceptance/p0s03/test_contract.sh \
  8e92a83915ac4a068408f5e562c50d7965efd7c0129e41a0e7fa38769567f6d8
verify_index_sha256 internal/platform/river/contract.go \
  f03a64b78f9fa0f809b869a7d473f42a9edecc41201805fec461f4ba0f1cb292
verify_index_sha256 acceptance/p0s04/contract_test.go \
  ebdcdc727519cd18219908a69571452a0c1ffe5751e3f0f392bff805b79bb102
verify_index_sha256 acceptance/p0s04/source_contract.go \
  8d02d5d5fdd76a31999ed76a9eb77d8343b09d4a0cd48c3531f893d29331f9d0
verify_index_sha256 acceptance/p0s04/test_source_contract.sh \
  5d92037d3a54cc450b7862d08d60015ac9b1a776d11230d1d373d0169a516697
verify_index_sha256 acceptance/p0s04/static_contract.sh \
  8cf9ee7189be9a859c660301a5a801cef50495cfcf394d1ed6854b4e73a39c5d
verify_index_sha256 acceptance/p0s04/test_static_contract.sh \
  8b53e98baeba0c106a489ad3ea9e3b65d618433a3e740b4109ec032e0dd6b97e
verify_index_sha256 docs/execution/slices/P0-S04.md \
  bb8cc8f7ff0ef4d6e76c8c124bafc661f562a230ec25826b8318a82311281608
verify_index_sha256 migrations/00025_automation_tag_trigger.sql \
  2d4922edaa830b1d5bb86dbb472c4b8e3cc30d235430df31656f3afe39dd5ce5
verify_index_sha256 internal/automation/store/tag_trigger.go \
  ed0205577c8339e5efe488a271b8ffa63b29b36aebe090dc2bd039d6af659191
verify_index_sha256 internal/automation/store/queries/tag_trigger.sql \
  8b6b2407f8bae5431a4ecb744410a93cb71a557c0efa967a4c7722c4570c7dfd
verify_index_sha256 internal/automation/store/generated/db.go \
  2ba99cbc008ae07cd9d559bd66e606a02e91a4a6c36e70d6890ff9fc2ef8dc40
verify_index_sha256 internal/automation/store/generated/models.go \
  fdbcd12fb0fd611b9eebdc81d44d2e6bc191583baba08e66348741c3b62f5221
verify_index_sha256 internal/automation/store/generated/querier.go \
  2eb9157ae561e1f32ec5dda8ad1b57ffc13f2f2ac35167708328147276bf2749
verify_index_sha256 internal/automation/store/generated/tag_trigger.sql.go \
  e368abf5dab842a41a790315ca8437937056b2c9018a1db89935feeed433b73d
verify_index_sha256 acceptance/automation/d01_integration_test.go \
  559edc55f61b63d38ac5754b0f7f8e1fd80e1bf02d3518a26388e4824edb1b57
verify_index_sha256 acceptance/automation/d01_migration_compatibility.sh \
  c3c594c95ad71da900b5591a55ee3e2897322cef8ffcc95526fe2d9704e98f31
verify_index_sha256 cmd/aicrm/legacy_automation_api_test.go \
  99823a1e71fb137d2a6c3709199e7c5cc540ec003c45556d5ca3e2034cf1d91d
verify_index_sha256 docs/execution/slices/P4-W0-D01.md \
  4caa2bd1343a63e5ee8395c8055486187f83e96000ebf83ecdf9b7ccead4e714
verify_index_sha256 scripts/verify_repo_receipts.pl \
  d28a528cfc1aa8d8a5c6fa62b652699059bb632e8640fbd868ba0e3967881e27
verify_index_sha256 acceptance/auth/a01_migration_compatibility.sh \
  049724b644bca1a8deff52191b2ae4cc4cabd0c72c413e2ed3ce7cba1d29e49f
verify_index_sha256 acceptance/auth/postgres.go \
  97a91d32f1d192bdbd77269cfdb58088375a7033ae67bc13470122d8d070b072
verify_index_sha256 acceptance/stats/l01_integration_test.go \
  5fdcca1ac4021563b428c08a1fc65eedc06d63f20ee7ef888fa5e6c214aec04c
verify_index_sha256 acceptance/stats/l01_migration_compatibility.sh \
  330e4987d26763ea8c8488f8b8007ef3427f9fb6b1248e2633611bbf1c02e645
verify_index_sha256 cmd/aicrm/legacy_auth.go \
  3475c768c2385ba02065f6ae11551279584a3fdfc6acbb8cf6cc357b1da8edbf
verify_index_sha256 cmd/aicrm/legacy_auth_test.go \
  969990053fc5b736d8481a1794a055216fedb89a26004af6a527e11cf3836a61
verify_index_sha256 acceptance/auth/si00b_migration_compatibility.sh \
  9a14fff10671d84b60a7e5a45c676170503dd348f5bfd48ed1436c0d7b3bc574
verify_index_sha256 docs/execution/slices/P4-SI00B.md \
  1391ffffb60cddcbaf04a2c49bbbbb85f9edc5fa159603210c98f05d01e14d59
verify_index_sha256 migrations/00028_auth_wecom_corp_id.sql \
  e93ad5201d446c484f45544fa596b4b0e1b3587e89c9ea03a7b88f5e498a0d5f
verify_index_sha256 migrations/00029_product_catalog.sql \
  3dab3890ece5afe54f2c383ec21a72c2292e0594a1c3f447b8f84c7bf9413e41
verify_index_sha256 acceptance/product/doc.go \
  5e6931d2b2be2d907090ccc9de710350fcf2c07cfbcf56b6d22f4a2d788373f6
verify_index_sha256 acceptance/product/i01a_integration_test.go \
  4cdf1057e04ca803c2ccda0e5fb7e97f134a103ce22191adc01c2ab857ee6fd0
verify_index_sha256 acceptance/product/i01a_migration_compatibility.sh \
  3bc73a389c22bb6f441cf2c65c7249a325b98c09866f8fb7b7d47f02ab93d05a
verify_index_sha256 docs/execution/slices/P4-I01A.md \
  0f071da55a5c1cd4ee4cb867f0322f5545f567cba5214e299ed96c649f8a5878
verify_index_sha256 internal/product/app/catalog.go \
  693d25657ec325261094ff065d00175a3ff262dcec987dcd126eada897799d48
verify_index_sha256 internal/product/app/catalog_test.go \
  a8aaa8d7237914fa4d220ef5df5ef99f48349b6ed222aaf383954640b3a8657f
verify_index_sha256 internal/product/http/handler.go \
  d1f133d1a7fb090757b6e2b387d112b35cfb5ff04dec641136aa013d76e28787
verify_index_sha256 internal/product/port/port.go \
  76c5af0bbb2403c06673ee4927581929a0127dc26069edd14c20f41b18a65f30
verify_index_sha256 internal/product/store/catalog_repository.go \
  e72ecd1b51af8860727548b7b528a0e5ca27e1472ae066a32175974a51c28816
verify_index_sha256 internal/product/store/queries/catalog.sql \
  18f463a0a404ebe439a8ebd464b5ec9ee59d3b14d806e04d86207f1f1a13451a
verify_index_sha256 internal/product/store/generated/catalog.sql.go \
  d3043bb5b893a7cc7e8e90518781a7684e714052a33d704dca2a8517236fb58a
verify_index_sha256 internal/product/store/generated/db.go \
  81ace641f87433f29f031e3f08f195dcc6f558060e31c1ff73be57638a549ab0
verify_index_sha256 internal/product/store/generated/models.go \
  d589e8998b577c7de19a839751708bf13cd100a453938f09a596a80b675145de
verify_index_sha256 internal/product/store/generated/querier.go \
  ee2e794ee795c60ea4a0b50242ac21fd49e601207a7d6f166faa01c7e866c763
verify_index_sha256 cmd/aicrm/legacy_api.go \
  7b88fe8dbf8f0230bef039c17c717797e1084f6eb30d6bdb327393beebf8b2d5
verify_index_sha256 cmd/aicrm/legacy_product_api_test.go \
  7418ef9787c6753c5dba66a7f065a0a386dbb4eb5f977570fef1a3958a7a0c6e

verify_index_sha256 migrations/00031_survey_questionnaire_crud.sql \
  e6b5f511829fc5db130720d96f608eb92ace8103779c0591c89f2d9aa256bf44
verify_index_sha256 acceptance/survey/doc.go \
  c0957c68c070902912fb00f6b3cd9a2f4873480ec7ed72e1ea069033e9b1ae0e
verify_index_sha256 acceptance/survey/f01a_integration_test.go \
  6602f9a9ee98a35eea3e9bfcbe73fb3b195760c67e34919fecb8284366b0d8ba
verify_index_sha256 acceptance/survey/f01a_migration_compatibility.sh \
  7d25a283497cfca8786c0b5a1b358cb3bacccfd8c3dd53d845345e1fc89708b9
verify_index_sha256 docs/execution/slices/P4-F01A.md \
  b9c40b5a29f16c89821e3cabb14768b389f00e90ae1cb4b0e112ad683228a541
verify_index_sha256 docs/execution/slices/P4-B01.md \
  9f64d1f4818d2d38df7d01d34a3b230dc57af3bf4c49a93b670663657d7e850d
verify_index_sha256 internal/survey/app/service.go \
  9a90c6ffdd3bbf8c442f721c2bdfc4a90db610022349cd9f64113f902d02ece2
verify_index_sha256 internal/survey/app/service_test.go \
  33fc1b28e8c09d12183a7b63e7d2828dbfc298417e90439b2906255b381ce512
verify_index_sha256 internal/survey/port/port.go \
  be12e7ee0a82936d46a19a689063c609cdab3117eb1bd9bde5613620a6d50fbe
verify_index_sha256 internal/survey/store/questionnaire_repository.go \
  fe86a5677cd6c65d8c68758d3bdbedc3aa7ecde2da18e8c55fe399f614dbd3db
verify_index_sha256 internal/survey/store/queries/questionnaires.sql \
  4e88c60cfb1148db00e021d56ab1bdef9fa1a99560044e448fdaa3f7374bfa1d
verify_index_sha256 internal/survey/store/generated/questionnaires.sql.go \
  68161f498449053a74d1c85fe892f0eae794e2c4757cbab3afc0207db62c0476
verify_index_sha256 internal/survey/store/generated/db.go \
  bfb18ec7a7485d7069f130b24a7f17c7b5ec8de8dff19a988f655c77afdec14c
verify_index_sha256 internal/survey/store/generated/models.go \
  3b0bce22184630a4c4120b740d26c952dda40c0501f0ffa3a535ba4f13ef36c6
verify_index_sha256 internal/survey/store/generated/querier.go \
  8aef6700895f2e63c0eb591bfd4bf590408899cad6b650329a20adbad1228168
verify_index_sha256 cmd/aicrm/legacy_survey_api.go \
  9ae377d223d2439ac70765e31865648c2a0032e5c8d844085fbba77f3d7e9495
verify_index_sha256 cmd/aicrm/legacy_survey_api_test.go \
  00af21e4eeed26712cd75aa7119e4e778dfadd3739823bcbcf05204f7e5f93c3
verify_index_sha256 migrations/00032_contact_channel_catalog.sql \
  ce291b6d3d18a8636dba618605ef77bc27cf8c462595d1ede886486a50de847f
verify_index_sha256 acceptance/contact/c01_channel_integration_test.go \
  7eb022dace090b700b361e9da18d3d2520ec30949e47686b92b3ac16d1d2f9c1
verify_index_sha256 acceptance/contact/c01_migration_compatibility.sh \
  56120ec521dbd0c9de967d6dbc77dd8260a8c4a931caba39adc358571edf4276
verify_index_sha256 docs/execution/slices/P4-C01.md \
  d2aacc7a7ad8b4048cf4d06587b3d7f6c1f8acc5d135701c5d6796eade59ae04
verify_index_sha256 internal/contact/app/channel_catalog.go \
  c089bad30f9f129048ce059237913d15de175a70914d1452dae9129516092112
verify_index_sha256 internal/contact/app/channel_catalog_test.go \
  48388a263573d90d60068b853777ef2b83998387db3571c4d1608a03262743ab
verify_index_sha256 internal/contact/store/channel_repository.go \
  7e7a2633dd5ea86e4b3a2455029cf01c1672a1cd2be4a23415b791cd52980ee8
verify_index_sha256 internal/contact/store/queries/channels.sql \
  3597e1c64e9a28c38f1ae0bcc14eb8b425492ced94e24d77da803d316fac3011
verify_index_sha256 internal/contact/store/generated/channels.sql.go \
  4a620eadf335c7d3b2d64282522d69168594ac9a5c3572b0f20c84b41e1ead04
verify_index_sha256 cmd/aicrm/legacy_channel_api.go \
  a9f707de524506a3c019f2d46ae9b12fc443fde011b53f7d77adf029c7ce7f7d
verify_index_sha256 cmd/aicrm/legacy_channel_api_test.go \
  8d55ebabd6757fa76e0607314508e58fb7a7945b3c544be8a13e2b6e143cb5b0
verify_index_sha256 migrations/00033_coupon_rules.sql \
  d61a184f8f374347825da595550fd7f4dfeab6c349a6bc051ae4a2c875cddb42
verify_index_sha256 acceptance/coupon/j01_integration_test.go \
  f6f5ce30ce0dc18b0e054b02f2c9dddf670602f07436b9bbd078e1666dd6c2d9
verify_index_sha256 acceptance/coupon/j01_migration_compatibility.sh \
  bf1db7363039c1d28c8ab0ba6ef740a8905dbd3e455abd033267893b9f3976aa
verify_index_sha256 docs/execution/slices/P4-J01.md \
  e462aec2fe5ac93864381eaedc01b347e450abcbbd801777c718a4c9cac48cdd
verify_index_sha256 internal/coupon/app/service.go \
  b613f00d4dbfb255f518fc5b36605ec7b4193dba042931fcf19e80bd99ab7061
verify_index_sha256 internal/coupon/port/port.go \
  7fee782548717ff49bc1abeef0f58283e0604f801f0464c11ab78c37d5f86185
verify_index_sha256 internal/coupon/store/repository.go \
  ad51494fb0b838da0548e2fb775c36968331d59d968e092c9426ba2eec11a5ff
verify_index_sha256 internal/coupon/store/queries/coupons.sql \
  d4f9a4a95d8323a00c0eeecd06fbb932e8b44ca5324ba0189eb5d95e8ad2b813
verify_index_sha256 internal/coupon/store/generated/coupons.sql.go \
  501b436d5bb767e6505fabc4891bb251df635ed3f5cff8c791ce4be251a235fd
verify_index_sha256 internal/coupon/store/generated/db.go \
  67b000d31eab5169e6d648939508855cf3f47ebb7ab02575f018a8a4ec4b5b7e
verify_index_sha256 internal/coupon/store/generated/models.go \
  db9ab4ea432fe259b84812b99cf38ac84b1b466669aec645f341b42d1a7aa8ef
verify_index_sha256 internal/coupon/store/generated/querier.go \
  3fb0992de1da361d99df300776911b409bf177b3edf8fe6f964533d232839440
verify_index_sha256 cmd/aicrm/legacy_coupon_api.go \
  d147073bfb81bb43f60b63045d542900123870c2dfbe73af155d90163263b2da
verify_index_sha256 cmd/aicrm/legacy_coupon_api_test.go \
  cdf75eb6a244355581e8c0102dad20881e0564ddea03f37df128c46090b9e773
verify_index_sha256 migrations/00034_media_group_invite_library.sql \
  7f72f9bce21c043ad7145ce11221aa8be831f85458afbd48bcaa516b70e9e8fa
verify_index_sha256 acceptance/media/h03_integration_test.go \
  5c09bb3ac834fa3b5b8a35fa473d4551d63ecc46c47b4f6e76a1b89d1af73dba
verify_index_sha256 acceptance/media/h03_migration_compatibility.sh \
  61c90bb6fd9129752a125d1b3f7c78cbeaa14f55fe8482d39cc0b1fc514f3f48
verify_index_sha256 docs/execution/slices/P4-H03.md \
  2a266ce94bb91b24f556bbe4458d21bfb1eff41552b0b10fbcea798841a84321
verify_index_sha256 internal/media/app/group_invite.go \
  7f8182526291c9836e08c8a1a8f9368f5ceb9f1e9c23d4bd52c4ad4363419d26
verify_index_sha256 internal/media/app/group_invite_test.go \
  3bfaacb8fe3366f5957ffde0def5042854dd2890a6db1acd3b29ed2a307818b1
verify_index_sha256 internal/media/domain/group_invite.go \
  76944f6cf21b24ca207b34a9f63b1b2fa489d4dc5dedd1c845eda5788be4b992
verify_index_sha256 internal/media/domain/group_invite_test.go \
  eff8438b7510e4dbaa3b3159a315978fd46cbd9eea371915f60bc1e1457788ed
verify_index_sha256 internal/media/store/group_invite_repository.go \
  47a39f765b1306e6f92b0737be6e90fe2e8f17ff190e319f07d74d01245d9d85
verify_index_sha256 internal/media/store/queries/group_invites.sql \
  bb6de34c3d38f5bbd821f02027a9d589209265814add21442ebe142321d39495
verify_index_sha256 internal/media/store/generated/group_invites.sql.go \
  f07b18059b5797744600d8aea9adef0e95123dd5cf679b271b87724a3e6a0e8e
verify_index_sha256 cmd/aicrm/legacy_group_invite_api.go \
  229711d408462a92599be1be67cb0e5a3371bd1da7aeadbc911bb06110e83f7d
verify_index_sha256 cmd/aicrm/legacy_group_invite_api_test.go \
  676cc3e231157bcb700318a383d65ed1c37be499c5d91019d5edbc749dfa0ffb
verify_index_sha256 migrations/00035_order_list_projection.sql \
  164aaeebefd228f712f283b4f89567c94bd1d514131e1431bd47c3d0ee8685f1
verify_index_sha256 acceptance/order/doc.go \
  935d854b7b67a5592bf24eb04830823e4fd72fe2c89b953a9f9e75297e9fc2c1
verify_index_sha256 acceptance/order/i03_integration_test.go \
  99b181d37925bd8171032a51c87bc5b96245b493f43aff1ba0de32e55d7bbcd7
verify_index_sha256 acceptance/order/i03_migration_compatibility.sh \
  1786212bd9de4d6a331f69ccf7639d7f85ecc520da0dbe1050236941a3c4c926
verify_index_sha256 docs/execution/slices/P4-I03.md \
  98e8a988efdae0de6a0874e23c6cc5148f45268d1c55810f080a572c14822c1c
verify_index_sha256 internal/order/app/list.go \
  34bf09b270b94d36be3ac2372ef4213dd24c6d11d806a81cc314aa10a53be77b
verify_index_sha256 internal/order/app/list_test.go \
  907b058107478510e2a1955b69474a60437b850be0c67441cd1fb2f151790c5f
verify_index_sha256 internal/order/port/port.go \
  2a44c338acbc056871d885a24376df60bcf6cb49120c1ea088b060ea71ffab18
verify_index_sha256 internal/order/store/repository.go \
  d7222d602db964f59117bf92b635543d7531f50ec70316c48d73891d53703bdc
verify_index_sha256 internal/order/store/queries/orders.sql \
  0f672810fbf73e7d7a038d3955e526374a5e04ec7e8ddda2a0541d2c972d5921
verify_index_sha256 internal/order/store/generated/orders.sql.go \
  0e181b7c3868500c322dcc4c6939ff658c23c44c5494c17cc6d384ee2eae7943
verify_index_sha256 internal/order/store/generated/db.go \
  2b89131dd95b43cc31005812bf5a29741fc90a9bf30e1d14a134422f1a50a973
verify_index_sha256 internal/order/store/generated/models.go \
  57e80a5408c877054ba1604f17331a3b0b79853630e29eabc144fdeff376187e
verify_index_sha256 internal/order/store/generated/querier.go \
  566cd19848f62587182d39acb1bd7b7ae3f097b6275c94132f2131b18660b5c8
verify_index_sha256 internal/contact/store/queries/order_read_port.sql \
  eb66cb97b6e3b20c40a1e3e9fee66fba92e985c1ffe9f772eee54a4700b5f3cf
verify_index_sha256 internal/contact/store/generated/order_read_port.sql.go \
  e9abab52ef97b9e6bac2d54cbdb4acdafa4638416a6250c52e4a68c80a8aeb26
verify_index_sha256 cmd/aicrm/legacy_order_api.go \
  f8fedb60c6bc5cd9e47e6278b4e553f2d9af3c17d99ce916bdfeebe8b27149af
verify_index_sha256 cmd/aicrm/legacy_order_api_test.go \
  47e8de625374894ffca2b4253c9f0e04b9cc931722fa9c158d1d7c3139a86a0a

scripts/verify_repo_receipts.pl receipts "${receipt_arguments[@]}"

ledger_base_ref="HEAD"
if [[ -n "${GITHUB_BASE_REF:-}" ]] && git rev-parse --verify --quiet "origin/${GITHUB_BASE_REF}" >/dev/null; then
  ledger_base_ref="$(git merge-base HEAD "origin/${GITHUB_BASE_REF}")"
elif git diff --cached --quiet -- docs/execution/slice-ledger.yml && git rev-parse --verify --quiet HEAD^ >/dev/null; then
  ledger_base_ref="HEAD^"
fi
ruby scripts/check_slice_ledger_history.rb \
  --base "${ledger_base_ref}:docs/execution/slice-ledger.yml" \
  --candidate :docs/execution/slice-ledger.yml

makefile="$(git show ':Makefile')"; alternate="$(find . -maxdepth 1 \( -name GNUmakefile -o -name makefile \) -print -quit)"; [[ -z "$alternate" ]] || fail "alternate Make entrypoint is forbidden: ${alternate#./}"
! grep -Eq '^[[:space:]]*(-?include|sinclude)([[:space:]]|$)' <<<"$makefile" && ! awk 'index($0,"$") && substr($0,1,1) != "\t" && $0 !~ /^[[:space:]]*#/ { bad=1 } END { exit bad ? 0 : 1 }' <<<"$makefile" ||
  fail "Makefile must not construct or import rules dynamically"
grep -Eq '^p0-s03-contract:[[:space:]]*$' <<<"$makefile" ||
  fail "Makefile is missing the P0-S03 contract target"
p0_s03_contract_recipe="$(
  awk '
    /^p0-s03-contract:[[:space:]]*$/ { capture = 1; next }
    capture && /^[^[:space:]]/ { exit }
    capture { print }
  ' <<<"$makefile"
)"
grep -Fqx $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s03/test_contract.sh' <<<"$p0_s03_contract_recipe" ||
  fail "P0-S03 contract target must run the gitless contract tests"
grep -Eq '^p0-s03-acceptance:[[:space:]]+p0-s03-contract([[:space:]]|$)' <<<"$makefile" ||
  fail "P0-S03 acceptance target must depend on the contract target"
ci_go_target="$(grep -E '^ci-go:[[:space:]]' <<<"$makefile" || true)"
[[ "$ci_go_target" =~ (^|[[:space:]])p0-s03-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S03 acceptance target"

require_make_line() {
  local line="$1"
  local label="$2"
  [[ "$(printf '%s\n' "$makefile" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "$label"
}

require_unique_make_target() {
  local target="$1"
  local count
  count="$(awk -v target="$target" '
    /^[^[:space:]#][^:]*:/ && $0 !~ /:[[:space:]]+override[[:space:]]+(SHELL|[.]SHELLFLAGS|GO)[[:space:]]*:=/ {
      header = $0
      sub(/:.*/, "", header)
      fields = split(header, names, /[[:space:]]+/)
      for (position = 1; position <= fields; position++) {
        if (names[position] == target) count++
      }
    }
    END { print count + 0 }
  ' <<<"$makefile")"
  [[ "$count" = "1" ]] || fail "Makefile target must be unique: $target"
}

make_target_recipe() {
  local header="$1"
  awk -v header="$header" '
    $0 == header { seen++; capture = 1; next }
    capture && /^[^[:space:]]/ { exit }
    capture { print }
    END { exit !(seen == 1) }
  ' <<<"$makefile"
}

for target in vet test build vuln; do
  recipe="$(make_target_recipe "$target:")" ||
    fail "Makefile target must be unique: $target"
  discovery='GOWORK=off $(GO) list ./...'
  if [[ "$target" = build ]]; then
    discovery="GOWORK=off \$(GO) list -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' ./..."
  fi
  [[ "$(grep -Fc "$discovery" <<<"$recipe" || true)" = "1" ]] ||
    fail "$target must use Go to discover buildable packages"
  [[ "$(grep -Fc "grep -Ev '(^|/)([.]git|node_modules|vendor)(/|\$\$)'" <<<"$recipe" || true)" = "1" ]] ||
    fail "$target must exclude .git, node_modules, and vendor packages"
  case "$target" in
    vet) expected='$(GO) vet $$packages' ;;
    test) expected='$(GO) test -race $$packages' ;;
    build) expected='$(GO) build $$packages' ;;
    vuln) expected='$(GO) tool -modfile=$(TOOLS_MOD) govulncheck $$packages' ;;
  esac
  grep -Fq "$expected" <<<"$recipe" ||
    fail "$target must consume the filtered Go package set"
done

require_make_line 'generate: generate-openapi generate-sqlc generate-orval' \
  "make generate must include the frozen Orval client"
require_unique_make_target generate-openapi
openapi_generate_recipe="$(make_target_recipe 'generate-openapi:')" ||
  fail "OpenAPI generate target must be unique"
expected_openapi_generate_recipe=$'\t@$(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen \\\n\t\t--config api/oapi-codegen.yaml api/openapi.yaml\n\t@$(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen \\\n\t\t--config api/oapi-codegen-p1-candidate.yaml api/openapi.yaml'
[[ "$openapi_generate_recipe" = "$expected_openapi_generate_recipe" ]] ||
  fail "OpenAPI generation lost the runtime or candidate boundary"
require_unique_make_target bootstrap-tools
require_unique_make_target orval-tool-check
require_unique_make_target generate-orval
require_unique_make_target orval-check
bootstrap_tools_recipe="$(make_target_recipe 'bootstrap-tools:')" ||
  fail "bootstrap tools target must be unique"
for fragment in \
  'bootstrap-tools: missing Go 1.26.6; install versions from .tool-versions' \
  'bootstrap-tools: expected Node.js 24.18.0 from .tool-versions' \
  'failed to install pinned oapi-codegen, sqlc, goose, and govulncheck tools' \
  'failed to install pinned Orval 7.21.0 and web tools' \
  '$(MAKE) --no-print-directory version-check orval-tool-check'; do
  grep -Fq -- "$fragment" <<<"$bootstrap_tools_recipe" ||
    fail "bootstrap tools lost a pinned installation or explicit failure boundary"
done
orval_tool_recipe="$(make_target_recipe 'orval-tool-check:')" ||
  fail "Orval tool target must be unique"
for fragment in \
  "missing pinned Orval 7.21.0" \
  "expected Orval 7.21.0" \
  "run 'make bootstrap-tools'"; do
  grep -Fq -- "$fragment" <<<"$orval_tool_recipe" ||
    fail "Orval tool check lost an explicit failure instruction"
done
orval_generate_recipe="$(make_target_recipe 'generate-orval: orval-tool-check')" ||
  fail "Orval generate target must be unique"
for fragment in \
  'PATH="$(dir $(abspath $(ORVAL))):$$PATH" $(ORVAL)' \
  '--input api/openapi.yaml --output web/src/api/generated/health.ts' \
  '--client fetch --mode single --clean web/src/api/generated --prettier'; do
  grep -Fq -- "$fragment" <<<"$orval_generate_recipe" ||
    fail "Orval generation lost a frozen input, output, client, or clean boundary"
done
grep -Fq '$$($(GO) list -m -f '\''{{.Version}}'\'' golang.org/x/text)" = "v0.39.0"' <<<"$makefile" ||
  fail "version-check must pin the GO-2026-5970 fixed x/text version"
orval_check_recipe="$(make_target_recipe 'orval-check:')" ||
  fail "Orval check target must be unique"
[[ "$(grep -Fc '$(MAKE) --no-print-directory generate-orval' <<<"$orval_check_recipe" || true)" = "2" ]] ||
  fail "Orval check must generate twice"
[[ "$(grep -Fc 'git diff --exit-code -- web/src/api/generated' <<<"$orval_check_recipe" || true)" = "2" ]] ||
  fail "Orval check must reject tracked generated drift after both passes"
[[ "$(grep -Fc 'git ls-files --others --exclude-standard -- web/src/api/generated' <<<"$orval_check_recipe" || true)" = "2" ]] ||
  fail "Orval check must reject untracked generated drift after both passes"

package_json="$(git show ':package.json')"
for fragment in \
  '"orval": "7.21.0"' \
  '"prettier": "3.9.6"' \
  '"js-yaml": "4.3.1"' \
  '"lodash": "4.18.1"' \
  '"orval:generate": "make generate-orval"' \
  '"orval:check": "make orval-check"' \
  '"orval:contract": "scripts/test_orval_generated_check.sh"' \
  'npm run orval:check && npm run orval:contract'; do
  [[ "$(grep -Fc "$fragment" <<<"$package_json" || true)" = "1" ]] ||
    fail "package.json lost a frozen P0-S06 Orval version, security override, or gate"
done

require_make_line 'unexport BASH_ENV ENV' \
  "Makefile must unexport BASH_ENV and ENV before starting recipes"
require_make_line '.PHONY: fmt-check vet test build vuln p0-s01-acceptance p0-s02-contract p0-s02-acceptance p0-s03-contract p0-s03-acceptance ci-go' \
  "Makefile must declare the P0-S02 contract and acceptance targets"
require_unique_make_target p0-s01-acceptance
p0_s01_acceptance_recipe="$(make_target_recipe 'p0-s01-acceptance:')" ||
  fail "P0-S01 acceptance target must be unique"
for line in \
  $'\t\tacceptance/p0s01/static_contract.sh && \\' \
  $'\t\t$(GO) test -race -timeout=15s -tags=p0s01_acceptance ./acceptance/p0s01 && \\' \
  $'\t\tacceptance/p0s01/process_blackbox.sh; \\'; do
  printf '%s\n' "$p0_s01_acceptance_recipe" | grep -Fqx "$line" ||
    fail "P0-S01 acceptance target must fail fast across static, tagged, and process checks"
done
require_unique_make_target p0-s02-contract
require_unique_make_target p0-s02-acceptance
require_make_line 'p0-s02-acceptance: p0-s02-contract' \
  "P0-S02 acceptance target must depend on the contract target"
p0_s02_contract_recipe="$(make_target_recipe 'p0-s02-contract:')" ||
  fail "P0-S02 contract target must be unique"
[[ "$p0_s02_contract_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s02/test_static_contract.sh' ]] ||
  fail "P0-S02 contract target must run only the frozen static-contract runner"
[[ "$ci_go_target" =~ (^|[[:space:]])p0-s02-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S02 acceptance target"
require_make_line '.PHONY: p0-s04-contract p0-s04-acceptance p0-s04-integration' \
  "Makefile must declare the P0-S04 targets exactly once"
require_unique_make_target p0-s04-contract
require_unique_make_target p0-s04-acceptance
require_unique_make_target p0-s04-integration
require_make_line 'p0-s04-contract:' \
  "Makefile is missing or overriding the P0-S04 contract target"
require_make_line 'p0-s04-acceptance: p0-s04-contract' \
  "P0-S04 acceptance target must depend only on the contract target"
require_make_line 'p0-s04-integration: p0-s04-contract' \
  "P0-S04 integration target must depend only on the contract target"

p0_s04_contract_recipe="$(make_target_recipe 'p0-s04-contract:')" ||
  fail "P0-S04 contract target must be unique"
[[ "$p0_s04_contract_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh\n\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_static_contract.sh' ]] ||
  fail "P0-S04 contract target must run only the frozen source and static runners"

p0_s04_empty_lines=(
  $'\t@shopt -s nullglob dotglob; \\'
  $'\t\triver_entries=(internal/platform/river/*); \\'
  $'\tif [[ -d internal && ! -L internal && \\'
  $'\t\t-d internal/platform && ! -L internal/platform && \\'
  $'\t\t-d internal/platform/river && ! -L internal/platform/river && \\'
  $'\t\t-f internal/platform/river/contract.go && ! -L internal/platform/river/contract.go && \\'
  $'\t\t"$${#river_entries[@]}" -eq 1 && "$${river_entries[0]}" = "internal/platform/river/contract.go" && \\'
  $'\t\t! -e internal/platform/river/runtime.go && ! -L internal/platform/river/runtime.go && \\'
  $'\t\t! -e internal/platform/river/migrate.go && ! -L internal/platform/river/migrate.go && \\'
  $'\t\t! -e internal/platform/river/runtime_test.go && ! -L internal/platform/river/runtime_test.go ]]; then \\'
)

p0_s04_acceptance_recipe="$(make_target_recipe 'p0-s04-acceptance: p0-s04-contract')" ||
  fail "P0-S04 acceptance target must be unique"
p0_s04_contract_test="$(git show ':acceptance/p0s04/contract_test.go')"
for line in \
  $'\tdatabaseURLEnv = "MIGRATION_TEST_DATABASE_URL"' \
  $'\tdatabaseURL := os.Getenv(databaseURLEnv)' \
  $'\tif err := fixtures.ValidateDatabaseURL(databaseURL); err != nil {' \
  $'\tpool, err := pgxpool.New(ctx, databaseURL)'; do
  [[ "$(printf '%s\n' "$p0_s04_contract_test" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "P0-S04 real PostgreSQL acceptance lost its validated dynamic database URL: $line"
done
! grep -Fq '127.0.0.1:5432' <<<"$p0_s04_contract_test" ||
  fail "P0-S04 real PostgreSQL acceptance must not hard-code the CI port"
for line in "${p0_s04_empty_lines[@]}" \
  $'\t\techo "P0-S04 acceptance gate: PENDING (implementation not present)"; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \\' \
  $'\t\tcoverage_output="$$(env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -cover -timeout=15s ./internal/platform/river 2>&1)" || { status=$$?; printf \'%s\\n\' "$$coverage_output"; exit "$$status"; }; \\' \
  $'\t\tif ! printf \'%s\\n\' "$$coverage_output" | awk \'$$1 == "ok" && $$2 == "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river" { matches++; value = $$0; if (value !~ /coverage: [0-9]+([.][0-9]+)?% of statements$$/) invalid = 1; else { sub(/^.*coverage: /, "", value); sub(/% of statements$$/, "", value); if (value + 0 <= 0) invalid = 1 } } END { exit !(matches == 1 && !invalid) }\'; then echo "P0-S04 acceptance gate: internal/platform/river must report positive numeric coverage" >&2; exit 1; fi; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=15s -tags=p0s04_acceptance -run \'^(TestPinnedRiverPublicAPISurface|TestRuntimeLifecycleContract|TestRuntimeStartContextIsolated|TestRuntimeCancellationWinsSimultaneousStopped|TestInvalidMigrationDirection)$$\' ./acceptance/p0s04; \\'; do
  printf '%s\n' "$p0_s04_acceptance_recipe" | grep -Fqx "$line" ||
    fail "P0-S04 acceptance target lost a frozen static, coverage, or non-PG call"
done

p0_s04_integration_recipe="$(make_target_recipe 'p0-s04-integration: p0-s04-contract')" ||
  fail "P0-S04 integration target must be unique"
for line in "${p0_s04_empty_lines[@]}" \
  $'\t\techo "P0-S04 integration gate: PENDING (implementation not present)"; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \\' \
  $'\t\ttest "$${ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST:-}" = "1" || { echo "ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 is required" >&2; exit 2; }; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 $(GO) test -race -timeout=45s -tags=p0s04_acceptance -run \'^TestOfficialMigrationUpDownUp$$\' ./acceptance/p0s04; \\'; do
  printf '%s\n' "$p0_s04_integration_recipe" | grep -Fqx "$line" ||
    fail "P0-S04 integration target lost a frozen static, guard, or PG call"
done

[[ "$(printf '%s\n' "$makefile" | grep -Ec '^ci-go:[[:space:]]' || true)" = "1" && "$ci_go_target" =~ (^|[[:space:]])p0-s04-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S04 acceptance target"

require_make_line '.PHONY: p2-s04-acceptance' \
  "Makefile must declare the P2-S04 acceptance target"
require_unique_make_target p2-s04-acceptance
p2_s04_acceptance_recipe="$(make_target_recipe 'p2-s04-acceptance:')" ||
  fail "P2-S04 acceptance target must be unique"
[[ "$p2_s04_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=45s ./acceptance/p2s04' ]] ||
  fail "P2-S04 acceptance target must run only the frozen queue-isolation acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s04-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S04 acceptance target"

require_make_line '.PHONY: p2-s05-acceptance' \
  "Makefile must declare the P2-S05 acceptance target"
require_unique_make_target p2-s05-acceptance
p2_s05_acceptance_recipe="$(make_target_recipe 'p2-s05-acceptance:')" ||
  fail "P2-S05 acceptance target must be unique"
[[ "$p2_s05_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=45s ./acceptance/p2s05' ]] ||
  fail "P2-S05 acceptance target must run only the frozen scheduler singleton acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s05-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S05 acceptance target"

require_make_line '.PHONY: p2-s07-acceptance' \
  "Makefile must declare the P2-S07 acceptance target"
require_unique_make_target p2-s07-acceptance
p2_s07_acceptance_recipe="$(make_target_recipe 'p2-s07-acceptance:')" ||
  fail "P2-S07 acceptance target must be unique"
[[ "$p2_s07_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s07' ]] ||
  fail "P2-S07 acceptance target must run only the frozen dispatcher crash-recovery acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s07-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S07 acceptance target"

require_make_line '.PHONY: p2-s08-acceptance' \
  "Makefile must declare the P2-S08 acceptance target"
require_unique_make_target p2-s08-acceptance
p2_s08_acceptance_recipe="$(make_target_recipe 'p2-s08-acceptance:')" ||
  fail "P2-S08 acceptance target must be unique"
[[ "$p2_s08_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=45s ./acceptance/p2s08' ]] ||
  fail "P2-S08 acceptance target must run only the frozen HTTP gateway acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s08-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S08 acceptance target"

require_make_line '.PHONY: p2-s09-acceptance' \
  "Makefile must declare the P2-S09 acceptance target"
require_unique_make_target p2-s09-acceptance
p2_s09_acceptance_recipe="$(make_target_recipe 'p2-s09-acceptance:')" ||
  fail "P2-S09 acceptance target must be unique"
[[ "$p2_s09_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s09' ]] ||
  fail "P2-S09 acceptance target must run only the frozen auth-session acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s09-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S09 acceptance target"

require_make_line '.PHONY: p2-s10-acceptance' \
  "Makefile must declare the P2-S10 acceptance target"
require_unique_make_target p2-s10-acceptance
p2_s10_acceptance_recipe="$(make_target_recipe 'p2-s10-acceptance:')" ||
  fail "P2-S10 acceptance target must be unique"
[[ "$p2_s10_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=30s ./acceptance/p2s10' ]] ||
  fail "P2-S10 acceptance target must run only the frozen RBAC acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s10-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S10 acceptance target"

require_make_line '.PHONY: p2-s11-acceptance' \
  "Makefile must declare the P2-S11 acceptance target"
require_unique_make_target p2-s11-acceptance
p2_s11_acceptance_recipe="$(make_target_recipe 'p2-s11-acceptance:')" ||
  fail "P2-S11 acceptance target must be unique"
[[ "$p2_s11_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s11' ]] ||
  fail "P2-S11 acceptance target must run only the frozen HTTP budget and router acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s11-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S11 acceptance target"

require_make_line '.PHONY: p2-s14-acceptance' \
  "Makefile must declare the P2-S14 acceptance target"
require_unique_make_target p2-s14-acceptance
p2_s14_acceptance_recipe="$(make_target_recipe 'p2-s14-acceptance:')" ||
  fail "P2-S14 acceptance target must be unique"
[[ "$p2_s14_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s14' ]] ||
  fail "P2-S14 acceptance target must run only the frozen stages store acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s14-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S14 acceptance target"

require_make_line '.PHONY: p2-s15-acceptance' \
  "Makefile must declare the P2-S15 acceptance target"
require_unique_make_target p2-s15-acceptance
p2_s15_acceptance_recipe="$(make_target_recipe 'p2-s15-acceptance:')" ||
  fail "P2-S15 acceptance target must be unique"
[[ "$p2_s15_acceptance_recipe" = $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s15' ]] ||
  fail "P2-S15 acceptance target must run only the frozen transactional stage-event acceptance package"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s15-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S15 acceptance target"

require_make_line '.PHONY: p2-s16-acceptance' \
  "Makefile must declare the P2-S16 acceptance target"
require_unique_make_target p2-s16-acceptance
p2_s16_acceptance_recipe="$(make_target_recipe 'p2-s16-acceptance:')" ||
  fail "P2-S16 acceptance target must be unique"
expected_p2_s16_recipe=$'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=60s ./acceptance/p2s16\n\t@/bin/bash -eu -o pipefail -c '\''env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run ./acceptance/p2s16/snapshotgen | env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./snapshot-gate compare ../acceptance/snapshots/catalog.v1.json'\'''
[[ "$p2_s16_acceptance_recipe" = "$expected_p2_s16_recipe" ]] ||
  fail "P2-S16 acceptance target must run the frozen CSRF and stdin-only snapshot checks"
[[ "$ci_go_target" =~ (^|[[:space:]])p2-s16-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P2-S16 acceptance target"

require_make_line '.PHONY: arch-import-lint arch-import-lint-test' \
  "Makefile must declare the architecture import lint targets"
require_unique_make_target arch-import-lint
require_unique_make_target arch-import-lint-test
arch_import_recipe="$(make_target_recipe 'arch-import-lint:')" ||
  fail "architecture import lint target must be unique"
[[ "$arch_import_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/check_arch_imports.go -root .' ]] ||
  fail "architecture import lint target lost the frozen checker call"
arch_import_test_recipe="$(make_target_recipe 'arch-import-lint-test:')" ||
  fail "architecture import lint test target must be unique"
[[ "$arch_import_test_recipe" = $'\t@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_arch_imports.sh' ]] ||
  fail "architecture import lint test target lost the frozen runner call"
for target in arch-import-lint arch-import-lint-test; do
  [[ "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] ||
    fail "ci-go must depend on $target"
done
require_make_line '.PHONY: ownership-lint ownership-lint-test' \
  "Makefile must declare the ownership lint targets"
for target in ownership-lint ownership-lint-test; do
  require_unique_make_target "$target"
  [[ "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] || fail "ci-go must depend on $target"
done
[[ "$(make_target_recipe 'ownership-lint:')" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/ownership/main.go -root .' ]] ||
  fail "ownership lint target lost the frozen checker call"
[[ "$(make_target_recipe 'ownership-lint-test:')" = $'\t@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_ownership.sh' ]] ||
  fail "ownership lint test target lost the frozen runner call"
require_make_line '.PHONY: acceptance-fixtures' \
  "Makefile must declare the acceptance fixture target"
require_make_line 'acceptance-fixtures: override SHELL := /bin/bash' \
  "acceptance fixture target must use absolute Bash"
require_make_line 'acceptance-fixtures: override .SHELLFLAGS := -eu -o pipefail -c' \
  "acceptance fixture target must pin fail-closed Bash flags"
require_make_line 'acceptance-fixtures: override GO := go' \
  "acceptance fixture target must reject hostile GO overrides"
require_unique_make_target acceptance-fixtures
acceptance_fixtures_recipe="$(make_target_recipe 'acceptance-fixtures:')" ||
  fail "acceptance fixture target must be unique"
for line in \
  $'\t@test -n "$${ACCEPTANCE_FIXTURES_TEST_DATABASE_URL:-}" || { echo "ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is required" >&2; exit 2; }' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$$ACCEPTANCE_FIXTURES_TEST_DATABASE_URL" $(GO) test -race -count=1 -timeout=30s ./acceptance/fixtures'; do
  printf '%s\n' "$acceptance_fixtures_recipe" | grep -Fqx "$line" ||
    fail "acceptance fixture target lost a frozen validation call"
done
[[ "$ci_go_target" =~ (^|[[:space:]])acceptance-fixtures($|[[:space:]]) ]] ||
  fail "ci-go must depend on acceptance-fixtures"
require_make_line '.PHONY: source-policy-lint source-policy-lint-test' \
  "Makefile must declare the source policy lint targets"
for target in source-policy-lint source-policy-lint-test; do
  require_unique_make_target "$target"
  [[ "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] || fail "ci-go must depend on $target"
done
[[ "$(make_target_recipe 'source-policy-lint:')" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/sourcepolicy/main.go -root .' ]] ||
  fail "source policy lint target lost the frozen checker call"
[[ "$(make_target_recipe 'source-policy-lint-test:')" = $'\t@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_source_policy.sh' ]] ||
  fail "source policy lint test target lost the frozen runner call"
require_make_line '.PHONY: slice-input-contract slice-input-contract-test' \
  "Makefile must declare the slice input contract targets"
require_make_line 'slice-input-contract slice-input-contract-test: override SHELL := /bin/bash' \
  "slice input contract targets must use absolute Bash"
require_make_line 'slice-input-contract slice-input-contract-test: override .SHELLFLAGS := -eu -o pipefail -c' \
  "slice input contract targets must pin fail-closed Bash flags"
for target in slice-input-contract slice-input-contract-test; do
  require_unique_make_target "$target"
done
[[ "$(make_target_recipe 'slice-input-contract:')" = $'\t@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_slice_inputs.sh' ]] ||
  fail "slice input contract target lost the frozen checker call"
require_make_line 'slice-input-contract-test: slice-input-contract' \
  "slice input tests must depend on the canonical checker"
[[ "$(make_target_recipe 'slice-input-contract-test: slice-input-contract')" = $'\t@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/test_slice_inputs.sh' ]] ||
  fail "slice input contract tests lost the frozen runner call"
[[ "$ci_go_target" =~ (^|[[:space:]])slice-input-contract-test($|[[:space:]]) ]] ||
  fail "ci-go must depend on slice-input-contract-test"
require_make_line '.PHONY: snapshot-gate snapshot-gate-test' \
  "Makefile must declare the snapshot gate targets"
require_unique_make_target snapshot-gate
require_unique_make_target snapshot-gate-test
require_make_line 'snapshot-gate-test: snapshot-gate' \
  "snapshot gate tests must depend on canonical catalog validation"
[[ "$ci_go_target" =~ (^|[[:space:]])snapshot-gate-test($|[[:space:]]) ]] ||
  fail "ci-go must depend on snapshot gate tests"
snapshot_gate_recipe="$(make_target_recipe 'snapshot-gate:')" ||
  fail "snapshot gate target must be unique"
[[ "$snapshot_gate_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./snapshot-gate validate ../acceptance/snapshots/catalog.v1.json' ]] ||
  fail "snapshot gate target lost canonical catalog validation"
! grep -Fq 'contract-replay' <<<"$(git show ':Makefile')" ||
  fail "Makefile restored the retired contract replay target"
snapshot_gate_test_recipe="$(make_target_recipe 'snapshot-gate-test: snapshot-gate')" ||
  fail "snapshot gate test target must be unique"
for line in \
  $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./snapshot-gate' \
  $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=15s ./snapshot-gate' \
  $'\t@env -u BASH_ENV -u ENV GO="$(GO)" acceptance/p0s10/test_snapshot_gate.sh'; do
  printf '%s\n' "$snapshot_gate_test_recipe" | grep -Fqx "$line" ||
    fail "snapshot gate tests lost a frozen vet, race, or gitless call"
done

require_make_line '.PHONY: legacy-route-export-test' \
  "Makefile must declare the P1-S01 exporter test target"
require_unique_make_target legacy-route-export-test
[[ "$ci_go_target" =~ (^|[[:space:]])legacy-route-export-test($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P1-S01 exporter tests"
p1_s01_recipe="$(make_target_recipe 'legacy-route-export-test:')" ||
  fail "P1-S01 exporter test target must be unique"
for call in '$(GO) -C tools vet ./legacy-route-export' '$(GO) -C tools test -race -timeout=15s ./legacy-route-export'; do
  [[ "$(grep -Fc "$call" <<<"$p1_s01_recipe" || true)" = "1" ]] ||
    fail "P1-S01 exporter lost a frozen vet or race test call"
done

require_make_line '.PHONY: feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion' \
  "Makefile must declare the feature matrix contract and completion targets"
require_make_line 'feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: override SHELL := /bin/bash' \
  "feature matrix targets must use the absolute Bash interpreter"
require_make_line 'feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: override .SHELLFLAGS := -eu -o pipefail -c' \
  "feature matrix targets must pin fail-closed Bash flags"
for target in feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion; do
  require_unique_make_target "$target"
done
feature_matrix_recipe="$(make_target_recipe 'feature-matrix-contract:')" ||
  fail "feature matrix contract target must be unique"
[[ "$feature_matrix_recipe" = $'\t@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_feature_matrix_contract.sh' ]] ||
  fail "feature matrix contract target lost the frozen validator call"
completion_header='feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: feature-matrix-contract'
completion_recipe="$(make_target_recipe "$completion_header")" ||
  fail "feature matrix completion targets must share one frozen recipe"
[[ "$completion_recipe" = $'\t@phase="$@"; phase="$${phase#feature-matrix-}"; phase="$${phase%-completion}"; /usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_feature_matrix_contract.sh --completion "$$phase"' ]] ||
  fail "feature matrix completion targets lost phase isolation"
[[ "$ci_go_target" =~ (^|[[:space:]])feature-matrix-contract($|[[:space:]]) ]] ||
  fail "ci-go must depend on feature-matrix-contract"
for target in feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion; do
  [[ ! "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] || fail "completion targets must stay outside base ci-go: $target"
done

require_make_line '.PHONY: migration-mapping-contract migration-mapping-p1-completion' "Makefile must declare the migration mapping gates"
for target in migration-mapping-contract migration-mapping-p1-completion; do require_unique_make_target "$target"; done
mapping_recipe="$(make_target_recipe 'migration-mapping-contract:')" || fail "migration mapping contract target must be unique"
for call in '$(GO) -C tools vet ./migration-mapping' '$(GO) -C tools test -race -timeout=15s ./migration-mapping' '$(GO) -C tools run ./migration-mapping'; do [[ "$(grep -Fc "$call" <<<"$mapping_recipe" || true)" = "1" ]] || fail "migration mapping contract lost a frozen Go call"; done
require_make_line 'migration-mapping-p1-completion: migration-mapping-contract' "migration mapping completion must depend on its contract"
[[ "$(make_target_recipe 'migration-mapping-p1-completion: migration-mapping-contract')" = *'$(GO) -C tools run ./migration-mapping --completion'* ]] || fail "migration mapping completion lost its pending gate"
[[ "$ci_go_target" =~ (^|[[:space:]])migration-mapping-contract($|[[:space:]]) && ! "$ci_go_target" =~ (^|[[:space:]])migration-mapping-p1-completion($|[[:space:]]) ]] || fail "ci-go must run only the base migration mapping contract"

require_make_line '.PHONY: p1-reconciliation-contract' "Makefile must declare the P1 reconciliation gate"
require_make_line 'p1-reconciliation-contract: override SHELL := /bin/bash' "P1 reconciliation must use absolute Bash"
require_make_line 'p1-reconciliation-contract: override .SHELLFLAGS := -eu -o pipefail -c' "P1 reconciliation must pin fail-closed Bash flags"
require_make_line 'p1-reconciliation-contract: override GO := go' "P1 reconciliation must reject hostile GO overrides"
require_unique_make_target 'p1-reconciliation-contract'
reconciliation_recipe="$(make_target_recipe 'p1-reconciliation-contract:')" || fail "P1 reconciliation target must be unique"
for line in \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./p1-reconciliation' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./p1-reconciliation' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./p1-reconciliation'; do
  printf '%s\n' "$reconciliation_recipe" | grep -Fqx "$line" || fail "P1 reconciliation lost a frozen Go call"
done
[[ "$ci_go_target" =~ (^|[[:space:]])p1-reconciliation-contract($|[[:space:]]) ]] || fail "ci-go must depend on p1-reconciliation-contract"

require_make_line '.PHONY: openapi-p1-contract' "Makefile must declare the P1-S11 OpenAPI gate"
require_make_line 'openapi-p1-contract: override SHELL := /bin/bash' "P1-S11 OpenAPI gate must use absolute Bash"
require_make_line 'openapi-p1-contract: override .SHELLFLAGS := -eu -o pipefail -c' "P1-S11 OpenAPI gate must pin fail-closed Bash flags"
require_make_line 'openapi-p1-contract: override GO := go' "P1-S11 OpenAPI gate must reject hostile GO overrides"
require_unique_make_target 'openapi-p1-contract'
openapi_p1_recipe="$(make_target_recipe 'openapi-p1-contract:')" || fail "P1-S11 OpenAPI target must be unique"
for line in \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./openapi-contract' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=60s ./openapi-contract' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./openapi-contract' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=20s ./acceptance/p1s11'; do
  printf '%s\n' "$openapi_p1_recipe" | grep -Fqx "$line" || fail "P1-S11 OpenAPI gate lost a frozen validation call"
done
[[ "$ci_go_target" =~ (^|[[:space:]])openapi-p1-contract($|[[:space:]]) ]] || fail "ci-go must depend on openapi-p1-contract"

require_make_line '.PHONY: query-plan-gate query-plan-gate-test' "Makefile must declare the query plan gates"
require_make_line 'query-plan-gate query-plan-gate-test: override SHELL := /bin/bash' "query plan targets must use absolute Bash"
require_make_line 'query-plan-gate query-plan-gate-test: override .SHELLFLAGS := -eu -o pipefail -c' "query plan targets must pin fail-closed Bash flags"
require_make_line 'query-plan-gate query-plan-gate-test: override GO := go' "query plan targets must reject hostile GO overrides"
for target in query-plan-gate query-plan-gate-test; do require_unique_make_target "$target"; done
query_plan_recipe="$(make_target_recipe 'query-plan-gate:')" || fail "query plan gate target must be unique"
[[ "$query_plan_recipe" = $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./query-plan-gate -root .. -base "$$QUERY_PLAN_BASE_SHA" -head "$$QUERY_PLAN_HEAD_SHA" -database-url "$$QUERY_PLAN_TEST_DATABASE_URL"' ]] ||
  fail "query plan gate lost its frozen Git and database inputs"
require_make_line 'query-plan-gate-test: query-plan-gate' "query plan tests must depend on the real gate"
query_plan_test_recipe="$(make_target_recipe 'query-plan-gate-test: query-plan-gate')" || fail "query plan test target must be unique"
for line in \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./query-plan-gate' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./query-plan-gate' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GO="$(GO)" /bin/bash scripts/test_query_plan_gate.sh'; do
  printf '%s\n' "$query_plan_test_recipe" | grep -Fqx "$line" || fail "query plan tests lost a frozen static or PostgreSQL call"
done
[[ "$ci_go_target" =~ (^|[[:space:]])query-plan-gate-test($|[[:space:]]) ]] || fail "ci-go must depend on query-plan-gate-test"

application_go_workflow="$(git show ':.github/workflows/application-go.yml')"
verify_postgres_step="$(
  awk '
    $0 == "      - name: Verify PostgreSQL version and migrations" { seen++; capture = 1; next }
    capture && /^      - name:/ { exit }
    capture { print }
    END { exit !(seen == 1) }
  ' <<<"$application_go_workflow"
)" || fail "application workflow must contain one PostgreSQL verification step"
for line in \
  '          ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST: "1"' \
  '          make migration-integration' \
  '          make p0-s04-integration'; do
  [[ "$(printf '%s\n' "$verify_postgres_step" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow lost the fixed P0-S04 integration environment or call"
done
for line in '      BASH_ENV: ""' '      ENV: ""'; do
  [[ "$(printf '%s\n' "$application_go_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow must clear BASH_ENV and ENV"
done
for line in \
  '          ACCEPTANCE_FIXTURES_TEST_DATABASE_URL: postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable' \
  '          QUERY_PLAN_TEST_DATABASE_URL: postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable' \
  '          QUERY_PLAN_BASE_SHA: ${{ github.event.pull_request.base.sha || github.event.before || github.sha }}' \
  '          QUERY_PLAN_HEAD_SHA: ${{ github.sha }}'; do
  [[ "$(printf '%s\n' "$application_go_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow lost a frozen query plan gate input"
done
for line in \
  '  web:' \
  '    name: application / web' \
  '        uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38' \
  '          node-version: 24.18.0' \
  '        run: npm install --global npm@11.12.1 --ignore-scripts --no-audit --no-fund' \
  '          npm ci --ignore-scripts --no-audit --no-fund' \
  '          npm run ci'; do
  [[ "$(printf '%s\n' "$application_go_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow lost a frozen P0-S05 web gate: $line"
done

secret_scan_workflow="$(git show ':'.github/workflows/secret-scan.yml)"
for line in \
  '        run: gitleaks git . --config .gitleaks.toml --redact --no-banner --exit-code 1' \
  '        run: scripts/test_gitleaks_config.sh'; do
  [[ "$(printf '%s\n' "$secret_scan_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "secret scan workflow lost its pinned config or false-positive regression: $line"
done
repo_contract_workflow="$(git show ':'.github/workflows/repo-contract.yml)"
[[ "$(printf '%s\n' "$repo_contract_workflow" | grep -Fxc '    timeout-minutes: 30' || true)" = "1" ]] ||
  fail "repo-contract workflow must retain the 30-minute full negative-suite budget"
gitleaks_config="$(git show ':'.gitleaks.toml)"
for line in \
  'useDefault = true' \
  'targetRules = ["generic-api-key"]' \
  'condition = "AND"' \
  'regexTarget = "line"' \
  "paths = ['''(^|/)web/src/api/generated/[^/]+\.ts$''']" \
  "regexes = ['''^\s*\*\s+OpenAPI spec version:\s+[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?\s*$''']"; do
  [[ "$(printf '%s\n' "$gitleaks_config" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "gitleaks false-positive boundary drifted: $line"
done

expected_workflows="$({
  printf '%s\n' .github/workflows/application-go.yml
  printf '%s\n' .github/workflows/repo-contract.yml
  printf '%s\n' .github/workflows/secret-scan.yml
} | LC_ALL=C sort)"
actual_workflows="$(
  git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml' |
    LC_ALL=C sort
)"
[[ "$actual_workflows" = "$expected_workflows" ]] ||
  fail "workflow file set drifted; every workflow requires a Codex-owned hash update"

for number in $(seq -w 1 11); do
  [[ -f "docs/adr/ADR-0${number}.md" ]] || fail "missing ADR-0${number}"
done

if git ls-files tools/contract-replay acceptance/p0s10/test_contract_replay.sh | grep -q .; then
  fail "retired contract replay implementation is still tracked"
fi
[[ "$(git ls-files acceptance/snapshots)" = "acceptance/snapshots/catalog.v1.json" ]] ||
  fail "snapshot catalog inventory drifted; tracked actual responses are forbidden"
grep -Fq 'ADR-011' <<<"$(git show ':docs/adr/ADR-001.md')" || fail "ADR-001 lost its ADR-011 supersession marker"
grep -Fq 'ADR-011' <<<"$(git show ':docs/adr/ADR-010.md')" || fail "ADR-010 lost its ADR-011 supersession marker"
grep -Fq '不能证明新旧系统行为一致' <<<"$(git show ':docs/adr/ADR-011.md')" || fail "ADR-011 lost the snapshot capability boundary"
identity_adr="$(git show ':docs/adr/ADR-012.md')"
for required_identity_adr_text in \
  'identities`、`customer_merges`、`pending_events`' \
  'identity_operation_receipts' \
  '不得复用为 operation receipt' \
  'ON CONFLICT DO NOTHING RETURNING' \
  'raw identity 不进入持久化' \
  'normalized identity 只可存在于 identity-owned `identities` storage' \
  '既不是 Resolve 键' \
  '不设 TTL' \
  '不得被 River/periodic worker 扫描' \
  '不得建立 GIN 或仅按 state 的索引' \
  '闭集、脱敏的审计对象'; do
  grep -Fq "$required_identity_adr_text" <<<"$identity_adr" ||
    fail "ADR-012 lost required Identity storage decision: $required_identity_adr_text"
done

(cd docs/spec && sha256sum -c SHA256SUMS)

v1_plan="$(git show ':docs/spec/AI-CRM-v2-执行方案.md')"
v2_plan="$(git show ':docs/spec/AI-CRM-v2-执行方案-v2-至P3.md')"
design="$(git show ':docs/spec/AI-CRM-v2-重构详细设计.md')"
grep -Fq '> P0–P3 范围以《AI-CRM-v2-执行方案-v2-至P3.md》为准。' <<<"$v1_plan" ||
  fail "v1 plan must defer P0-P3 to the v2 plan"
grep -Fq 'b3613f635692c932021036f8f81babf24fca8222' <<<"$v2_plan" ||
  fail "v2 plan lost the frozen actual baseline footnote"
customers_ddl="$(awk '/^CREATE TABLE customers \(/ { capture=1 } capture { print } capture && /^\);/ { exit }' <<<"$design")"
[[ -n "$customers_ddl" ]] || fail "design lost the customers DDL"
! grep -Eq '^[[:space:]]+(external_userid|unionid|openid|phone)[[:space:]]' <<<"$customers_ddl" ||
  fail "customers DDL restored a forbidden external identity column"
! grep -Fq 'idx_customers_unionid' <<<"$design" || fail "design restored the forbidden customers unionid index"
grep -Eq '^[[:space:]]+scope[[:space:]]+TEXT[[:space:]]+NOT NULL' <<<"$design" ||
  fail "design identities scope must remain required"
! grep -Fq '/tools/contract-replay' <<<"$design" || fail "design restored the retired contract replay file_path"
grep -Fq '/acceptance/snapshots' <<<"$design" || fail "design lost the snapshot gate file_path"
grep -Fq '快照只防新系统自身回归，不能防新旧行为不一致' <<<"$design" ||
  fail "design lost the snapshot capability limitation"
grep -Fq 'River 队列固定为 `critical/event/outbound/sync/heavy/ai`' <<<"$design" ||
  fail "design lost the fixed six-queue topology"
grep -Fq 'CREATE TABLE inbox_events (' <<<"$design" ||
  fail "design lost the persistent inbox contract"
grep -Fq 'outcome_unknown' <<<"$design" ||
  fail "design lost the outbound unknown-outcome state"
grep -Fq 'event_deliveries 推迟到 P4' <<<"$design" ||
  fail "design lost the P4 per-consumer delivery deferral"
implementation_plan="$(git show ':docs/execution/implementation-plan.md')"
grep -Fq 'contact → (identity ∥ segment) → (wecom ∥ outbound)' <<<"$implementation_plan" ||
  fail "implementation plan lost the frozen P3 waves"

slice_policy_documents=(
  AGENTS.md
  docs/governance/agent-orchestration.md
  docs/execution/implementation-plan.md
  docs/plans/2026-08-13-p4-legacy-backend-parity.md
  docs/spec/AI-CRM-v2-执行方案-v2-至P3.md
  docs/spec/AI-CRM-v2-P2P3执行计划.md
  docs/execution/slice-card-template.md
)
for slice_policy_document in "${slice_policy_documents[@]}"; do
  slice_policy_source="$(git show ":$slice_policy_document")"
  grep -Fq 'SCOPE_FROZEN_REPAIR_ONLY' <<<"$slice_policy_source" ||
    fail "slice correction policy lost repair-only state: $slice_policy_document"
  grep -Fq 'HARD_STOP_REDLINE_READ_ONLY' <<<"$slice_policy_source" ||
    fail "slice correction policy lost redline hard-stop state: $slice_policy_document"
done

slice_policy_agents="$(git show ':AGENTS.md')"
grep -Fq '第 2 个起立即降档并进入 `SCOPE_FROZEN_REPAIR_ONLY`，冻结已批准能力范围' \
  <<<"$slice_policy_agents" ||
  fail "AGENTS lost the count-two scope freeze rule"
grep -Fq '第 3 个及以后保持该状态，不得仅因计数达到 3 丢弃候选' \
  <<<"$slice_policy_agents" ||
  fail "AGENTS restored count-three candidate discard"
grep -Fq '停止修复、重跑、' <<<"$slice_policy_agents" ||
  fail "AGENTS lost immediate redline stop actions"
grep -Fq '已按旧规则 HARD STOP 的 W0/A/H/I 及两次 W0 候选永久只读' \
  <<<"$slice_policy_agents" ||
  fail "AGENTS lost historical hard-stop non-revival"

single_instance_adr="$(git show ':docs/adr/ADR-013.md')"
single_instance_orchestration="$(git show ':docs/governance/agent-orchestration.md')"
single_instance_p4_plan="$(git show ':docs/plans/2026-08-13-p4-legacy-backend-parity.md')"
for required_single_instance_adr_text in \
  '单实例、单企业、单数据库' \
  'tenant model、tenant selector/switch、tenant RBAC、tenant 列、tenant 复合索引' \
  'actor 身份、RBAC/capability、session/CSRF/安全 redirect、数据归属' \
  'DROP_SOURCE_SCOPE' \
  '28/316' \
  '2/781' \
  '不得向 domain、store、SQL 或公共 port 传播' \
  'provider identity namespace' \
  '不得回改 `00004_auth.sql`' \
  '不计入 781/293/316 业务进度'; do
  grep -Fq "$required_single_instance_adr_text" <<<"$single_instance_adr" ||
    fail "ADR-013 lost single-instance decision: $required_single_instance_adr_text"
done
grep -Fq '部署模型固定为单实例、单企业、单数据库私有化' <<<"$slice_policy_agents" ||
  fail "AGENTS lost the single-instance deployment model"
grep -Fq '部署语义以 [ADR-013]' <<<"$single_instance_orchestration" ||
  fail "agent orchestration lost ADR-013 deployment semantics"
grep -Fq 'P4 以 [ADR-013]' <<<"$single_instance_p4_plan" ||
  fail "P4 plan lost ADR-013 deployment semantics"
for active_single_instance_document in \
  "$slice_policy_agents" \
  "$single_instance_orchestration" \
  "$single_instance_p4_plan"; do
  ! grep -Fq 'tenant/actor/授权/数据隔离破坏' <<<"$active_single_instance_document" ||
    fail "active policy restored tenant isolation as a product redline"
  ! grep -Fq '鉴权/tenant' <<<"$active_single_instance_document" ||
    fail "active policy restored tenant as a port requirement"
  ! grep -Fq 'auth/tenant' <<<"$active_single_instance_document" ||
    fail "active policy restored tenant as a review requirement"
done

[[ "$(git show ':docs/migration-mapping.jsonl' | grep -c 'tenant_id')" = "28" ]] ||
  fail "legacy migration mapping tenant_id source facts must remain exactly 28/316"
[[ "$(git show ':docs/api-mapping.jsonl' | grep -c '"name":"tenant_id"')" = "2" ]] ||
  fail "legacy API tenant_id compatibility parameters must remain exactly 2/781"

allowed_tenant_runtime_paths="$(printf '%s\n' \
  migrations/00004_auth.sql \
  migrations/00028_auth_wecom_corp_id.sql)"
actual_tenant_runtime_paths="$(git grep -Il -E 'tenant_id|TenantID|TenantId|tenant selector|tenant switch|cross.?tenant|multi.?tenant|跨租户' -- '*.go' '*.sql' | LC_ALL=C sort)"
[[ "$actual_tenant_runtime_paths" = "$allowed_tenant_runtime_paths" ]] ||
  fail "runtime tenant semantics escaped the temporary Auth compatibility bridge"

slice_policy_ledger="$(git show ':docs/execution/slice-ledger.yml')"
slice_policy_template="$(git show ':docs/execution/slice-card-template.md')"
for required_slice_policy_ledger_text in \
  '    non_redline_slice_induced_1: ACTIVE_REPAIR_ALLOWED' \
  '    non_redline_slice_induced_gte_2: SCOPE_FROZEN_REPAIR_ONLY' \
  '    non_redline_slice_induced_gte_3_discards_candidate: false' \
  '    redline_any_count: HARD_STOP_REDLINE_READ_ONLY' \
  '  policy_effective_scope: forward_only_after_policy_merge_for_fresh_or_no_WIP_candidates' \
  '  historical_hard_stops_retroactive_revival: false' \
  '    historical_W0_A_H_I_and_two_W0_candidates: permanent_read_only_no_revival_copy_or_cherry_pick'; do
  grep -Fxq "$required_slice_policy_ledger_text" <<<"$slice_policy_ledger" ||
    fail "slice ledger lost correction policy: $required_slice_policy_ledger_text"
done
for required_redline_category in \
  '    - security_boundary_auth_bypass_secret_leak_injection_or_open_redirect' \
  '    - cross_domain_ownership_or_required_transaction_atomicity_violation' \
  '    - duplicate_payment_refund_provider_real_external_effect_or_outcome_unknown_auto_retry' \
  '    - irreversible_data_damage_or_migration_loss' \
  '    - unauthorized_production_write_or_real_wecom_send_payment_refund_external_operation'; do
  grep -Fxq "$required_redline_category" <<<"$slice_policy_ledger" ||
    fail "slice ledger lost closed redline category: $required_redline_category"
  redline_category_name="${required_redline_category#    - }"
  grep -Fq "$redline_category_name" <<<"$slice_policy_template" ||
    fail "slice card template lost closed redline category: $redline_category_name"
done
grep -Fq '安全、数据损坏/不可逆风险、已决 ADR、全部架构铁律与 CI 门禁均不得进入' \
  <<<"$(git show ':docs/backlog/post-launch.md')" ||
  fail "post-launch backlog lost its non-deferrable boundaries"

forbidden_path_pattern='(^|/)(\.env[^/]*|node_modules|vendor|dist|build|coverage|\.cache|playwright-report|test-results|\.auth|\.browser)(/|$)|^(data|runtime|logs|uploads|tmp)(/|$)|(^|/)(id_rsa[^/]*|cookies[^/]*\.json|credentials[^/]*\.json)$|\.(pem|key|p12|pfx|db|sqlite|sqlite3|dump|zip)$'
if git ls-files | grep -E "$forbidden_path_pattern" >/dev/null; then
  git ls-files | grep -E "$forbidden_path_pattern" >&2
  fail "forbidden generated, credential, data, or binary file_path is tracked"
fi

if git ls-files | grep -E '(^|/)handoffs(/|$)|AI-CRM-v2-v3\.(patch|zip)$' >/dev/null; then
  fail "rejected Pro handoff artifacts must not enter the repository"
fi

if git grep --cached -n 'pull_request_target' -- .github/workflows; then
  fail "pull_request_target is forbidden"
fi

if git grep --cached -n -F '\' -- .github/workflows; then
  fail "workflow backslashes are forbidden because YAML escapes bypass text policy"
fi

if git grep --cached -n -E '[&*]|!!|<<[[:space:]]*:' -- .github/workflows; then
  fail "workflow YAML anchors, aliases, tags, and merge keys are forbidden"
fi

if git grep --cached -n -E \
  "[\"'][[:alnum:]_-]+[\"'][[:space:]]*:" \
  -- .github/workflows; then
  fail "quoted workflow keys are forbidden because they bypass policy scanners"
fi

if git grep --cached -n -i -E \
  '(^|[^[:alnum:]_])write(-all)?([^[:alnum:]_]|$)|(^|[^[:alnum:]_])environment([^[:alnum:]_]|$)|(^|[^[:alnum:]_])deploy(ment|ments|ing)?([^[:alnum:]_]|$)' \
  -- .github/workflows; then
  fail "workflow write permission, environment, or deployment is forbidden during bootstrap"
fi

if git grep --cached -n -i -E \
  '(^|[^[:alnum:]_])secrets([^[:alnum:]_]|$)' \
  -- .github/workflows; then
  fail "workflow secrets context is forbidden during bootstrap"
fi

while IFS= read -r workflow; do
  awk '
    /^permissions:[[:space:]]*$/ { in_top_permissions = 1; saw_permissions = 1; next }
    in_top_permissions && /^[^[:space:]]/ { in_top_permissions = 0 }
    in_top_permissions && /^[[:space:]]*($|#)/ { next }
    in_top_permissions {
      permission_entries++
      if ($0 == "  contents: read") saw_contents = 1
      else if ($0 == "  checks: read") saw_checks = 1
      else if ($0 == "  pull-requests: read") saw_pull_requests = 1
      else invalid_permission = 1
    }
    END {
      exit !(saw_permissions && permission_entries == 3 && saw_contents && saw_checks && saw_pull_requests && !invalid_permission)
    }
  ' "$workflow" ||
    fail "$workflow must declare only top-level contents/checks/pull-requests read permissions"

  [[ "$(grep -Ec '^[[:space:]]*permissions[[:space:]]*:' "$workflow")" -eq 1 ]] ||
    fail "$workflow must have exactly one canonical permissions key"

  canonical_uses_pattern='^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*[^[:space:]#]+([[:space:]]*#.*)?$'
  uses_key_pattern='(^|[^[:alnum:]_])uses[[:space:]]*:'
  while IFS= read -r workflow_line; do
    [[ ! "$workflow_line" =~ $uses_key_pattern ]] ||
      [[ "$workflow_line" =~ $canonical_uses_pattern ]] ||
      fail "$workflow contains a non-canonical uses mapping"
  done < "$workflow"
done < <(git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml')

while IFS= read -r action_ref; do
  [[ -z "$action_ref" ]] && continue
  [[ "$action_ref" != ./* ]] || continue
  action_name="${action_ref%@*}"
  action_sha="${action_ref##*@}"
  [[ -n "$action_name" && "$action_ref" == *@* &&
    "$action_sha" =~ ^[0-9a-f]{40}$ ]] ||
    fail "GitHub Action is not pinned to a full lowercase hex commit SHA: $action_ref"
done < <(
  git grep --cached -h -E '^[[:space:]]*(-[[:space:]]*)?uses:' \
    -- .github/workflows |
    sed -E 's/^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*([^[:space:]#]+).*/\2/'
)

shell_shadow_pattern='^[[:space:]]*for[[:space:]]+(path|PATH|IFS|HOME|PWD|SHELL|LANG|UID|STATUS)[[:space:]]+in([[:space:];]|$)|^[[:space:]]*(local|typeset|declare)[[:space:]]+([^#;]*[[:space:]])?(path|PATH|IFS|HOME|PWD|SHELL|LANG|UID|STATUS)(=|[[:space:];]|$)'
shell_shadow_matches="$(
  git grep --cached -n -E "$shell_shadow_pattern" -- \
    ':(glob)scripts/*.sh' ':(glob)scripts/**/*.sh' || true
)"
[[ -z "$shell_shadow_matches" ]] ||
  fail "shell loop/local variables must not shadow environment names: $shell_shadow_matches"

p2s18_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p2-s18-acceptance:$' <<<"$p2s18_make")" -eq 1 ]] ||
  fail "P2-S18 acceptance target must be declared exactly once"
grep -Eq '^ci-go:.*[[:space:]]p2-s18-acceptance([[:space:]]|$)' <<<"$p2s18_make" ||
  fail "P2-S18 acceptance target must remain connected to ci-go"
grep -Fq '$(GO) test -race -count=1 ./internal/platform/deployment ./cmd/aicrm-config' <<<"$p2s18_make" ||
  fail "P2-S18 generator race tests must remain in the acceptance target"
grep -Fq 'acceptance/p2s18/test_tier_config.sh' <<<"$p2s18_make" ||
  fail "P2-S18 black-box acceptance must remain in the acceptance target"

[[ "$(grep -Ec '^g2-release-archive-contract:$' <<<"$p2s18_make")" -eq 1 ]] ||
  fail "G2 release archive contract target must be declared exactly once"
grep -Eq '^ci-go:.*[[:space:]]g2-release-archive-contract([[:space:]]|$)' <<<"$p2s18_make" ||
  fail "G2 release archive contract must remain connected to ci-go"
grep -Fqx $'\t@env -u BASH_ENV -u ENV scripts/test_package_release_archive.sh' <<<"$p2s18_make" ||
  fail "G2 release archive contract must run the permanent archive tests"

[[ "$(grep -Ec '^g2-web-edge-contract:$' <<<"$p2s18_make")" -eq 1 ]] ||
  fail "G2 web edge contract target must be declared exactly once"
grep -Eq '^ci-go:.*[[:space:]]g2-web-edge-contract([[:space:]]|$)' <<<"$p2s18_make" ||
  fail "G2 web edge contract must remain connected to ci-go"
grep -Fqx $'\t@env -u BASH_ENV -u ENV scripts/test_g2_web_edge.sh' <<<"$p2s18_make" ||
  fail "G2 web edge contract must run the permanent edge tests"

p2s18_tier_source="$(git show :internal/platform/deployment/tier.go)"
for queue_name in CRITICAL EVENT OUTBOUND SYNC HEAVY AI; do
  [[ "$(grep -Fc "AICRM_RIVER_${queue_name}_MAX_WORKERS=" <<<"$p2s18_tier_source")" -eq 1 ]] ||
    fail "P2-S18 generated environment must contain the fixed queue exactly once: $queue_name"
done
! grep -Eiq 'AICRM_RIVER_DEFAULT|QueueDefault' <<<"$p2s18_tier_source" ||
  fail "P2-S18 must not generate or reference a default River queue"
for forbidden_key in DATABASE_URL PASSWORD TOKEN COOKIE SECRET WECOM; do
  ! grep -Fq "AICRM_${forbidden_key}=" <<<"$p2s18_tier_source" ||
    fail "P2-S18 generated environment must not contain credential key: $forbidden_key"
done

p2s18_compose="$(git show :deploy/compose.yml)"
grep -Fq 'image: postgres:16.14-bookworm' <<<"$p2s18_compose" ||
  fail "P2-S18 Compose PostgreSQL image drifted"
[[ "$(grep -Fxc '    profiles: [split]' <<<"$p2s18_compose")" -eq 2 ]] ||
  fail "P2-S18 Compose must keep exactly two split-role services"
! grep -Eiq '(redis|kafka|rabbitmq|nats|kubernetes)' <<<"$p2s18_compose" ||
  fail "P2-S18 Compose introduced forbidden infrastructure"

p2s18_staging="$(git show :scripts/staging_deploy.sh)"
authorization_line="$(grep -nF 'AICRM_ALLOW_STAGING_DEPLOY=1 is required for --apply' <<<"$p2s18_staging" | cut -d: -f1)"
first_docker_line="$(grep -nF 'docker compose' <<<"$p2s18_staging" | head -1 | cut -d: -f1)"
[[ "$authorization_line" =~ ^[0-9]+$ && "$first_docker_line" =~ ^[0-9]+$ && "$authorization_line" -lt "$first_docker_line" ]] ||
  fail "P2-S18 staging authorization must fail before the first Docker call"
! grep -Eq '(^|[;&|[:space:]])(goose|migrate)([;&|[:space:]]|$)' <<<"$p2s18_staging" ||
  fail "P2-S18 staging script must not run migrations"

p3c02a_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p3-c02a-acceptance:$' <<<"$p3c02a_make")" -eq 1 ]] ||
  fail "P3-C02A acceptance target must be declared exactly once"
grep -Fq 'ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is required' <<<"$p3c02a_make" ||
  fail "P3-C02A acceptance must fail closed without the test database URL"
grep -Fq '$(GO) test -race -count=1 -timeout=45s ./acceptance/p3c02a' <<<"$p3c02a_make" ||
  fail "P3-C02A target must run the real PostgreSQL mutation acceptance"

p3c02a_workflow="$(git show :.github/workflows/application-go.yml)"
grep -Fqx '          ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c02a-acceptance' <<<"$p3c02a_workflow" ||
  fail "application workflow must run P3-C02A against the migration database"

p3c02a_api="$(git show :cmd/aicrm/api.go)"
grep -Fq '{http.MethodPatch, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateCustomer)}' <<<"$p3c02a_api" ||
  fail "updateCustomer must remain protected by the CSRF middleware"

p3c02a_events="$(git show :internal/events/port/port.go)"
grep -Eq 'EvCustomerUpdated[[:space:]]*=[[:space:]]*"customer\.updated"' <<<"$p3c02a_events" ||
  fail "P3-C02A customer.updated event contract drifted"

p3c02a_openapi="$(git show :api/openapi.yaml)"
[[ "$(grep -Fc 'gender: { type: integer, format: int32, minimum: -32768, maximum: 32767, nullable: true }' <<<"$p3c02a_openapi")" -eq 2 ]] ||
  fail "P3-C02A gender bounds must remain aligned with PostgreSQL SMALLINT"

p3c02c_api="$(git show :cmd/aicrm/api.go)"
for route in \
  '{http.MethodPatch, "/api/v1/customers/{customer_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.UpdateCustomer)}' \
  '{http.MethodPut, "/api/v1/customers/{customer_id}/stage", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.SetCustomerStage)}' \
  '{http.MethodPut, "/api/v1/customers/{customer_id}/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.AddCustomerTag)}' \
  '{http.MethodDelete, "/api/v1/customers/{customer_id}/tags/{tag_id}", authport.CapabilityCustomersWrite, true, http.HandlerFunc(wrapper.RemoveCustomerTag)}'; do
  grep -Fq "$route" <<<"$p3c02c_api" ||
    fail "P3-C02C mutation route lost CustomersWrite or CSRF protection: $route"
done

p3c02c_query="$(git show :internal/contact/store/queries/customer_mutations.sql)"
grep -Fq 'sqlc.narg(scope_owner_staff_id)::bigint IS NULL' <<<"$p3c02c_query" ||
  fail "P3-C02C global mutation scope predicate drifted"
grep -Fq 'c.owner_staff_id = sqlc.narg(scope_owner_staff_id)::bigint' <<<"$p3c02c_query" ||
  fail "P3-C02C owner mutation scope must remain inside the locking SQL predicate"

p3c02c_handler="$(git show :internal/contact/http/customer_mutation_handler.go)"
grep -Fq 'const maxCustomerMutationBodyBytes = 1 << 20' <<<"$p3c02c_handler" ||
  fail "P3-C02C mutation body limit drifted"
grep -Fq 'if _, duplicate := object[key]; duplicate {' <<<"$p3c02c_handler" ||
  fail "P3-C02C top-level duplicate JSON key rejection drifted"
grep -Fq 'ScopeOwnerStaffID: scopeOwnerStaffID' <<<"$p3c02c_handler" ||
  fail "P3-C02C handler stopped passing owner scope into the transaction-bound command"

p3c02b_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p3-c02b-acceptance:$' <<<"$p3c02b_make")" -eq 1 ]] ||
  fail "P3-C02B acceptance target must be declared exactly once"
grep -Fq '$(GO) test -race -count=1 -timeout=45s ./acceptance/p3c02b' <<<"$p3c02b_make" ||
  fail "P3-C02B target must run real PostgreSQL customer-detail acceptance"

p3c02b_workflow="$(git show :.github/workflows/application-go.yml)"
grep -Fqx '          ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c02b-acceptance' <<<"$p3c02b_workflow" ||
  fail "application workflow must run P3-C02B against the migration database"

p3c02b_query="$(git show :internal/contact/store/queries/customer_detail.sql)"
[[ "$(grep -Ec '^-- name:' <<<"$p3c02b_query")" -eq 1 ]] ||
  fail "P3-C02B customer and tags must use one SQL statement snapshot"
grep -Fq 'c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint' <<<"$p3c02b_query" ||
  fail "P3-C02B owner scope must remain a SQL predicate"
grep -Fq 'LEFT JOIN customer_tags AS ct ON ct.customer_id = c.id' <<<"$p3c02b_query" ||
  fail "P3-C02B snapshot query must include customer tags"
grep -Fq 'ORDER BY COALESCE(g.sort_order, 0), t.sort_order, t.id' <<<"$p3c02b_query" ||
  fail "P3-C02B tag ordering contract drifted"
! grep -Eiq '(wecom_tag_id|unionid|openid|external_userid)' <<<"$p3c02b_query" ||
  fail "P3-C02B detail query exposed an external identity"

p3c02b_extra="$(git show :internal/contact/app/customer_list_service.go)"
for forbidden_identity_key in unionid openid externaluserid phone mobile identity alipayuserid wecomtagid; do
  grep -Fq "\"$forbidden_identity_key\"" <<<"$p3c02b_extra" ||
    fail "P3-C02B channel-neutral extra guard lost key: $forbidden_identity_key"
done
[[ "$(grep -Fc '"ext:"' <<<"$p3c02b_extra")" -ge 2 ]] ||
  fail "P3-C02B channel-neutral extra guard lost ext namespace handling"
grep -Fq 'if canonicalCustomerExtraKey(key) == "kind" && isString && isExternalIdentityKind(kind) {' <<<"$p3c02b_extra" ||
  fail "P3-C02B identity kind guard must reject every canonical kind collision"

p3c02b_api="$(git show :cmd/aicrm/api.go)"
grep -Eq 'customerDetail[[:space:]]+\*contacthttp\.CustomerDetailHandler' <<<"$p3c02b_api" ||
  fail "P3-C02B candidate handler lost customer detail wiring"
grep -Fq 'contactstore.NewCustomerDetailRepository()' <<<"$p3c02b_api" ||
  fail "P3-C02B runtime lost customer detail repository wiring"
grep -Fq 'handler.customerDetail.GetCustomer(writer, request, customerID)' <<<"$p3c02b_api" ||
  fail "P3-C02B generated operation is not delegated to the detail handler"

p3c02b_handler="$(git show :internal/contact/http/customer_detail_handler.go)"
grep -Fq $'case errors.Is(err, contactapp.ErrInvalidCustomerDetailQuery):\n\t\tcode = platformhttp.CodeNotFound' <<<"$p3c02b_handler" ||
  fail "P3-C02B invalid path identifiers must remain hidden as 404"

p3c02d_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p3-c02d-acceptance:$' <<<"$p3c02d_make")" -eq 1 ]] ||
  fail "P3-C02D acceptance target must be declared exactly once"
grep -Fq '$(GO) test -race -count=1 -timeout=90s ./acceptance/p3c02d' <<<"$p3c02d_make" ||
  fail "P3-C02D target must run real PostgreSQL customer-event acceptance"

p3c02d_workflow="$(git show :.github/workflows/application-go.yml)"
grep -Fqx '          ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c02d-acceptance' <<<"$p3c02d_workflow" ||
  fail "application workflow must run P3-C02D against the migration database"

p3c02d_query="$(git show :internal/contact/store/queries/customer_events.sql)"
grep -Fq 'ce.customer_id = event_customer.customer_id' <<<"$p3c02d_query" ||
  fail "P3-C02D timeline query lost customer scope"
grep -Fq 'c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint' <<<"$p3c02d_query" ||
  fail "P3-C02D owner scope must remain a SQL predicate"
grep -Fq 'AND (ce.occurred_at, ce.id) < (' <<<"$p3c02d_query" ||
  fail "P3-C02D timeline keyset tuple drifted"
grep -Fq 'ORDER BY ce.occurred_at DESC, ce.id DESC' <<<"$p3c02d_query" ||
  fail "P3-C02D timeline ordering drifted"
! grep -Eiq '(^|[^[:alpha:]])(offset|count[[:space:]]*[(])' <<<"$p3c02d_query" ||
  fail "P3-C02D timeline query must not use OFFSET or COUNT"

p3c02d_service="$(git show :internal/contact/app/customer_event_service.go)"
grep -Fq 'if _, duplicate := fields[key]; duplicate {' <<<"$p3c02d_service" ||
  fail "P3-C02D cursor must reject duplicate JSON fields"
grep -Fq 'cursor.Operation != customerEventCursorOperation' <<<"$p3c02d_service" ||
  fail "P3-C02D cursor lost operation binding"
grep -Fq 'cursor.FilterHash != expectedFilterHash' <<<"$p3c02d_service" ||
  fail "P3-C02D cursor lost customer and owner scope binding"

p3c02d_handler="$(git show :internal/contact/http/customer_event_handler.go)"
grep -Fq 'if *params.Cursor == "" {' <<<"$p3c02d_handler" ||
  fail "P3-C02D handler must reject an explicitly empty cursor"
grep -Fq 'item.CustomerID <= 0' <<<"$p3c02d_handler" ||
  fail "P3-C02D handler must reject invalid event customer IDs"
grep -Fq 'authorization.Capability != authport.CapabilityCustomerEventsRead' <<<"$p3c02d_handler" ||
  fail "P3-C02D handler lost customer.events.read authorization"

p3c02d_api="$(git show :cmd/aicrm/api.go)"
grep -Eq 'customerEvents[[:space:]]+\*contacthttp\.CustomerEventHandler' <<<"$p3c02d_api" ||
  fail "P3-C02D candidate handler lost timeline wiring"
grep -Fq 'contactstore.NewCustomerEventRepository()' <<<"$p3c02d_api" ||
  fail "P3-C02D runtime lost timeline repository wiring"
grep -Fq 'handler.customerEvents.ListCustomerEvents(writer, request, customerID, params)' <<<"$p3c02d_api" ||
  fail "P3-C02D generated operation is not delegated to the timeline handler"

p3c02d_acceptance="$(git show :acceptance/p3c02d/customer_event_integration_test.go)"
grep -Fq 'productionQuery := generatedCustomerEventQuery(t)' <<<"$p3c02d_acceptance" ||
  fail "P3-C02D EXPLAIN must execute the generated production query"
grep -Fq '"EXPLAIN (COSTS OFF)\n"+productionQuery' <<<"$p3c02d_acceptance" ||
  fail "P3-C02D generated production query is not connected to EXPLAIN"
for anchor in \
  'CREATE TABLE acceptance_fixtures.customer_merge_lineage (' \
  'merged_customer_id BIGINT PRIMARY KEY REFERENCES acceptance_fixtures.customers(id),' \
  'primary_customer_id BIGINT NOT NULL REFERENCES acceptance_fixtures.customers(id),' \
  'CONSTRAINT customer_merge_lineage_distinct CHECK (merged_customer_id <> primary_customer_id),' \
  'CONSTRAINT customer_merge_lineage_actor CHECK (' \
  'CONSTRAINT customer_merge_lineage_reason CHECK (' \
  'CREATE INDEX idx_customer_merge_lineage_primary' \
  'ON acceptance_fixtures.customer_merge_lineage (primary_customer_id, merged_customer_id);' \
  '"FROM customer_merge_lineage AS lineage", "FROM acceptance_fixtures.customer_merge_lineage AS lineage", 1'; do
  grep -Fq "$anchor" <<<"$p3c02d_acceptance" ||
    fail "P3-C02D lineage-aware fixture drifted: $anchor"
done
grep -Fq 'strings.Contains(plan.String(), "Seq Scan on customer_events")' <<<"$p3c02d_acceptance" ||
  fail "P3-C02D EXPLAIN must reject a customer_events sequential scan without rejecting lineage setup scans"
grep -Fq 'strings.Contains(plan.String(), "customer_events_timeline_idx")' <<<"$p3c02d_acceptance" ||
  fail "P3-C02D EXPLAIN must retain the timeline index assertion"

p3c02e_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p3-c02e-acceptance:$' <<<"$p3c02e_make")" -eq 1 ]] ||
  fail "P3-C02E acceptance target must be declared exactly once"
grep -Fq '$(GO) test -race -count=1 -timeout=90s ./acceptance/p3c02e' <<<"$p3c02e_make" ||
  fail "P3-C02E target must run real PostgreSQL tag-catalog acceptance"

p3c02e_workflow="$(git show :.github/workflows/application-go.yml)"
grep -Fqx '          ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c02e-acceptance' <<<"$p3c02e_workflow" ||
  fail "application workflow must run P3-C02E against the migration database"

p3c02e_query="$(git show :internal/contact/store/queries/tags.sql)"
grep -Fq 'LEFT JOIN tag_groups AS g ON g.id = t.group_id' <<<"$p3c02e_query" ||
  fail "P3-C02E tag catalog lost local group join"
grep -Fq '(t.group_id IS NULL)' <<<"$p3c02e_query" ||
  fail "P3-C02E ungrouped tags must remain last"
grep -Fq $'g.sort_order,\n  g.id,\n  t.sort_order,\n  t.id' <<<"$p3c02e_query" ||
  fail "P3-C02E deterministic tag catalog order drifted"
! grep -Fqi 'wecom_tag_id' <<<"$p3c02e_query" ||
  fail "P3-C02E tag catalog must not read wecom_tag_id"

p3c02e_service="$(git show :internal/contact/app/tag_catalog_service.go)"
grep -Fq 'return nil, errors.Join(ErrTagCatalogUnavailable, err)' <<<"$p3c02e_service" ||
  fail "P3-C02E tag catalog lost fail-closed error mapping"
grep -Fq 'return leftGrouped' <<<"$p3c02e_service" ||
  fail "P3-C02E grouped tags must remain before ungrouped tags"

p3c02e_handler="$(git show :internal/contact/http/tag_catalog_handler.go)"
grep -Fq 'record.GroupSortOrder == nil' <<<"$p3c02e_handler" ||
  fail "P3-C02E handler must reject an incomplete group identity and sort triple"
grep -Fq 'validTagCatalogResponseText(*record.GroupName)' <<<"$p3c02e_handler" ||
  fail "P3-C02E handler must validate provider-neutral group names before serialization"

p3c02e_api="$(git show :cmd/aicrm/api.go)"
grep -Eq 'tags[[:space:]]+\*contacthttp\.TagCatalogHandler' <<<"$p3c02e_api" ||
  fail "P3-C02E runtime lost tag catalog handler wiring"
grep -Fq 'contactstore.NewTagCatalogRepository()' <<<"$p3c02e_api" ||
  fail "P3-C02E runtime lost tag catalog repository wiring"
grep -Fq '{http.MethodGet, "/api/v1/tags", authport.CapabilityCustomersRead, false, http.HandlerFunc(wrapper.ListTags)}' <<<"$p3c02e_api" ||
  fail "P3-C02E runtime route lost customers.read authorization"

p3c02e_snapshot="$(git show :acceptance/snapshots/catalog.v1.json)"
[[ "$(grep -Fc '"operation_id": "listTags"' <<<"$p3c02e_snapshot")" -eq 3 ]] ||
  fail "P3-C02E listTags must have normal, boundary, and error handler snapshots"
grep -Fq '"path": "/api/v1/tags"' <<<"$p3c02e_snapshot" ||
  fail "P3-C02E listTags snapshot lost its generated route"
p3c02e_snapshot_generator="$(git show :acceptance/p2s16/snapshot.go)"
grep -Fq 'contacthttp.NewTagCatalogHandler(item.tags)' <<<"$p3c02e_snapshot_generator" ||
  fail "P3-C02E snapshot must execute the real tag catalog handler"
grep -Fq 'operationID: "listTags"' <<<"$p3c02e_snapshot_generator" ||
  fail "P3-C02E snapshot generator lost the listTags operation"

p3c02e_openapi="$(git show :api/openapi.yaml)"
grep -Fq 'x-aicrm-rbac-scopes: { admin: global, ops: global, sales: owner_staff }' <<<"$(sed -n '/operationId: listTags/,/\/api\/v1\/stages:/p' <<<"$p3c02e_openapi")" ||
  fail "P3-C02E listTags OpenAPI must preserve the existing sales owner_staff authorization model"
for response in '"403": { $ref: "#/components/responses/Forbidden" }' '"503": { $ref: "#/components/responses/ServiceUnavailable" }'; do
  grep -Fq "$response" <<<"$(sed -n '/operationId: listTags/,/\/api\/v1\/stages:/p' <<<"$p3c02e_openapi")" ||
    fail "P3-C02E listTags OpenAPI lost required response: $response"
done

p3c02e_acceptance="$(git show :acceptance/p3c02e/tag_catalog_integration_test.go)"
grep -Fq 'GRANT SELECT (id, group_id, name, sort_order) ON acceptance_fixtures.tags' <<<"$p3c02e_acceptance" ||
  fail "P3-C02E acceptance must deny the WeCom tag identifier column"
grep -Fq 'databaseError.Code != "42501"' <<<"$p3c02e_acceptance" ||
  fail "P3-C02E column privilege negative is missing"

p3c05_domain="$(git show :web/src/customer-detail.ts)"
for operation in getCustomer updateCustomer setCustomerStage addCustomerTag removeCustomerTag listCustomerEvents listTags; do
  grep -Eq "(^|[[:space:]])${operation},?$" <<<"$p3c05_domain" ||
    fail "P3-C05 must use generated client operation: $operation"
done
grep -Fq 'const SAME_ORIGIN: RequestInit = { credentials: "same-origin" };' <<<"$p3c05_domain" ||
  fail "P3-C05 reads lost same-origin credentials"
grep -Fq 'headers: { "X-CSRF-Token": csrfToken }' <<<"$p3c05_domain" ||
  fail "P3-C05 writes lost the server-bound CSRF header"
grep -Fq 'function exactObject(' <<<"$p3c05_domain" ||
  fail "P3-C05 response parsing must remain fail-closed"
! grep -Eqi 'external_?user|unionid|openid|wecom_tag_id' <<<"$p3c05_domain" ||
  fail "P3-C05 channel-neutral UI exposed an external identity"

p3c05_ui="$(git show :web/src/customer-detail-ui.tsx)"
grep -Fq 'if (lock.current) return undefined;' <<<"$p3c05_ui" ||
  fail "P3-C05 lost synchronous double-submit protection"
grep -Fq 'loadCustomerDetail(transport, customerID),' <<<"$p3c05_ui" ||
  fail "P3-C05 mutation success must refetch server facts"
grep -Fq '操作已提交，但未能重新读取服务端事实。请稍后刷新确认。' <<<"$p3c05_ui" ||
  fail "P3-C05 lost the successful-write/unconfirmed-read warning"

p3c05_main="$(git show :web/src/main.tsx)"
grep -Fq 'const match = /^\/customers\/([1-9]\d*)$/.exec(pathname);' <<<"$p3c05_main" ||
  fail "P3-C05 dynamic route must accept only an exact positive OneID"
grep -Fq 'Number.isSafeInteger(customerID)' <<<"$p3c05_main" ||
  fail "P3-C05 dynamic route lost the safe-integer boundary"
grep -Fq '<CustomerDetailPage' <<<"$p3c05_main" ||
  fail "P3-C05 customer detail page is not mounted"

p3c05_list="$(git show :web/src/customers-ui.tsx)"
grep -Fq 'href={`/customers/${customer.id}`}' <<<"$p3c05_list" ||
  fail "P3-C05 customer rows lost their native detail links"
grep -Fq 'onClick={onCustomerNavigate}' <<<"$p3c05_list" ||
  fail "P3-C05 customer links lost History API integration"

p3c06_data="$(git show :cmd/aicrm-contact-perf-data/main.go)"
for anchor in \
  'performanceDatabase       = "aicrm_perf"' \
  'resetToken                = "AICRM_PERF_RESET_V1"' \
  'customerCount     = 200000' \
  'tagsPerCustomer   = 3' \
  'config.MaxConns = 1' \
  'TRUNCATE TABLE public.customer_tags, public.customer_events' \
  'tx.CopyFrom(' \
  'ANALYZE public.stages' \
  'info.Mode().Perm()&0o077 != 0' \
  'host == "127.0.0.1" || host == "::1"'; do
  grep -Fq "$anchor" <<<"$p3c06_data" ||
    fail "P3-C06A1 data generator contract drifted: $anchor"
done
for forbidden_identity in external_userid unionid openid phone_number; do
  grep -Fq "\"$forbidden_identity\"" <<<"$p3c06_data" ||
    fail "P3-C06A1 customer identity-column guard drifted: $forbidden_identity"
done
! grep -Fq 'strings.HasPrefix(argument, "--database-url=")' <<<"$p3c06_data" ||
  fail "P3-C06A1 database credentials must not be accepted in process arguments"
grep -Fq 'strings.HasPrefix(argument, "--database-url-file=")' <<<"$p3c06_data" ||
  fail "P3-C06A1 private database URL file is required"
grep -Fq 'return rel == "cmd/aicrm-contact-perf-data/main.go" || rel == "cmd/aicrm-contact-perf/main.go"' <<<"$(git show :scripts/sourcepolicy/main.go)" ||
  fail "P3-C06 performance source-policy exceptions must remain two exact command paths"
grep -Fq 'performance-command-copy' <<<"$(git show :scripts/test_source_policy.sh)" ||
  fail "P3-C06A1 source-policy exception copy-path negative is missing"
grep -Fq 'performance-runner-copy' <<<"$(git show :scripts/test_source_policy.sh)" ||
  fail "P3-C06A2 source-policy exception copy-path negative is missing"

p3c06_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p3-c06a1-contract:$' <<<"$p3c06_make")" -eq 1 ]] ||
  fail "P3-C06A1 contract target must be declared exactly once"
grep -Eq '^ci-go:.* p3-c06a1-contract( |$)' <<<"$p3c06_make" ||
  fail "P3-C06A1 contract target is not connected to ci-go"

p3c06_runner="$(git show :cmd/aicrm-contact-perf/main.go)"
for anchor in \
  'requiredCustomers = 200_000' \
  'result := make([]scenario, 0, 4096)' \
  'for selectorMask := 0; selectorMask < 1<<selectorGroups; selectorMask++' \
  'EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)' \
  'contactstore.NewCustomerQueryRepository()' \
  'platformstore.NewUnitOfWork(pool)' \
  'validateReceipt(*result, opts.sourceSHA, opts.mainCIURL)' \
  'environment.SourceSHA != expectedSHA' \
  'environment.MainCIURL != expectedMainCIURL' \
  'set.StringVar(&result.mainCIURL, "main-ci-url"' \
  'Explain        json.RawMessage' \
  'decodePlanEvidence(plan.Query, plan.Explain)' \
  'result.GlobalP95MS < float64(latencyLimit)' \
  'host == "127.0.0.1" || host == "::1"' \
  'readBoundedRegularFile(path, 4096, true)'; do
  grep -Fq "$anchor" <<<"$p3c06_runner" ||
    fail "P3-C06A2 performance runner contract drifted: $anchor"
done
! grep -Fq '"database-url"' <<<"$p3c06_runner" ||
  fail "P3-C06A2 database credentials must not be accepted in process arguments"
grep -Fq 'set.StringVar(&databaseURLFile, "database-url-file"' <<<"$p3c06_runner" ||
  fail "P3-C06A2 private database URL file is required"
grep -Fq 'set.StringVar(&result.sourceSHA, "source-sha"' <<<"$p3c06_runner" ||
  fail "P3-C06A2 trusted source SHA input is required"
grep -Fq '"aicrm-contact-perf":' <<<"$(git show :scripts/check_arch_imports.go)" ||
  fail "P3-C06A2 exact architecture composition root is missing"
grep -Fq 'performance-composition-copy' <<<"$(git show :scripts/test_arch_imports.sh)" ||
  fail "P3-C06A2 architecture composition copy-path negative is missing"
[[ "$(grep -Ec '^p3-c06a2-contract:$' <<<"$p3c06_make")" -eq 1 ]] ||
  fail "P3-C06A2 contract target must be declared exactly once"
grep -Eq '^ci-go:.* p3-c06a2-contract( |$)' <<<"$p3c06_make" ||
  fail "P3-C06A2 contract target is not connected to ci-go"

p3c06c_query="$(git show :internal/contact/store/queries/customers.sql)"
p3c06c_bounded="${p3c06c_query#*-- name: CountCustomerIDsBounded :one}"
for anchor in \
  'SELECT count(*)::bigint' \
  'FROM (' \
  'SELECT c.id' \
  'UNION ALL' \
  'sqlc.narg(tag_id)::bigint IS NULL' \
  'sqlc.narg(tag_id)::bigint IS NOT NULL' \
  'FROM customer_tags AS ct' \
  'CROSS JOIN LATERAL (' \
  'c.id = ct.customer_id' \
  'ct.tag_id = sqlc.narg(tag_id)::bigint' \
  'ORDER BY c.updated_at DESC, c.id DESC' \
  'LIMIT sqlc.arg(total_limit)::integer' \
  ') AS bounded_customer_ids'; do
  grep -Fq "$anchor" <<<"$p3c06c_bounded" ||
    fail "P3-C06C capped bounded-total query drifted: $anchor"
done
for forbidden in 'after_updated_at' 'after_id'; do
  ! grep -Fq "$forbidden" <<<"$p3c06c_bounded" ||
    fail "P3-C06C bounded total must not apply cursor: $forbidden"
done

p3c06c_repository="$(git show :internal/contact/store/customer_query_repository.go)"
for anchor in \
  'queries.CountCustomerIDsBounded(ctx, countCustomerIDsBoundedParams(query))' \
  'BoundedTotal: boundedTotal' \
  'pgx.QueryExecModeCacheDescribe' \
  'type customerQueryDBTX struct{ pgx.Tx }'; do
  grep -Fq "$anchor" <<<"$p3c06c_repository" ||
    fail "P3-C06C contact query-plan boundary drifted: $anchor"
done
[[ "$(grep -Fc 'pgx.QueryExecModeCacheDescribe' <<<"$p3c06c_repository")" -eq 2 ]] ||
  fail "P3-C06C custom planning must remain scoped to Query and QueryRow"

p3c06c_config="$(git show :internal/config/schema.go)"
p3c06c_api="$(git show :cmd/aicrm/api.go)"
p3c06c_components="$(git show :cmd/aicrm/components.go)"
for anchor in \
  'url.ParseQuery(parsed.RawQuery)' \
  'capacity < 1' \
  'poolConfig.ConnConfig.DescriptionCacheCapacity < 1'; do
  grep -Fq "$anchor" <<<"$p3c06c_config
$p3c06c_api
$p3c06c_components" ||
    fail "P3-C06C description-cache fail-closed boundary drifted: $anchor"
done
[[ "$(grep -Fhc 'poolConfig.ConnConfig.DescriptionCacheCapacity < 1' <(printf '%s\n' "$p3c06c_api") <(printf '%s\n' "$p3c06c_components") | awk '{total += $1} END {print total+0}')" -eq 2 ]] ||
  fail "P3-C06C API and worker pools must both require description cache capacity"
for anchor in \
  'receiverType.Name != "customerQueryDBTX"' \
  'function.Name.Name != "Query" && function.Name.Name != "QueryRow"' \
  'identifier.Obj == receiver.Names[0].Obj' \
  'customer-plan-wrapper-wrong-receiver' \
  'customer-plan-wrapper-shadowed-receiver'; do
  grep -Fq "$anchor" <<<"$(git show :scripts/sourcepolicy/main.go; git show :scripts/test_source_policy.sh)" ||
    fail "P3-C06C source-policy receiver boundary drifted: $anchor"
done

for anchor in \
  'CountCustomerIDsBounded' \
  'queries[0].Name != "CountCustomerIDsBounded"' \
  'plan.Query != "CountCustomerIDsBounded"' \
  'explainArgs := append([]any{pgx.QueryExecModeCacheDescribe}, statement.Args...)'; do
  grep -Fq "$anchor" <<<"$p3c06_runner" ||
    fail "P3-C06C runner exact-query receipt drifted: $anchor"
done

p3c03_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p3-c03-migration-acceptance:$' <<<"$p3c03_make")" -eq 1 ]] ||
  fail "P3-C03 migration acceptance target must be declared exactly once"
grep -Fq '$(GO) test -race -count=1 -timeout=45s ./acceptance/contact -args -database-url "$$P3C03_TEST_DATABASE_URL"' <<<"$p3c03_make" ||
  fail "P3-C03 target must run the real PostgreSQL partition acceptance"

p3c03_workflow="$(git show :.github/workflows/application-go.yml)"
grep -Fqx '          P3C03_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c03-migration-acceptance' <<<"$p3c03_workflow" ||
  fail "application workflow must run P3-C03 after migration up/down/up"

p3c03_migration="$(git show :migrations/00006_customer_events.sql)"
grep -Fq 'CREATE FUNCTION public.aicrm_ensure_customer_event_partitions(' <<<"$p3c03_migration" ||
  fail "P3-C03 partition maintainer must be created in public explicitly"
grep -Fq 'BEFORE UPDATE OR DELETE ON public.customer_events' <<<"$p3c03_migration" ||
  fail "P3-C03 customer timeline must remain database-enforced append-only"
! grep -Eq 'PARTITION[[:space:]]+OF[[:space:]]+public[.]customer_events[[:space:]]+DEFAULT' <<<"$p3c03_migration" ||
  fail "P3-C03 must not add an unbounded default timeline partition"

p3c03_scheduler="$(git show :cmd/aicrm/scheduler.go)"
grep -Fq 'ID:         "contact.customer_events.partitions"' <<<"$p3c03_scheduler" ||
  fail "P3-C03 periodic partition job ID drifted"
grep -Fq 'Queue:      platformjobqueue.QueueHeavy' <<<"$p3c03_scheduler" ||
  fail "P3-C03 partition maintenance must remain isolated on heavy"
grep -Fq 'RunOnStart: true' <<<"$p3c03_scheduler" ||
  fail "P3-C03 partition maintenance must run when the worker starts"

p3c07a_migration="$(git show :migrations/00007_contact_merge_lineage.sql)"
for anchor in \
  'CREATE TABLE customer_merge_lineage (' \
  'merged_customer_id  BIGINT PRIMARY KEY REFERENCES customers(id)' \
  'primary_customer_id BIGINT NOT NULL REFERENCES customers(id)' \
  'CREATE INDEX idx_customer_merge_lineage_primary' \
  'DROP TABLE customer_merge_lineage;'; do
  grep -Fq "$anchor" <<<"$p3c07a_migration" ||
    fail "P3-C07A lineage-only migration drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE ' <<<"$p3c07a_migration")" -eq 1 ]] ||
  fail "P3-C07A migration must create exactly one lineage table"
! grep -Eq 'customer_event_idempotency|customer_events|external_event' <<<"$p3c07a_migration" ||
  fail "P3-C07A migration absorbed the P3-C07B/C event scope"

p3c07a_queries="$(git show :internal/contact/store/queries/customer_mutations.sql)"
p3c07a_merge_queries="${p3c07a_queries#*-- name: CreateCustomerForIdentity :one}"
for anchor in \
  'ORDER BY c.id' \
  'FOR UPDATE;' \
  '-- name: CopyCustomerTagsForMerge :execrows' \
  'ON CONFLICT (customer_id, tag_id) DO NOTHING;' \
  '-- name: MarkCustomerMerged :execrows' \
  '-- name: InsertCustomerMergeLineage :execrows' \
  '-- name: ResolveEffectiveCustomerRoot :one'; do
  grep -Fq -- "$anchor" <<<"$p3c07a_merge_queries" ||
    fail "P3-C07A merge SQL drifted: $anchor"
done
grep -Fqx 'ORDER BY c.id' <<<"$p3c07a_merge_queries" ||
  fail "P3-C07A customer locks must remain in ascending ID order"
! grep -Eq 'customer_event_idempotency|AppendExternalEvent' <<<"$p3c07a_merge_queries" ||
  fail "P3-C07A SQL absorbed the P3-C07C external-event registry"

p3c07a_repository="$(git show :internal/contact/store/merge_port_repository.go)"
for anchor in \
  'customerMutationQueriesFromContext(ctx)' \
  'queries.LockCustomersForMerge(ctx' \
  'return replaySameDirectionMerge(ctx, queries, command)' \
  'return []error{contactport.ErrMergeStoreFailed, err.cause}' \
  'case "40001", "40P01":'; do
  grep -Fq "$anchor" <<<"$p3c07a_repository" ||
    fail "P3-C07A transaction/retry boundary drifted: $anchor"
done
! grep -Fq 'NewUnitOfWork' <<<"$p3c07a_repository" ||
  fail "P3-C07A repository opened a nested UnitOfWork"

p3c07a_acceptance="$(git show :acceptance/contact/merge_lineage_integration_test.go)"
for anchor in \
  'TestContactMergeLineageCommitsTagUnionSoftDeleteAndReplay' \
  'TestContactMergeLineageRollsBackCreateTagsDeleteAndLineage' \
  'TestContactMergeLineageRetryableDatabaseErrorRerunsWholeUoW' \
  'TestSameDirectionMergeReplaySurvivesLaterLineageMerge' \
  'version != "160014"' \
  'attempts != 2'; do
  grep -Fq "$anchor" <<<"$p3c07a_acceptance" ||
    fail "P3-C07A real PostgreSQL acceptance drifted: $anchor"
done

p3c07a_make="$(git show :Makefile)"
[[ "$(grep -Ec '^p3-c07a-acceptance:$' <<<"$p3c07a_make")" -eq 1 ]] ||
  fail "P3-C07A acceptance target must be declared exactly once"
grep -Fq '$(GO) test -race -count=1 -timeout=90s ./acceptance/contact -args -database-url "$$P3C07A_TEST_DATABASE_URL"' <<<"$p3c07a_make" ||
  fail "P3-C07A target must run the real PostgreSQL lineage acceptance"
p3c07a_workflow="$(git show :.github/workflows/application-go.yml)"
grep -Fqx '          P3C07A_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c07a-acceptance' <<<"$p3c07a_workflow" ||
  fail "application workflow must run P3-C07A after migration up/down/up"

p3c07a_ownership="$(git show :docs/architecture/table-ownership.yml)"
grep -Fq '  customer_merge_lineage: contact' <<<"$p3c07a_ownership" ||
  fail "P3-C07A contact-owned lineage ownership drifted"
p3c07a_card="$(git show :docs/execution/slices/P3-C07A.md)"
for excluded_scope in customer_event_idempotency AppendExternalEvent EXPLAIN; do
  grep -Fq "$excluded_scope" <<<"$p3c07a_card" ||
    fail "P3-C07A card lost excluded-scope receipt: $excluded_scope"
done
p3c07_ledger="$(git show :docs/execution/slice-ledger.yml)"
for governance_anchor in \
  '    status: SUPERSEDED_BY_RESCOPE' \
  '    candidate_disposition: READ_ONLY_FOR_REFERENCE_NOT_FOR_PUSH_PR_OR_MERGE' \
  '    supersede_reason: slice_induced_correction_count_reached_4_requires_serial_rescope' \
  '    continuation_exception_status: INVALIDATED_BY_RESCOPE_HARD_STOP' \
  '    slice_induced_correction_count: 4' \
  '  - slice_id: P3-C07A' \
  '    supersedes: P3-C07'; do
  grep -Fq "$governance_anchor" <<<"$p3c07_ledger" ||
    fail "P3-C07 rescope ledger receipt drifted: $governance_anchor"
done

p3c07b1_query="$(git show :internal/contact/store/queries/customer_events.sql)"
for anchor in \
  'WITH RECURSIVE root_customer AS' \
  'AND NOT c.is_deleted' \
  'c.owner_staff_id = sqlc.narg(owner_staff_id)::bigint' \
  'FROM customer_merge_lineage AS lineage' \
  'JOIN lineage_ids AS parent' \
  'ON lineage.primary_customer_id = parent.customer_id' \
  'WHERE ce.customer_id = event_customer.customer_id' \
  'AND (ce.occurred_at, ce.id) < (' \
  'ORDER BY candidate.occurred_at DESC, candidate.id DESC'; do
  grep -Fq "$anchor" <<<"$p3c07b1_query" ||
    fail "P3-C07B1 lineage timeline query drifted: $anchor"
done
! grep -Eiq '(^|[^[:alpha:]])(offset|count[[:space:]]*[(])' <<<"$p3c07b1_query" ||
  fail "P3-C07B1 timeline query must not use OFFSET or COUNT"

p3c07b1_repository="$(git show :internal/contact/store/customer_event_repository.go)"
grep -Fq 'customerEventQueriesFromContext(ctx)' <<<"$p3c07b1_repository" ||
  fail "P3-C07B1 repository lost transaction-bound query access"
! grep -Fq 'NewUnitOfWork' <<<"$p3c07b1_repository" ||
  fail "P3-C07B1 repository opened a nested UnitOfWork"

p3c07b1_acceptance="$(git show :acceptance/contact/lineage_timeline_integration_test.go)"
for anchor in \
  'TestLineageTimelineUsesStableGlobalKeysetAndPreservesOrigin' \
  'OwnerStaffID: &ownerStaffID' \
  'pageCount < 3' \
  'repeated across keyset pages' \
  'want descending ids' \
  'direct merged customer=' \
  'wrong owner error=' \
  'openContactLineagePool(t)'; do
  grep -Fq "$anchor" <<<"$p3c07b1_acceptance" ||
    fail "P3-C07B1 real PostgreSQL behavior acceptance drifted: $anchor"
done
! grep -Fq 'EXPLAIN' <<<"$p3c07b1_acceptance" ||
  fail "P3-C07B1 absorbed the excluded C07B2 EXPLAIN scope"

p3c07b1_card="$(git show :docs/execution/slices/P3-C07B1.md)"
for anchor in \
  'Base SHA：`cefaf5fd35ff7b7154b9b769fa859a496f042713`' \
  'slice_induced_correction_count=3' \
  '本片无 DDL' \
  'P3-C07B2' \
  'P3-C07C' \
  'PRODUCTION_DATABASE_NOT_EXECUTED' \
  'P3_C07B0_PR_144'; do
  grep -Fq "$anchor" <<<"$p3c07b1_card" ||
    fail "P3-C07B1 card boundary drifted: $anchor"
done

for governance_anchor in \
  '  - slice_id: P3-C07B' \
  '    status: HARD_STOP_READ_ONLY_SUPERSEDED_BY_RESCOPE' \
  '    source_task_id: 019ff61a-73b3-7291-a4de-7d5f352580a8' \
  '    slice_induced_correction_count: 3' \
  '  - slice_id: P3-C07B1' \
  '    supersedes: P3-C07B' \
  '    scope_induced_correction_count: 1' \
  '    verification_induced_correction_count: 9' \
  '      - C07B2_EXPLAIN_AND_PERFORMANCE_NOT_EXECUTED'; do
  grep -Fq "$governance_anchor" <<<"$p3c07_ledger" ||
    fail "P3-C07B1 rescope ledger receipt drifted: $governance_anchor"
done

p3c07b2_acceptance="$(git show :acceptance/contact/lineage_timeline_plan_integration_test.go)"
for anchor in \
  'TestLineageTimelineGenericPlanUsesExistingIndexes' \
  'lineagePlanTargetCustomers     = 51' \
  'lineagePlanDistractorCustomers = 19_950' \
  'lineagePlanEventsPerTarget     = 100' \
  'lineagePlanEventsPerDistractor = 10' \
  'SET plan_cache_mode = force_generic_plan' \
  'PREPARE ` + statementName + `(timestamptz,bigint,integer,bigint,bigint) AS ` + query' \
  'EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE' \
  'pg_prepared_statements' \
  'genericPlans < 1 || customPlans != 0' \
  'generatedLineageTimelineQuery(t)' \
  'internal", "contact", "store", "generated", "customer_events.sql.go"' \
  'ANALYZE `+relation' \
  'idx_customer_merge_lineage_primary' \
  '_customer_id_occurred_at_id_idx' \
  'node.NodeType == "Seq Scan" && isLineageTimelinePlanTarget(node.RelationName)' \
  'strings.HasPrefix(relation, "customer_events_")'; do
  grep -Fq "$anchor" <<<"$p3c07b2_acceptance" ||
    fail "P3-C07B2 generic plan acceptance drifted: $anchor"
done
! grep -Fq 'enable_seqscan' <<<"$p3c07b2_acceptance" ||
  fail "P3-C07B2 must use the natural PostgreSQL planner"

require_unique_make_target 'p3-c07b2-acceptance'
p3c07b2_recipe="$(make_target_recipe 'p3-c07b2-acceptance:')" ||
  fail "P3-C07B2 acceptance target must be unique"
for line in \
  $'\t@test -n "$${P3C07B2_TEST_DATABASE_URL:-}" || { echo "P3C07B2_TEST_DATABASE_URL is required" >&2; exit 2; }' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s -run \'^TestLineageTimelineGenericPlanUsesExistingIndexes$$\' ./acceptance/contact -args -database-url "$$P3C07B2_TEST_DATABASE_URL"'; do
  printf '%s\n' "$p3c07b2_recipe" | grep -Fqx "$line" ||
    fail "P3-C07B2 acceptance target lost its exact real PostgreSQL plan invocation"
done
p3c07b2_workflow="$(git show :.github/workflows/application-go.yml)"
grep -Fq 'P3C07B2_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c07b2-acceptance' <<<"$p3c07b2_workflow" ||
  fail "P3-C07B2 acceptance is disconnected from application CI"

p3c07b2_card="$(git show :docs/execution/slices/P3-C07B2.md)"
for anchor in \
  'Base SHA：`5651878273299ccfdae91f84c5d61476c8ab04e1`' \
  '20,001 customers、10,025 lineage edges、204,600' \
  '`idx_customer_merge_lineage_primary`' \
  '`customer_events_*`' \
  '无 DDL、migration 或索引修改' \
  '禁止 `enable_seqscan=off`' \
  'PRODUCTION_DATABASE_NOT_EXECUTED'; do
  grep -Fq "$anchor" <<<"$p3c07b2_card" ||
    fail "P3-C07B2 card boundary drifted: $anchor"
done
for governance_anchor in \
  '  - slice_id: P3-C07B2' \
  '    dependency: P3_C07B1_MERGED_EXACT_MAIN_CI_SUCCESS' \
  '      - DDL_migration_or_index_changes' \
  '    slice_induced_correction_count: 0' \
  '    infra_induced_correction_count: 0' \
  '    scope_induced_correction_count: 0' \
  '    verification_induced_correction_count: 3' \
  '      - IDENTITY_PR_149_CLOSED_NOT_MERGED_SHARED_QUEUE_RELEASED_FIRST_READY_API_RECHECK_REQUIRED'; do
  grep -Fq "$governance_anchor" <<<"$p3c07_ledger" ||
    fail "P3-C07B2 ledger receipt drifted: $governance_anchor"
done

p3s00_migration="$(git show :migrations/00008_segment_contract.sql)"
for anchor in \
  '-- +goose Up' \
  'CREATE TABLE segments (' \
  'definition     JSONB NOT NULL,' \
  'created_by     BIGINT REFERENCES admin_users(id),' \
  "CONSTRAINT segments_definition_object CHECK (jsonb_typeof(definition) = 'object')" \
  "CONSTRAINT segments_refresh_mode CHECK (refresh_mode IN ('manual', 'scheduled'))" \
  "CONSTRAINT segments_refresh_status CHECK (refresh_status IN ('idle', 'running', 'failed'))" \
  'CREATE INDEX idx_segments_refresh_due' \
  'CREATE TABLE segment_members (' \
  'segment_id  BIGINT NOT NULL REFERENCES segments(id) ON DELETE CASCADE,' \
  'customer_id BIGINT NOT NULL REFERENCES customers(id),' \
  'CREATE INDEX idx_segment_members_customer ON segment_members (customer_id, segment_id);' \
  '-- +goose Down' \
  'DROP TABLE segment_members;' \
  'DROP TABLE segments;'; do
  grep -Fq -- "$anchor" <<<"$p3s00_migration" ||
    fail "P3-S00 segment migration receipt drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE ' <<<"$p3s00_migration")" -eq 2 ]] ||
  fail "P3-S00 migration must create exactly segments and segment_members"
! grep -Eq 'CREATE TABLE (customers|identities|customer_events|event_log|river_)' <<<"$p3s00_migration" ||
  fail "P3-S00 migration must not absorb Contact, Identity, event, or River ownership"

p3s00_ownership="$(git show :docs/architecture/table-ownership.yml)"
for anchor in \
  '  segment:' \
  '    package: internal/segment' \
  '    tables: [segments, segment_members, segment_refresh_receipts, segment_operation_receipts]' \
  '  segment:' \
  '    tables: [customers, customer_tags, tags]' \
  '    reason: indexed_audience_compilation'; do
  grep -Fq "$anchor" <<<"$p3s00_ownership" ||
    fail "P3-S00 Segment ownership/read boundary drifted: $anchor"
done

p3s00_port="$(git show :internal/segment/port/port.go)"
for anchor in \
  'type Definition json.RawMessage' \
  'type RefreshMode string' \
  'type RefreshStatus string' \
  'RefreshModeManual    RefreshMode = "manual"' \
  'RefreshModeScheduled RefreshMode = "scheduled"' \
  'RefreshStatusIdle    RefreshStatus = "idle"' \
  'RefreshStatusRunning RefreshStatus = "running"' \
  'RefreshStatusFailed  RefreshStatus = "failed"' \
  'IdempotencyKey string' \
  'List(context.Context, string, int32) (Page, error)' \
  'Get(context.Context, SegmentID) (Segment, error)' \
  'Create(context.Context, CreateCommand) (Segment, error)' \
  'Update(context.Context, UpdateCommand) (Segment, error)' \
  'ListMembers(context.Context, SegmentID, string, int32) (MemberPage, error)' \
  'RequestRefresh(context.Context, RefreshCommand) (Segment, error)'; do
  grep -Fq "$anchor" <<<"$p3s00_port" ||
    fail "P3-S00 Segment port receipt drifted: $anchor"
done
! grep -Eq 'pgx[.]|Query\(|Exec\(|Prepare\(' <<<"$p3s00_port" ||
  fail "P3-S00 port must not add direct SQL execution"

p3s00_port_contract="$(git show :docs/architecture/port-contracts.md)"
for anchor in \
  '## segment/port' \
  'type Definition json.RawMessage' \
  'RequestRefresh(context.Context, RefreshCommand) (Segment, error)' \
  'DSL 语法、错误码与 S02 QueryProgram / S03 固定 sqlc query-family 分层' \
  'Create、Update、RequestRefresh 的 IdempotencyKey 必填。' \
  '不能借 Segment definition 自行筛选。'; do
  grep -Fq "$anchor" <<<"$p3s00_port_contract" ||
    fail "P3-S00 Segment port-contract receipt drifted: $anchor"
done

p3s00_card="$(git show :docs/execution/slices/P3-S00.md)"
for anchor in \
  '不实现 parser/compiler/store/worker/handler/UI' \
  'Segment 独占写 `segments`、`segment_members`。它只读 contact 的' \
  'identity、event_log 或 River 表。' \
  'S01/S02 只能返回下列稳定 reason code' \
  'Decision B 将 compiler 分层' \
  'S02 只能把已验证 AST 映射为唯一、' \
  '闭合的 typed QueryProgram IR；S03 才将该 IR 映射为固定 sqlc 查询族并执行集合代数。' \
  'S02 的 QueryProgram 不含 SQL、identifier、查询模板或 escape hatch' \
  'S03 是唯一可执行 SQL 片' \
  '运行时不得发射、' \
  '拼接、format 或执行 SQL 文本' \
  '不能使用 ORM、动态 SQL builder' \
  '`Query/Exec/Prepare`。' \
  'PENDING_EXTERNAL_GATE'; do
  grep -Fq "$anchor" <<<"$p3s00_card" ||
    fail "P3-S00 frozen scope or SQL-safety receipt drifted: $anchor"
done

p3s00_openapi="$(git show :api/openapi.yaml)"
p3s00_openapi_paths="$(sed -n '/^  \/api\/v1\/segments:/,/^  \/api\/v1\/stages:/p' <<<"$p3s00_openapi")"
for anchor in \
  'operationId: createSegment' \
  'operationId: updateSegment' \
  'operationId: requestSegmentRefresh' \
  'x-aicrm-capability: segments.write' \
  'x-aicrm-capability: segments.read' \
  '$ref: "#/components/parameters/CSRFToken"' \
  '$ref: "#/components/parameters/IdempotencyKey"'; do
  grep -Fq "$anchor" <<<"$p3s00_openapi_paths" ||
    fail "P3-S00 OpenAPI security receipt drifted: $anchor"
done
[[ "$(grep -Fc '$ref: "#/components/parameters/CSRFToken"' <<<"$p3s00_openapi_paths")" -eq 3 ]] ||
  fail "P3-S00 write operations must retain CSRF protection"
[[ "$(grep -Fc '$ref: "#/components/parameters/IdempotencyKey"' <<<"$p3s00_openapi_paths")" -eq 3 ]] ||
  fail "P3-S00 write operations must retain idempotency protection"
for anchor in \
  '    SegmentDefinition:' \
  '        - $ref: "#/components/schemas/SegmentDefinitionAnd"' \
  '        - $ref: "#/components/schemas/SegmentDefinitionOr"' \
  '        - $ref: "#/components/schemas/SegmentDefinitionPredicate"' \
  '          enum: [stage_id, owner_staff_id, channel_id, tag_id, added_at, last_interact_at, is_deleted]'; do
  grep -Fq "$anchor" <<<"$p3s00_openapi" ||
    fail "P3-S00 closed DSL OpenAPI receipt drifted: $anchor"
done

p3s01_ast="$(git show :internal/segment/dsl/ast.go)"
for anchor in \
  'MaxDefinitionBytes = 64 << 10' \
  'MaxDepth           = 8' \
  'MaxNodes           = 128' \
  'MaxGroupChildren   = 64' \
  'MaxListValues      = 1000' \
  'ReasonDuplicateKey' \
  'ReasonUnsupportedOperator' \
  'type AST struct' \
  'type Node interface' \
  'type Value interface' \
  'func (ast AST) CanonicalJSON()'; do
  grep -Fq "$anchor" <<<"$p3s01_ast" ||
    fail "P3-S01 typed AST receipt drifted: $anchor"
done

p3s01_parser="$(git show :internal/segment/dsl/parser.go)"
for anchor in \
  'func Parse(input []byte) (AST, error)' \
  'decoder.UseNumber()' \
  'return nil, fieldError(keyPointer, ReasonDuplicateKey)' \
  'if depth > MaxDepth' \
  'if budget.nodes > MaxNodes' \
  'if len(array) > MaxListValues || budget.listValues+len(array) > MaxListValues' \
  'sort.Slice(values' \
  'func appendPointer(pointer, token string) string'; do
  grep -Fq "$anchor" <<<"$p3s01_parser" ||
    fail "P3-S01 parser safety receipt drifted: $anchor"
done
! grep -Eq 'pgx[.]|Query\(|Exec\(|Prepare\(|database/sql|internal/(contact|identity|platform|outbound)/' <<<"$p3s01_parser" ||
  fail "P3-S01 parser must not absorb SQL, execution, or another domain"

p3s01_card="$(git show :docs/execution/slices/P3-S01.md)"
for anchor in \
  'P3-S01：Segment DSL typed AST 与 fail-closed parser' \
  '重复 key 在进入 map 前拒绝' \
  'S02 SQL compiler' \
  'PENDING_EXTERNAL_GATE'; do
  grep -Fq "$anchor" <<<"$p3s01_card" ||
    fail "P3-S01 scope receipt drifted: $anchor"
done

p3s01_tests="$(git show :internal/segment/dsl/parser_test.go)"
for anchor in \
  'TestParseCanonicalAST' \
  'TestParseRejectsClosedDSLViolations' \
  'TestParseLimits' \
  'FuzzParseNeverPanicsAndCanonicalRoundTrips'; do
  grep -Fq "$anchor" <<<"$p3s01_tests" ||
    fail "P3-S01 parser test corpus receipt drifted: $anchor"
done

p3s02_compiler="$(git show :internal/segment/compiler/compiler.go)"
for anchor in \
  'Package compiler turns the closed Segment AST into a safe query program.' \
  'type Opcode string' \
  'type Program struct{ root node }' \
  '"universe": "all"' \
  'func Compile(ast dsl.AST, reference time.Time) (Program, error)' \
  'func compilePredicate(predicate dsl.Predicate, reference time.Time)' \
  'ReasonCompileUnrepresentable' \
  'ReasonCompileUnsafe'; do
  grep -Fq "$anchor" <<<"$p3s02_compiler" ||
    fail "P3-S02 QueryProgram compiler receipt drifted: $anchor"
done
! grep -Eq 'database/sql|pgx[.]|Query\(|Exec\(|Prepare\(|internal/(contact|identity|platform|outbound)/' <<<"$p3s02_compiler" ||
  fail "P3-S02 compiler must not absorb SQL, execution, or another domain"

p3s02_card="$(git show :docs/execution/slices/P3-S02.md)"
for anchor in \
  'P3-S02：Segment QueryProgram compiler' \
  '总指挥 2026-08-13 Decision B' \
  'S03 独占 index contract、固定 sqlc 查询与 executor。' \
  '不新增 DDL、customers 查询、sqlc 生成物、PG EXPLAIN、executor' \
  'PENDING_EXTERNAL_GATE'; do
  grep -Fq "$anchor" <<<"$p3s02_card" ||
    fail "P3-S02 scope receipt drifted: $anchor"
done

p3s02_tests="$(git show :internal/segment/compiler/compiler_test.go)"
for anchor in \
  'TestCompileLeafFamiliesAndStableProgram' \
  'TestCompileTableDrivenSafetyMatrix' \
  'FuzzCompileCanonicalASTIsDeterministicAndSQLFree' \
  'if len(cases) != 50' \
  'assertNoSQLText'; do
  grep -Fq "$anchor" <<<"$p3s02_tests" ||
    fail "P3-S02 compiler test corpus receipt drifted: $anchor"
done

p3s03_migration="$(git show :migrations/00009_segment_query_indexes.sql)"
for anchor in \
  'CREATE INDEX idx_customers_segment_stage' \
  'ON customers (stage_id, id);' \
  'CREATE INDEX idx_customers_segment_owner' \
  'ON customers (owner_staff_id, id);' \
  'CREATE INDEX idx_customers_segment_channel' \
  'ON customers (channel_id, id);' \
  'CREATE INDEX idx_customers_segment_added' \
  'ON customers (added_at, id);' \
  'CREATE INDEX idx_customers_segment_interact' \
  'ON customers (last_interact_at, id);' \
  'CREATE INDEX idx_customers_segment_deleted' \
  'ON customers (is_deleted, id);' \
  'DROP INDEX idx_customers_segment_deleted;' \
  'DROP INDEX idx_customers_segment_stage;'; do
  grep -Fq "$anchor" <<<"$p3s03_migration" ||
    fail "P3-S03 index contract drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE INDEX idx_customers_segment_' <<<"$p3s03_migration")" -eq 6 &&
   "$(grep -Ec '^DROP INDEX idx_customers_segment_' <<<"$p3s03_migration")" -eq 6 ]] ||
  fail "P3-S03 migration must create and drop exactly six Segment customer indexes"
! grep -Eiq 'CREATE[[:space:]]+TABLE|INSERT[[:space:]]+INTO|UPDATE[[:space:]]|DELETE[[:space:]]+FROM|identit|customer_events' <<<"$p3s03_migration" ||
  fail "P3-S03 migration must remain index-only and outside Identity/timeline semantics"

p3s03_queries="$(git show :internal/segment/store/queries/audience.sql)"
for query_name in \
  SelectSegmentUniverse SelectSegmentStageEqual SelectSegmentStageAny \
  SelectSegmentOwnerEqual SelectSegmentOwnerAny SelectSegmentChannelEqual \
  SelectSegmentChannelAny SelectSegmentTagAny SelectSegmentAddedBefore \
  SelectSegmentAddedAfter SelectSegmentLastInteractBefore \
  SelectSegmentLastInteractAfter SelectSegmentDeletedEqual; do
  grep -Fq -- "-- name: $query_name :many" <<<"$p3s03_queries" ||
    fail "P3-S03 fixed sqlc query missing: $query_name"
done
[[ "$(grep -Ec '^-- name: SelectSegment[A-Za-z]+ :many$' <<<"$p3s03_queries")" -eq 13 ]] ||
  fail "P3-S03 query family must contain exactly universe plus twelve leaf queries"
! grep -Eiq 'customer_events|(^|[^A-Za-z0-9_])segments([^A-Za-z0-9_]|$)|segment_members|SELECT[[:space:]]+[*]|;.*[^[:space:]]' <<<"$p3s03_queries" ||
  fail "P3-S03 query family exceeded the read-only customer/tag boundary"

p3s03_executor="$(git show :internal/segment/compiler/executor.go)"
for anchor in \
  'type QuerySet interface {' \
  'func Execute(ctx context.Context, program Program, queries QuerySet) ([]int64, error)' \
  'return intersectIDs(universe, selected), nil' \
  'case complement:' \
  'return differenceIDs(run.universe, ids), nil' \
  'func normalizeIDs(ids []int64) ([]int64, error)'; do
  grep -Fq "$anchor" <<<"$p3s03_executor" ||
    fail "P3-S03 executor receipt drifted: $anchor"
done
! grep -Eq 'database/sql|pgx[.]|SELECT |FROM |WHERE |Query\(|Exec\(|Prepare\(' <<<"$p3s03_executor" ||
  fail "P3-S03 executor must not contain a second SQL or database implementation"

p3s03_store="$(git show :internal/segment/store/query_set.go)"
for anchor in \
  'type QuerySet struct{ queries *segmentdb.Queries }' \
  'func NewQuerySet(db segmentdb.DBTX) *QuerySet' \
  'SelectSegmentUniverse' \
  'SelectSegmentStageEqual' \
  'SelectSegmentTagAny' \
  'SelectSegmentDeletedEqual'; do
  grep -Fq "$anchor" <<<"$p3s03_store" ||
    fail "P3-S03 sqlc adapter receipt drifted: $anchor"
done
! grep -Eq 'Query\(|QueryRow\(|Exec\(|Prepare\(|database/sql|squirrel|gorm' <<<"$p3s03_store" ||
  fail "P3-S03 store must call generated sqlc methods only"

p3s03_plan_gate="$(git show :tools/query-plan-gate/main.go)"
for anchor in \
  'partitionSegmentQueries' \
  'seedSegmentPlanFixture' \
  'generate_series(1, 200000)' \
  'VACUUM (ANALYZE)' \
  'SET plan_cache_mode = force_generic_plan'; do
  grep -Fq "$anchor" <<<"$p3s03_plan_gate" ||
    fail "P3-S03 meaningful generic-plan fixture drifted: $anchor"
done
! grep -Fq 'enable_seqscan' <<<"$p3s03_plan_gate" ||
  fail "P3-S03 query-plan gate must not fake plans by disabling Seq Scan"

p3s03_tests="$(git show :internal/segment/compiler/executor_test.go)"
for anchor in \
  'TestExecuteLeafAndCombinationSemanticMatrix' \
  'if len(cases) != 61' \
  'TestExecuteUniverseComplementOrderAndFailClosed' \
  'TestExecutePropagatesContextAndStoreErrorsWithoutQueryText'; do
  grep -Fq "$anchor" <<<"$p3s03_tests" ||
    fail "P3-S03 executor test corpus receipt drifted: $anchor"
done

p3s03_card="$(git show :docs/execution/slices/P3-S03.md)"
for anchor in \
  '全仓唯一 QueryProgram→数据库客户集合实现' \
  'is_deleted=true' \
  '20 万 customers / 40 万' \
  'PENDING_EXTERNAL_GATE' \
  '不实现 segments/segment_members 刷新写入'; do
  grep -Fq "$anchor" <<<"$p3s03_card" ||
    fail "P3-S03 scope receipt drifted: $anchor"
done

r4b_migration="$(git show :migrations/00010_identity_storage.sql)"
for anchor in \
  'CREATE TABLE identities (' \
  'CREATE TABLE customer_merges (' \
  'CREATE TABLE pending_events (' \
  'CREATE TABLE identity_operation_receipts (' \
  'CREATE CONSTRAINT TRIGGER identity_operation_receipts_complete_before_commit' \
  'CREATE TRIGGER customer_merges_append_only' \
  'DROP TABLE identity_operation_receipts;' \
  'DROP TABLE identities;'; do
  grep -Fq "$anchor" <<<"$r4b_migration" ||
    fail "P3-R4B storage migration lost required contract: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE ' <<<"$r4b_migration")" -eq 4 &&
   "$(grep -Ec '^DROP TABLE ' <<<"$r4b_migration")" -eq 4 ]] ||
  fail "P3-R4B migration must own exactly four tables"
! grep -Eiq 'jsonb_path|keyvalue[(]|CREATE[[:space:]]+INDEX|TTL|river' <<<"$r4b_migration" ||
  fail "P3-R4B migration restored a prohibited JSONPath, keyvalue, index, TTL, or River contract"

r4b_ownership="$(git show :docs/architecture/table-ownership.yml)"
grep -Fq '      - identity_operation_receipts' <<<"$r4b_ownership" ||
  fail "P3-R4B receipt table lost Identity ownership"
grep -Fq 'unknown_table: deny' <<<"$r4b_ownership" ||
  fail "P3-R4B ownership policy lost unknown-table deny"
r4b_ownership_checker="$(git show :scripts/ownership/main.go)"
grep -Fq '"identity_operation_receipts": "identity"' <<<"$r4b_ownership_checker" ||
  fail "P3-R4B ownership checker lost receipt critical owner"

r4b_card="$(git show :docs/execution/slices/P3-R4B.md)"
for anchor in \
  'Identity storage-only 00010' \
  '不修改 ADR、OpenAPI、public/internal ports、auth、keyring、runtime' \
  'Contact-owned acceptance' \
  '不执行 live migration'; do
  grep -Fq "$anchor" <<<"$r4b_card" ||
    fail "P3-R4B card boundary drifted: $anchor"
done

r4b_acceptance_recipe="$(make_target_recipe 'p3-r4b-identity-storage-acceptance:')" ||
  fail "P3-R4B acceptance target must be unique"
[[ "$r4b_acceptance_recipe" = *'P3R4B_TEST_DATABASE_URL is required'* &&
   "$r4b_acceptance_recipe" = *'./acceptance/identity -args -database-url "$$P3R4B_TEST_DATABASE_URL"'* ]] ||
  fail "P3-R4B acceptance target lost its explicit database contract"
grep -Fq 'P3R4B_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-r4b-identity-storage-acceptance' \
  <(git show :.github/workflows/application-go.yml) ||
  fail "P3-R4B acceptance is disconnected from application migration CI"

r3b_migration="$(git show :migrations/00011_contact_external_event_idempotency.sql)"
for anchor in \
  'CREATE TABLE customer_event_idempotency (' \
  'FOREIGN KEY (event_occurred_at, event_id)' \
  'REFERENCES customer_events (occurred_at, id)' \
  'UNIQUE (event_occurred_at, event_id)' \
  "jsonb_typeof(payload) = 'object'" \
  'DROP TABLE customer_event_idempotency;'; do
  grep -Fq "$anchor" <<<"$r3b_migration" || fail "P3-C07C-R3B migration receipt drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE ' <<<"$r3b_migration")" -eq 1 && "$(grep -Ec '^DROP TABLE ' <<<"$r3b_migration")" -eq 1 ]] ||
  fail "P3-C07C-R3B migration must own exactly one table"
! grep -Eiq 'CREATE[[:space:]]+INDEX|ALTER[[:space:]]+TABLE|TRIGGER|FUNCTION|river|event_log' <<<"$r3b_migration" ||
  fail "P3-C07C-R3B migration exceeded storage-only registry scope"

r3b_ownership="$(git show :docs/architecture/table-ownership.yml)"
grep -Fq '      - customer_event_idempotency' <<<"$r3b_ownership" || fail "P3-C07C-R3B registry ownership drifted"
r3b_ownership_checker="$(git show :scripts/ownership/main.go)"
grep -Fq '"customer_event_idempotency": "contact"' <<<"$r3b_ownership_checker" || fail "P3-C07C-R3B critical owner guard drifted"

r3b_card="$(git show :docs/execution/slices/P3-C07C-R3B.md)"
for anchor in 'Contact external-event registry storage only' '不修改 `AppendExternalEvent` runtime、Contact port/store/sqlc' '不执行 live migration'; do
  grep -Fq "$anchor" <<<"$r3b_card" || fail "P3-C07C-R3B card boundary drifted: $anchor"
done
r3b_acceptance_recipe="$(make_target_recipe 'p3-c07c-r3b-storage-acceptance:')" || fail "P3-C07C-R3B acceptance target must be unique"
[[ "$r3b_acceptance_recipe" = *'P3C07C_R3B_TEST_DATABASE_URL is required'* && "$r3b_acceptance_recipe" = *'./acceptance/contact -args -external-event-storage-database-url "$$P3C07C_R3B_TEST_DATABASE_URL"'* ]] ||
  fail "P3-C07C-R3B acceptance target lost its explicit database contract"
grep -Fq 'P3C07C_R3B_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c07c-r3b-storage-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P3-C07C-R3B acceptance is disconnected from application migration CI"

r3c_port="$(git show :internal/contact/port/port.go)"
grep -Fq 'ErrExternalEventConflict = errors.New("external customer event conflict")' <<<"$r3c_port" ||
  fail "P3-C07C-R3C public conflict error drifted"

r3c_queries="$(git show :internal/contact/store/queries/external_events.sql)"
for anchor in \
  '-- name: LockExternalEventIdempotencyKey :exec' \
  'SELECT pg_advisory_xact_lock(' \
  "hashtextextended(sqlc.arg(idempotency_key)::text, 0)" \
  '-- name: GetExternalEventIdempotency :one' \
  '-- name: InsertExternalEventIdempotency :execrows' \
  'ON CONFLICT (idempotency_key) DO NOTHING;'; do
  grep -Fq -- "$anchor" <<<"$r3c_queries" || fail "P3-C07C-R3C SQL behavior drifted: $anchor"
done

r3c_repository="$(git show :internal/contact/store/merge_port_repository.go)"
for anchor in \
  'func (repository *MergePortRepository) AppendExternalEvent(' \
  'queries.LockExternalEventIdempotencyKey(ctx, command.IdempotencyKey)' \
  'effectiveID, err := resolveEffectiveCustomerRoot(ctx, queries, command.CustomerID)' \
  'return replayExternalEvent(ctx, queries, effectiveID, command, existing)' \
  'queries.AppendCustomerEvent(ctx' \
  'queries.InsertExternalEventIdempotency(ctx' \
  '!sameJSONObject(existing.Payload, command.Payload)' \
  'return 0, contactport.ErrExternalEventConflict'; do
  grep -Fq "$anchor" <<<"$r3c_repository" || fail "P3-C07C-R3C runtime receipt drifted: $anchor"
done
! grep -Eq 'BeginTx\(|NewUnitOfWork\(' <<<"$r3c_repository" ||
  fail "P3-C07C-R3C repository must remain transaction-bound"

r3c_acceptance="$(git show :acceptance/contact/external_event_behavior_integration_test.go)"
for anchor in \
  'const attempts = 10' \
  'errors.Is(err, platformport.ErrTransactionRequired)' \
  'errors.Is(err, contactport.ErrExternalEventConflict)' \
  'for _, replayCustomerID := range []contactport.CustomerID{mergedID, finalRootID}' \
  'return rollbackMarker' \
  'wantRegistryCount, wantEventCount int'; do
  grep -Fq "$anchor" <<<"$r3c_acceptance" || fail "P3-C07C-R3C acceptance drifted: $anchor"
done

r3c_card="$(git show :docs/execution/slices/P3-C07C-R3C.md)"
for anchor in 'behavior-only' '不修改 migration、ownership、canonical' '10-way concurrency' '不执行 live migration/cutover'; do
  grep -Fq "$anchor" <<<"$r3c_card" || fail "P3-C07C-R3C card boundary drifted: $anchor"
done
r3c_acceptance_recipe="$(make_target_recipe 'p3-c07c-r3c-behavior-acceptance:')" ||
  fail "P3-C07C-R3C acceptance target must be unique"
[[ "$r3c_acceptance_recipe" = *'P3C07C_R3C_TEST_DATABASE_URL is required'* &&
   "$r3c_acceptance_recipe" = *"-run '^TestExternalEvent'"* &&
   "$r3c_acceptance_recipe" = *'-args -database-url "$$P3C07C_R3C_TEST_DATABASE_URL"'* ]] ||
  fail "P3-C07C-R3C acceptance target lost its explicit database contract"
grep -Fq 'P3C07C_R3C_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-c07c-r3c-behavior-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P3-C07C-R3C acceptance is disconnected from application migration CI"

p3s04a_app="$(git show :internal/segment/app/refresh.go)"
for anchor in \
  'service.uow.Within(ctx' \
  'service.store.LockDefinition(txCtx, segmentID)' \
  'dsl.Parse(definition)' \
  'compiler.Compile(ast, reference)' \
  'compiler.Execute(txCtx, program, queries)' \
  'service.store.ReplaceMembers(txCtx, segmentID, customerIDs, reference)' \
  'service.events.Append(txCtx, eventport.Event{' \
  'Type:           "segment.refreshed"' \
  'IdempotencyKey: "segment.refresh:"'; do
  grep -Fq "$anchor" <<<"$p3s04a_app" ||
    fail "P3-S04A transaction or pipeline receipt drifted: $anchor"
done
if grep -Eq '(^|[^[:alnum:]_])(Query|Exec|Prepare)\(' <<<"$p3s04a_app"; then
  fail "P3-S04A app must not add direct SQL execution"
fi

p3s04a_queries="$(git show :internal/segment/store/queries/refresh.sql)"
for anchor in \
  '-- name: LockSegmentDefinitionForRefresh :one' \
  'FOR UPDATE;' \
  '-- name: DeleteSegmentMembersForRefresh :exec' \
  '-- name: InsertSegmentMembersForRefresh :exec' \
  'FROM unnest(sqlc.arg(customer_ids)::bigint[]) AS customer_id;' \
  '-- name: CompleteSegmentRefresh :one' \
  "refresh_status = 'idle'"; do
  grep -Fq -- "$anchor" <<<"$p3s04a_queries" ||
    fail "P3-S04A fixed replacement query receipt drifted: $anchor"
done
[[ "$(grep -c '^-- name:' <<<"$p3s04a_queries")" -eq 4 ]] ||
  fail "P3-S04A replacement query family must contain exactly four fixed sqlc queries"
if grep -Eiq '\b(customers|customer_tags|identities|customer_merges|river_job|event_log)\b' <<<"$p3s04a_queries"; then
  fail "P3-S04A replacement SQL exceeded Segment-owned tables"
fi

p3s04a_store="$(git show :internal/segment/store/refresh_repository.go)"
for anchor in \
  'platformstore.TxFromContext(ctx)' \
  'segmentdb.New(tx)' \
  'return NewQuerySet(tx), nil' \
  'queries.DeleteSegmentMembersForRefresh' \
  'queries.InsertSegmentMembersForRefresh' \
  'queries.CompleteSegmentRefresh'; do
  grep -Fq "$anchor" <<<"$p3s04a_store" ||
    fail "P3-S04A transaction-bound store receipt drifted: $anchor"
done
if grep -Eq '\.(Query|QueryRow|Exec|Prepare)\(' <<<"$p3s04a_store"; then
  fail "P3-S04A store must call generated sqlc methods only"
fi

p3s04a_acceptance="$(git show :acceptance/segment/refresh_integration_test.go)"
for anchor in \
  'TestRefreshOncePG16ReplacesMembersAtomically' \
  'TestRefreshOncePG16RollsBackPartialReplacementAndRejectsInjection' \
  'TestRefreshOncePG16SerializesConcurrentSameSegmentCalls' \
  'SEGMENT_TEST_DATABASE_URL' \
  'version != "160014"' \
  'reject_refresh_event'; do
  grep -Fq "$anchor" <<<"$p3s04a_acceptance" ||
    fail "P3-S04A PG16 acceptance receipt drifted: $anchor"
done

p3s04a_card="$(git show :docs/execution/slices/P3-S04A.md)"
for anchor in \
  'P3-S04A：Segment 单次事务成员替换核心' \
  'S03 固定 query family' \
  '任一解析、查询、写入或 event append 失败时' \
  'S04B：cron 严格语法/规范化校验' \
  'S04C：Create/Update/RequestRefresh durable idempotency receipt' \
  'PENDING_EXTERNAL_GATE' \
  '不接 River'; do
  grep -Fq "$anchor" <<<"$p3s04a_card" ||
    fail "P3-S04A scope or dependency receipt drifted: $anchor"
done

p3s04b_cron="$(git show :internal/segment/app/cron.go)"
for anchor in \
  'func CanonicalRefreshCron(mode segmentport.RefreshMode, refreshCron *string)' \
  'case segmentport.RefreshModeManual:' \
  'case segmentport.RefreshModeScheduled:' \
  'if refreshCron != nil {' \
  'if refreshCron == nil {' \
  'if len(fields) != 5 {' \
  'ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}' \
  'if canonical[2] != "*" && canonical[4] != "*" {' \
  'return strings.Join(canonical, " "), nil' \
  'if seen[position] {' \
  'if character < '\''0'\'' || character > '\''9'\'' {'; do
  grep -Fq "$anchor" <<<"$p3s04b_cron" ||
    fail "P3-S04B cron validation receipt drifted: $anchor"
done
if grep -Eq 'time\.(Ticker|AfterFunc)|go[[:space:]]+func|\.(Query|QueryRow|Exec|Prepare)\(' <<<"$p3s04b_cron" ||
   grep -Fq '"time"' <<<"$p3s04b_cron" ||
   grep -Fq 'github.com/riverqueue' <<<"$p3s04b_cron" ||
   grep -Fq 'internal/platform/scheduler' <<<"$p3s04b_cron"; then
  fail "P3-S04B must remain a pure validation slice without timers River or SQL"
fi

p3s04b_tests="$(git show :internal/segment/app/cron_test.go)"
for anchor in \
  'TestCanonicalRefreshCron' \
  'manual cron rejected' \
  'ambiguous day fields rejected' \
  'FuzzCanonicalRefreshCron' \
  'canonical cron did not round-trip'; do
  grep -Fq "$anchor" <<<"$p3s04b_tests" ||
    fail "P3-S04B validation test receipt drifted: $anchor"
done

p3s04b_card="$(git show :docs/execution/slices/P3-S04B.md)"
for anchor in \
  'P3-S04B：Segment cron 严格校验' \
  'manual` 只接受 `refresh_cron=nil`' \
  '五字段数字 cron' \
  'day-of-month 与 day-of-week 同时受限' \
  '不引入第三方 cron 库、`time.Ticker`、`time.AfterFunc`' \
  'S04E 才把已验证的 scheduled 定义注册到唯一' \
  'PENDING_EXTERNAL_GATE'; do
  grep -Fq "$anchor" <<<"$p3s04b_card" ||
    fail "P3-S04B card boundary drifted: $anchor"
done

w4_migration="$(git show :migrations/00015_wecom_sync_state.sql)"
for anchor in \
  'CREATE TABLE wecom_sync_state (' \
  'sync_key     TEXT PRIMARY KEY' \
  'cursor       TEXT NOT NULL DEFAULT' \
  'completed_at TIMESTAMPTZ' \
  'DROP TABLE wecom_sync_state;'; do
  grep -Fq -- "$anchor" <<<"$w4_migration" ||
    fail "P3-W4 migration receipt drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE ' <<<"$w4_migration")" -eq 1 && "$(grep -Ec '^DROP TABLE ' <<<"$w4_migration")" -eq 1 ]] ||
  fail "P3-W4 migration must own exactly wecom_sync_state"
if grep -Eiq 'CREATE[[:space:]]+TABLE[[:space:]]+(customers|identities|pending_events|customer_events|event_log|river_)' <<<"$w4_migration"; then
  fail "P3-W4 migration exceeded WeCom state ownership"
fi

w4_queries="$(git show :internal/wecom/store/queries/sync_state.sql)"
for anchor in \
  '-- name: LoadWeComSyncState :one' \
  '-- name: AdvanceWeComSyncState :one' \
  'ON CONFLICT (sync_key) DO UPDATE' \
  'WHERE wecom_sync_state.cursor = sqlc.arg(expected_cursor)::text' \
  'wecom_sync_state.completed_at IS NULL'; do
  grep -Fq -- "$anchor" <<<"$w4_queries" ||
    fail "P3-W4 cursor CAS query receipt drifted: $anchor"
done
if grep -Eiq '\b(customers|identities|pending_events|customer_events|event_log|river_job)\b' <<<"$w4_queries"; then
  fail "P3-W4 cursor SQL exceeded WeCom state ownership"
fi

w4_service="$(git show :internal/wecom/app/sync_contacts.go)"
for anchor in \
  'ListExternalContacts(context.Context, string, string)' \
  'page.NextCursor != "" && page.NextCursor == state.Cursor' \
  'page.NextCursor == ""' \
  'ErrCursorSyncDone' \
  'maxCursorAdvanceAttempts = 3'; do
  grep -Fq -- "$anchor" <<<"$w4_service" ||
    fail "P3-W4 resume service receipt drifted: $anchor"
done
if grep -Eq 'internal/(identity|contact|outbound)|/callback|/worker' <<<"$w4_service"; then
  fail "P3-W4 service exceeded cursor-resume boundary"
fi

w4_store="$(git show :internal/wecom/store/sync_state_repository.go)"
for anchor in \
  'platformstore.TxFromContext(ctx)' \
  'wecomdb.New(tx).AdvanceWeComSyncState' \
  'return wecomapp.ErrCursorAdvanced'; do
  grep -Fq -- "$anchor" <<<"$w4_store" ||
    fail "P3-W4 transaction-bound store receipt drifted: $anchor"
done
if grep -Eq '\.(Query|QueryRow|Exec|Prepare)\(' <<<"$w4_store"; then
  fail "P3-W4 store must call generated sqlc methods only"
fi

w4_card="$(git show :docs/execution/slices/P3-W4.md)"
for anchor in \
  '00015_wecom_sync_state.sql' \
  '不 push、不建 PR' \
  '不建档、不归因、不写 Contact/Identity' \
  '真实企微、生产数据库、live migration/cutover、外发均为 `NOT EXECUTED`'; do
  grep -Fq -- "$anchor" <<<"$w4_card" ||
    fail "P3-W4 card boundary drifted: $anchor"
done
w4_acceptance_recipe="$(make_target_recipe 'p3-w4-acceptance:')" || fail "P3-W4 acceptance target must be unique"
[[ "$w4_acceptance_recipe" = *'WECOM_SYNC_TEST_DATABASE_URL is required'* && "$w4_acceptance_recipe" = *'./acceptance/wecom'* ]] ||
  fail "P3-W4 acceptance target lost its PostgreSQL contract"
grep -Fq 'WECOM_SYNC_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-w4-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P3-W4 acceptance is disconnected from application migration CI"

w5_service="$(git show :internal/wecom/app/identity_contact.go)"
for anchor in \
  'IdentityContactFromCallback IdentityContactFactSource = "callback_inbox"' \
  'IdentityContactFromSync     IdentityContactFactSource = "directory_sync"' \
  'Kind:      identityport.KindWeComExternalUserID' \
  'Scope:     "wecom-corp:" + fact.CorpID' \
  'Assurance: identityport.AssuranceVerified' \
  'processor.identity.Ingest(ctx, command)' \
  'sha256.Sum256([]byte("wecom.identity-contact.v1\x00"' \
  'ErrIdentityContactIngestFailed' \
  'identityport.IngestConflict'; do
  grep -Fq -- "$anchor" <<<"$w5_service" ||
    fail "P3-W5 Identity ingest adapter receipt drifted: $anchor"
done
if grep -Eq 'internal/(contact|identity)/(app|store)|internal/platform/store|\.(Query|QueryRow|Exec|Prepare)\(|github.com/riverqueue|net/http' <<<"$w5_service"; then
  fail "P3-W5 adapter bypassed the frozen Identity port or added runtime/provider work"
fi

w5_tests="$(git show :internal/wecom/app/identity_contact_test.go)"
for anchor in \
  'TestIdentityContactProcessorMapsVerifiedFactsToIdentityIngest' \
  'TestIdentityContactProcessorReplayUsesStableCommandAndResult' \
  'TestIdentityContactProcessorScopesIdempotencyByCorp' \
  'TestIdentityContactProcessorSurfacesRetryableFailureWithoutChangingCommand' \
  'TestIdentityContactProcessorFailsClosedForInvalidInputOrResult' \
  'command leaked raw input'; do
  grep -Fq -- "$anchor" <<<"$w5_tests" ||
    fail "P3-W5 acceptance receipt drifted: $anchor"
done

w5_card="$(git show :docs/execution/slices/P3-W5.md)"
for anchor in \
  'W2_DURABLE_INBOX_CRITICAL_JOB_PAYLOAD_MISSING' \
  'W4_DURABLE_ITEM_HANDOFF_MISSING' \
  'I5_INGEST_IMPLEMENTATION_MISSING' \
  '没有修改 O2 已合并的连续 `00016`' \
  'O2 #182 已 CLOSED' \
  '已安全重放 exact-green main' \
  '所有外部门均为 `NOT EXECUTED`'; do
  grep -Fq -- "$anchor" <<<"$w5_card" ||
    fail "P3-W5 dependency or hard-boundary receipt drifted: $anchor"
done
w5_acceptance_recipe="$(make_target_recipe 'p3-w5-acceptance:')" || fail "P3-W5 acceptance target must be unique"
[[ "$w5_acceptance_recipe" = *'$(GO) test -race -count=1 -timeout=30s ./internal/wecom/app'* ]] ||
  fail "P3-W5 acceptance target lost the Identity ingest adapter tests"

p3s05b_migration="$(git show :migrations/00018_segment_crud_receipts.sql)"
for anchor in \
  'CREATE TABLE public.segment_operation_receipts (' \
  "CHECK (operation IN ('create', 'update'))" \
  'UNIQUE (operation, actor_scope, key_digest)' \
  'octet_length(key_digest) = 32' \
  'octet_length(payload_digest) = 32' \
  'DEFERRABLE INITIALLY DEFERRED' \
  'SET search_path = pg_catalog' \
  'FROM public.segment_operation_receipts' \
  'completed segment operation receipts are immutable' \
  'DROP TABLE public.segment_operation_receipts;'; do
  grep -Fq -- "$anchor" <<<"$p3s05b_migration" || fail "P3-S05B-R2 receipt migration drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE ' <<<"$p3s05b_migration")" -eq 1 ]] || fail "P3-S05B-R2 migration must create exactly one Segment receipt table"
! grep -Eiq 'CREATE[[:space:]]+TABLE[[:space:]]+(customers|identities|pending_events|customer_events|event_log|river_)' <<<"$p3s05b_migration" ||
  fail "P3-S05B-R2 migration exceeded Segment ownership"

p3s05b_ownership="$(git show :docs/architecture/table-ownership.yml)"
grep -Fq '    tables: [segments, segment_members, segment_refresh_receipts, segment_operation_receipts]' <<<"$p3s05b_ownership" ||
  fail "P3-S05B-R2 operation receipt lost Segment ownership"
grep -Fq '      segment_operation_receipts: transaction_scoped_crud_idempotency_receipts' <<<"$p3s05b_ownership" ||
  fail "P3-S05B-R2 receipt ownership constraint drifted"

p3s05b_queries="$(git show :internal/segment/store/queries/crud.sql)"
for anchor in \
  '-- name: ReserveSegmentOperationReceipt :one' \
  'ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING' \
  '-- name: CompleteSegmentOperationReceipt :one' \
  '-- name: ListSegments :many' \
  '-- name: ListSegmentMemberRecords :many'; do
  grep -Fq -- "$anchor" <<<"$p3s05b_queries" || fail "P3-S05B-R2 fixed sqlc query receipt drifted: $anchor"
done
! grep -Eiq '(EXECUTE[[:space:]]|format\(|information_schema|pg_catalog)' <<<"$p3s05b_queries" || fail "P3-S05B-R2 query family gained a SQL escape hatch"

p3s05b_app="$(git show :internal/segment/app/crud.go)"
for anchor in \
  'service.uow.Within(ctx' \
  'sha256.Sum256([]byte(key))' \
  'subtle.ConstantTimeCompare' \
  'Type: "segment." + string(operation) + "d"' \
  'service.store.CompleteReceipt'; do
  grep -Fq -- "$anchor" <<<"$p3s05b_app" || fail "P3-S05B-R2 same-UoW CRUD boundary drifted: $anchor"
done
! grep -Eq 'internal/(contact|identity|outbound)/(app|store|http|worker)|time[.](Ticker|AfterFunc)' <<<"$p3s05b_app" || fail "P3-S05B-R2 app crossed its frozen domain/runtime boundary"

p3s05b_runtime="$(git show :cmd/aicrm/api.go)"
for anchor in \
  '{http.MethodGet, "/api/v1/segments", authport.CapabilitySegmentsRead' \
  '{http.MethodPost, "/api/v1/segments", authport.CapabilitySegmentsWrite, true' \
  '{http.MethodPatch, "/api/v1/segments/{segment_id}", authport.CapabilitySegmentsWrite, true' \
  '{http.MethodGet, "/api/v1/segments/{segment_id}/members", authport.CapabilitySegmentsRead'; do
  grep -Fq -- "$anchor" <<<"$p3s05b_runtime" || fail "P3-S05B-R2 runtime route drifted: $anchor"
done

p3s05b_card="$(git show :docs/execution/slices/P3-S05B-R2.md)"
for anchor in '00018' '同一 PostgreSQL UoW' '不改 OpenAPI、public port、DSL/compiler、River、UI' 'PENDING_EXTERNAL_GATE'; do
  grep -Fq -- "$anchor" <<<"$p3s05b_card" || fail "P3-S05B-R2 card boundary drifted: $anchor"
done
p3s05b_acceptance_recipe="$(make_target_recipe 'p3-s05b-acceptance:')" || fail "P3-S05B-R2 acceptance target must be unique"
[[ "$p3s05b_acceptance_recipe" = *'SEGMENT_CRUD_TEST_DATABASE_URL is required'* && "$p3s05b_acceptance_recipe" = *'TestSegmentCRUDReceiptAndRuntimeFlow'* ]] ||
  fail "P3-S05B-R2 acceptance target lost its PostgreSQL contract"
grep -Fq 'SEGMENT_CRUD_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-s05b-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P3-S05B-R2 PG behavior acceptance is disconnected from application CI"

p3o4_migration="$(git show :migrations/00020_outbound_send_attempts.sql)"
for anchor in 'CREATE TABLE outbound_send_attempts' "state IN ('reserved', 'dispatching', 'succeeded', 'retryable_failed', 'final_failed', 'outcome_unknown')" "job_kind IN ('outbound_enqueue_one', 'outbound_enqueue_batch_task')" 'UNIQUE'; do
  grep -Fq -- "$anchor" <<<"$p3o4_migration" || fail "P3-O4 stable attempt migration drifted: $anchor"
done
! grep -Eiq 'CREATE[[:space:]]+TABLE[[:space:]]+(customers|identities|segments|event_log|river_)' <<<"$p3o4_migration" || fail "P3-O4 migration exceeded Outbound ownership"

p3o4_sender="$(git show :internal/outbound/app/sender.go)"
for anchor in 'service.repository.ReserveSendAttempt' 'service.repository.StartSendAttempt' 'service.provider.Send(ctx, request)' 'ProviderFailureInterruptedDispatch' 'SendAttemptOutcomeUnknown'; do
  grep -Fq -- "$anchor" <<<"$p3o4_sender" || fail "P3-O4 sender safety boundary drifted: $anchor"
done
! grep -Eiq 'wecom|wechatwork|workwx|internal/(contact|identity|segment)/' <<<"$p3o4_sender" || fail "P3-O4 sender gained a real provider or forbidden cross-domain dependency"

p3o4_worker="$(git show :internal/outbound/worker/sender.go)"
for anchor in 'QueueOutbound' 'NewTokenBucket' 'OutboundEnqueueOneJobKind' 'OutboundEnqueueBatchJobKind'; do
  grep -Fq -- "$anchor" <<<"$p3o4_worker" || fail "P3-O4 River/token-bucket boundary drifted: $anchor"
done

p3o4_card="$(git show :docs/execution/slices/P3-O4.md)"
for anchor in '00020' '测试 provider' '绝不执行真实外发' '不改 `outbound_tasks` 业务状态' '不含 O5-O8'; do
  grep -Fq -- "$anchor" <<<"$p3o4_card" || fail "P3-O4 frozen scope card drifted: $anchor"
done
p3o4_recipe="$(make_target_recipe 'p3-o4-sender-acceptance:')" || fail "P3-O4 acceptance target must be unique"
[[ "$p3o4_recipe" = *'P3O4_SENDER_TEST_DATABASE_URL is required'* && "$p3o4_recipe" = *'TestSender'* ]] || fail "P3-O4 acceptance target lost its PostgreSQL contract"
grep -Fq 'P3O4_SENDER_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-o4-sender-acceptance' <(git show :.github/workflows/application-go.yml) || fail "P3-O4 PG acceptance is disconnected from application CI"
grep -Fq 'outbound_send_attempts: stable_provider_attempt_receipts_owned_by_outbound' <(git show :docs/architecture/table-ownership.yml) || fail "P3-O4 receipt lost Outbound ownership"

p3o5_migration="$(git show :migrations/00021_outbound_task_status.sql)"
for anchor in \
  "status IN ('pending', 'sending', 'sent', 'retryable_failed', 'final_failed', 'outcome_unknown', 'cancelled')" \
  'ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0' \
  'ADD COLUMN current_attempt_id BIGINT REFERENCES outbound_send_attempts(id)' \
  'ADD CONSTRAINT outbound_tasks_status_lifecycle'; do
  grep -Fq -- "$anchor" <<<"$p3o5_migration" || fail "P3-O5 task status migration drifted: $anchor"
done
! grep -Eiq 'next_retry_at|CREATE[[:space:]]+TABLE|river_job|wecom|wechatwork|workwx' <<<"$p3o5_migration" || fail "P3-O5 migration exceeded status-only ownership"

p3o5_sender="$(git show :internal/outbound/app/sender.go)"
for anchor in \
  'service.repository.MarkTaskSending' \
  'service.repository.ProjectTaskResult' \
  'eventport.EvOutboundSent' \
  'eventport.EvOutboundFailed' \
  'outbound.send-result:%d' \
  'fact.OccurredAt'; do
  grep -Fq -- "$anchor" <<<"$p3o5_sender" || fail "P3-O5 atomic status/event boundary drifted: $anchor"
done
! grep -Eiq 'wecom|wechatwork|workwx|next_retry|InsertMany|JobInsert|River.*Insert' <<<"$p3o5_sender" || fail "P3-O5 gained provider or retry scheduling"

p3o5_queries="$(git show :internal/outbound/store/queries/send_attempts.sql)"
for anchor in '-- name: MarkOutboundTaskSending :execrows' '-- name: ProjectOutboundTaskResult :execrows' "SET status = 'sending'" 'status_updated_at = history.completed_at'; do
  grep -Fq -- "$anchor" <<<"$p3o5_queries" || fail "P3-O5 status projection query drifted: $anchor"
done
! grep -Eiq 'INSERT[[:space:]]+INTO[[:space:]]+(river_job|outbound_batches)|DELETE[[:space:]]+FROM|next_retry' <<<"$p3o5_queries" || fail "P3-O5 query gained retry, batch, or deletion semantics"

p3o5_card="$(git show :docs/execution/slices/P3-O5.md)"
for anchor in '00021' '不在 O5 创建任何 River 重试 job' '绝不执行真实外发' '不含 O6 重试/取消、O7 API、O8 UI'; do
  grep -Fq -- "$anchor" <<<"$p3o5_card" || fail "P3-O5 frozen scope card drifted: $anchor"
done
p3o5_recipe="$(make_target_recipe 'p3-o5-status-acceptance:')" || fail "P3-O5 acceptance target must be unique"
[[ "$p3o5_recipe" = *'P3O5_STATUS_TEST_DATABASE_URL is required'* && "$p3o5_recipe" = *'TestSender'* ]] || fail "P3-O5 acceptance target lost its PostgreSQL contract"
grep -Fq 'P3O5_STATUS_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-o5-status-acceptance' <(git show :.github/workflows/application-go.yml) || fail "P3-O5 PG acceptance is disconnected from application CI"
grep -Fq 'outbound_tasks: provider_result_driven_status_projection_without_retry_scheduling' <(git show :docs/architecture/table-ownership.yml) || fail "P3-O5 task status lost Outbound ownership"

p3o6a_migration="$(git show :migrations/00022_outbound_send_attempt_history.sql)"
for anchor in \
  'CREATE TABLE IF NOT EXISTS outbound_send_attempt_history' \
  'UNIQUE (send_attempt_id, river_attempt)' \
  "state IN ('reserved', 'dispatching', 'succeeded', 'retryable_failed', 'final_failed', 'outcome_unknown')" \
  '-- +goose Down' \
  'SELECT 1;'; do
  grep -Fq -- "$anchor" <<<"$p3o6a_migration" || fail "P3-O6A attempt-history migration drifted: $anchor"
done
! grep -Eiq 'DROP[[:space:]]+TABLE|CREATE[[:space:]]+TABLE[[:space:]]+(river_|customers|identities|segments|event_log)' <<<"$p3o6a_migration" ||
  fail "P3-O6A migration lost history-preserving Outbound-only down compatibility"

p3o6a_queries="$(git show :internal/outbound/store/queries/send_attempts.sql)"
for anchor in \
  '-- name: BackfillOutboundFirstAttemptHistory :execrows' \
  '-- name: ReserveOutboundAttemptHistory :one' \
  '-- name: PrepareOutboundSendAttemptRetry :one' \
  '-- name: LoadOutboundTaskResultFact :one' \
  'RETURNING history.id, history.send_attempt_id, history.river_attempt, history.river_max_attempts' \
  'previous.state = '\''retryable_failed'\'''; do
  grep -Fq -- "$anchor" <<<"$p3o6a_queries" || fail "P3-O6A attempt lifecycle query drifted: $anchor"
done
! grep -Eiq '(FROM|JOIN|UPDATE|INTO)[[:space:]]+(public[.])?river_job|next_retry_at' <<<"$p3o6a_queries" ||
  fail "P3-O6A Outbound SQLC query gained River catalog or a second retry scheduler"

p3o6a_worker="$(git show :internal/outbound/worker/sender.go)"
for anchor in \
  'RiverAttempt: int32(job.Attempt)' \
  'RiverMaxAttempts: int32(job.MaxAttempts)' \
  'RiverJobState: string(job.State)' \
  'if attempt.State == outboundapp.SendAttemptRetryableFailed' \
  'return ErrRetryableSendAttempt'; do
  grep -Fq -- "$anchor" <<<"$p3o6a_worker" || fail "P3-O6A River retry boundary drifted: $anchor"
done

p3o6a_compat="$(git show :acceptance/outbound/o6a_migration_compatibility.sh)"
for anchor in \
  "goose -dir migrations postgres \"\$database_url\" down" \
  'ON CONFLICT (river_job_id) DO UPDATE' \
  '[[ "$rollback_waterline" = "21" && "$rollback_history" = "2" ]]' \
  '[[ "$upgrade_waterline" = "35" && "$upgrade_history" = "2" && "$marker_count" = "1" ]]'; do
  grep -Fq -- "$anchor" <<<"$p3o6a_compat" || fail "P3-O6A historical migration acceptance drifted: $anchor"
done

p3o6a_card="$(git show :docs/execution/slices/P3-O6A.md)"
for anchor in '00022' 'outcome_unknown' 'Outbound SQLC schema corpus' 'LEGACY-S06-026' '绝不执行真实外发' '不含 cancel、manual retry'; do
  grep -Fq -- "$anchor" <<<"$p3o6a_card" || fail "P3-O6A frozen scope card drifted: $anchor"
done
p3o6a_recipe="$(make_target_recipe 'p3-o6a-retry-acceptance:')" || fail "P3-O6A acceptance target must be unique"
[[ "$p3o6a_recipe" = *'P3O6A_RETRY_TEST_DATABASE_URL is required'* && "$p3o6a_recipe" = *'o6a_migration_compatibility.sh'* && "$p3o6a_recipe" = *'TestOutboundOutcomeUnknownIsNotRetriedByRealRiver'* ]] ||
  fail "P3-O6A acceptance target lost real retry, history compatibility, or unknown-outcome coverage"
grep -Fq 'P3O6A_RETRY_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-o6a-retry-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P3-O6A PG/River acceptance is disconnected from application CI"
grep -Fq 'outbound_send_attempt_history: river_attempt_lifecycle_history_without_business_retry_scheduling' <(git show :docs/architecture/table-ownership.yml) ||
  fail "P3-O6A attempt history lost Outbound ownership"

p3o6b1_migration="$(git show :migrations/00023_outbound_cancel_control.sql)"
for anchor in \
  'CREATE TABLE IF NOT EXISTS outbound_task_job_links' \
  'UNIQUE (task_id, generation)' \
  'UNIQUE (river_job_id)' \
  'CREATE TABLE IF NOT EXISTS outbound_control_receipts' \
  "operation = 'cancel'" \
  "state IN ('reserved', 'completed')" \
  "task_status = 'cancelled'" \
  '-- +goose Down' \
  'SELECT 1;'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_migration" || fail "P3-O6B1 cancel migration drifted: $anchor"
done
! grep -Eiq 'REFERENCES[[:space:]]+(public[.])?river_job|DROP[[:space:]]+(TABLE|COLUMN|CONSTRAINT|INDEX)|manual[_ -]?retry' <<<"$p3o6b1_migration" ||
  fail "P3-O6B1 migration gained a River FK, destructive down, or manual retry scope"

p3o6b1_app="$(git show :internal/outbound/app/control.go)"
for anchor in \
  'target.Status != TaskStatusPending' \
  'ErrCancelTransitionConflict' \
  'ErrCancelWorkerWon' \
  'outbound.cancelled' \
  'service.repository.DeletePendingTaskJob' \
  'service.events.Append' \
  'service.repository.CompleteCancel'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_app" || fail "P3-O6B1 cancel service drifted: $anchor"
done
! grep -Eiq 'manual[_ -]?retry|outcome_unknown.*retry|net/http|wecom|wechatwork|workwx' <<<"$p3o6b1_app" ||
  fail "P3-O6B1 cancel service gained retry, HTTP, or provider scope"

p3o6b1_queries="$(git show :internal/outbound/store/queries/control.sql)"
for anchor in \
  '-- name: LockOutboundTaskForCancel :one' \
  'FOR UPDATE;' \
  '-- name: RecordOutboundTaskJobLink :one' \
  '-- name: MarkOutboundTaskJobCancelled :one' \
  '-- name: MarkOutboundTaskCancelled :one' \
  '-- name: CompleteOutboundCancelReceipt :one'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_queries" || fail "P3-O6B1 Outbound SQLC drifted: $anchor"
done
! grep -Eiq '(FROM|JOIN|UPDATE|INTO|DELETE[[:space:]]+FROM)[[:space:]]+(public[.])?river_job|next_retry_at' <<<"$p3o6b1_queries" ||
  fail "P3-O6B1/O6B2 Outbound SQLC gained River catalog or retry scheduling"

p3o6b1_platform="$(git show :internal/platform/jobqueue/outbound_cancel.go)"
for anchor in \
  'client.client.JobGetTx' \
  'client.client.JobListTx' \
  'client.client.JobDeleteTx' \
  'Queues(outboundQueueName)' \
  'Kinds(outboundEnqueueOneKind, outboundEnqueueBatchKind)' \
  'outboundTaskID(candidate)'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_platform" || fail "P3-O6B1 typed River adapter drifted: $anchor"
done
! grep -Eq '\.Where\(|\.(Query|QueryRow|Exec|Prepare|SendBatch)\(' <<<"$p3o6b1_platform" ||
  fail "P3-O6B1 typed River adapter gained dynamic SQL or a direct database call"

p3o6b1_compat="$(git show :acceptance/outbound/o6b1_migration_compatibility.sh)"
for anchor in \
  '[[ "$rollback_waterline" = "22" && "$receipts" = "1" && "$links" = "1" ]]' \
  '[[ "$upgrade_waterline" = "35" && "$receipts" = "1" && "$links" = "1" ]]' \
  '[[ "$events" = "1" && "$jobs" = "0" && "$task_status" = "cancelled" ]]' \
  '[[ "$outbound_links" = "1" && "$river_foreign_keys" = "0" ]]'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_compat" || fail "P3-O6B1 historical migration acceptance drifted: $anchor"
done

p3o6b1_card="$(git show :docs/execution/slices/P3-O6B1.md)"
for anchor in '00023' '只允许 `pending → cancelled`' 'JobListTx' '不提前携带 manual retry' 'legacy cancel route' '真实外发' 'REAL_WECOM_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_card" || fail "P3-O6B1 frozen scope card drifted: $anchor"
done
p3o6b1_recipe="$(make_target_recipe 'p3-o6b1-cancel-acceptance:')" || fail "P3-O6B1 acceptance target must be unique"
[[ "$p3o6b1_recipe" = *'P3O6B1_CANCEL_TEST_DATABASE_URL is required'* && "$p3o6b1_recipe" = *'o6b1_migration_compatibility.sh'* && "$p3o6b1_recipe" = *'TestCancel'* ]] ||
  fail "P3-O6B1 acceptance target lost cancellation, race, or migration coverage"
grep -Fq 'P3O6B1_CANCEL_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-o6b1-cancel-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P3-O6B1 PG/River acceptance is disconnected from application CI"
grep -Fq 'outbound_task_job_links: immutable_outbound_task_to_river_job_generation_link' <(git show :docs/architecture/table-ownership.yml) ||
  fail "P3-O6B1 task/job link lost Outbound ownership"
grep -Fq 'outbound_control_receipts: transaction_scoped_idempotent_outbound_control_results' <(git show :docs/architecture/table-ownership.yml) ||
  fail "P3-O6B1 control receipt lost Outbound ownership"

p3o6b2_migration="$(git show :migrations/00024_outbound_manual_retry_control.sql)"
for anchor in \
  "operation IN ('cancel', 'manual_retry')" \
  "operation = 'cancel' AND task_status = 'cancelled'" \
  "operation = 'manual_retry' AND task_status = 'pending'" \
  '-- +goose Down' \
  'SELECT 1;'; do
  grep -Fq -- "$anchor" <<<"$p3o6b2_migration" || fail "P3-O6B2 migration drifted: $anchor"
done
! grep -Eiq 'CREATE[[:space:]]+TABLE|REFERENCES[[:space:]]+(public[.])?river_job|DROP[[:space:]]+(TABLE|COLUMN|INDEX)' <<<"$p3o6b2_migration" ||
  fail "P3-O6B2 migration gained a table, River FK, or destructive down"

p3o6b2_app="$(git show :internal/outbound/app/manual_retry.go)"
for anchor in \
  'target.Status != TaskStatusFinalFailed && target.Status != TaskStatusCancelled' \
  'job.Generation != target.Job.Generation+1' \
  'outbound.manual_retry_requested' \
  'service.repository.InsertManualRetryJob' \
  'service.events.Append' \
  'service.repository.CompleteManualRetry'; do
  grep -Fq -- "$anchor" <<<"$p3o6b2_app" || fail "P3-O6B2 manual retry service drifted: $anchor"
done
! grep -Eiq 'outcome_unknown[^\n]*(allow|retry)|retryable_failed[^\n]*(allow|manual)|net/http|wecom|wechatwork|workwx' <<<"$p3o6b2_app" ||
  fail "P3-O6B2 service gained unknown/retryable retry, HTTP, or provider scope"

for anchor in \
  '-- name: ReserveOutboundManualRetryReceipt :one' \
  '-- name: LockOutboundTaskForManualRetry :one' \
  'FOR UPDATE OF task;' \
  '-- name: MarkOutboundTaskManualRetryPending :one' \
  "task.status IN ('final_failed', 'cancelled')" \
  '-- name: CompleteOutboundManualRetryReceipt :one'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_queries" || fail "P3-O6B2 Outbound SQLC drifted: $anchor"
done

for anchor in \
  'type outboundManualRetryOneArgs struct' \
  'type outboundManualRetryBatchArgs struct' \
  'client.client.InsertTx' \
  'Queue: outboundQueueName'; do
  grep -Fq -- "$anchor" <<<"$p3o6b1_platform" || fail "P3-O6B2 typed River adapter drifted: $anchor"
done
! grep -Eq '\.Where\(|\.(Query|QueryRow|Exec|Prepare|SendBatch)\(' <<<"$p3o6b1_platform" ||
  fail "P3-O6B2 typed River adapter gained dynamic SQL or a direct database call"

p3o6b2_compat="$(git show :acceptance/outbound/o6b2_migration_compatibility.sh)"
for anchor in \
  'goose -dir migrations postgres "$database_url" down-to 23' \
  '[[ "$rollback_waterline" = "23" && "$receipts" = "1" && "$links" = "2" ]]' \
  '[[ "$upgrade_waterline" = "35" && "$receipts" = "1" && "$links" = "2" ]]' \
  '[[ "$events" = "1" && "$jobs" = "1" && "$task_status" = "pending" ]]'; do
  grep -Fq -- "$anchor" <<<"$p3o6b2_compat" || fail "P3-O6B2 historical migration acceptance drifted: $anchor"
done

p3o6b2_card="$(git show :docs/execution/slices/P3-O6B2.md)"
for anchor in '00024' '`final_failed/cancelled → pending`' '`retryable_failed/outcome_unknown/sent/sending/pending`' 'typed args' 'O7' 'REAL_WECOM_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p3o6b2_card" || fail "P3-O6B2 frozen scope card drifted: $anchor"
done
p3o6b2_recipe="$(make_target_recipe 'p3-o6b2-manual-retry-acceptance:')" || fail "P3-O6B2 acceptance target must be unique"
[[ "$p3o6b2_recipe" = *'P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL is required'* && "$p3o6b2_recipe" = *'o6b2_migration_compatibility.sh'* && "$p3o6b2_recipe" = *'TestManualRetry'* ]] ||
  fail "P3-O6B2 acceptance target lost retry, typed batch, rollback, or migration coverage"
grep -Fq 'P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p3-o6b2-manual-retry-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P3-O6B2 PG/River acceptance is disconnected from application CI"

p4d01_migration="$(git show :migrations/00025_automation_tag_trigger.sql)"
for anchor in \
  'CREATE TABLE IF NOT EXISTS event_deliveries' \
  'PRIMARY KEY (event_id, consumer)' \
  "status IN ('pending', 'processing', 'completed', 'final_failed', 'outcome_unknown')" \
  'CREATE TABLE IF NOT EXISTS automation_trigger_receipts' \
  "consumer = 'automation.tag-trigger.v1'" \
  'UNIQUE (event_id, consumer)' \
  'automation_trigger_receipts_list_idx' \
  '-- +goose Down' \
  'SELECT 1;'; do
  grep -Fq -- "$anchor" <<<"$p4d01_migration" || fail "P4-W0-D01 migration drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE IF NOT EXISTS ' <<<"$p4d01_migration")" -eq 2 ]] ||
  fail "P4-W0-D01 migration must create exactly delivery and Automation receipt tables"
! grep -Eiq 'DROP[[:space:]]+(TABLE|COLUMN|CONSTRAINT|INDEX)|REFERENCES[[:space:]]+(public[.])?river_job|automation_execution_trace' <<<"$p4d01_migration" ||
  fail "P4-W0-D01 migration gained destructive rollback, River FK, or retired trace semantics"

p4d01_event_queries="$(git show :internal/events/store/queries/event_log.sql)"
for anchor in \
  '-- name: ClaimEventsMissingDelivery :many' \
  'delivery.consumer = sqlc.arg(consumer)' \
  '-- name: ClaimEventDelivery :one' \
  'delivery.lease_expires_at <= sqlc.arg(claimed_at)' \
  '-- name: OutcomeUnknownEventDelivery :execrows'; do
  grep -Fq -- "$anchor" <<<"$p4d01_event_queries" || fail "P4-W0-D01 Events SQLC drifted: $anchor"
done
! grep -Eiq '(INSERT[[:space:]]+INTO|UPDATE|DELETE[[:space:]]+FROM)[[:space:]]+automation_trigger_receipts' <<<"$p4d01_event_queries" ||
  fail "P4-W0-D01 Events query crossed into Automation-owned receipts"

p4d01_automation="$(git show :internal/automation/store/tag_trigger.go)"
for anchor in \
  'ReserveAutomationTriggerReceipt' \
  'EvAutomationTriggered' \
  'CompleteAutomationTriggerReceipt' \
  'consumer.deliveries.Complete' \
  'eventport.PoisonDelivery'; do
  grep -Fq -- "$anchor" <<<"$p4d01_automation" || fail "P4-W0-D01 Automation consumer drifted: $anchor"
done
! grep -Eiq 'event_deliveries|river_job|automation_execution_trace|internal/events/store' <<<"$p4d01_automation" ||
  fail "P4-W0-D01 Automation consumer bypassed its public Events port or revived retired trace semantics"

p4d01_acceptance="$(git show :acceptance/automation/d01_integration_test.go)"
for anchor in \
  'TestD01ContactProducerAndAutomationConsumerCloseOneObservableLoop' \
  'TestD01RetryPoisonOutcomeUnknownLeaseAndBackfill' \
  'TestD01ProducerRollsBackAllFiveFactsWhenAcceptanceFails' \
  'TestD01S200KReceiptPlanHasNoIllegalSequentialScan' \
  'TestD01StorageCatalogIsValidatedAndHasNoRiverOrAutomationCrossDomainFK' \
  'historical dispatched'; do
  grep -Fq -- "$anchor" <<<"$p4d01_acceptance" || fail "P4-W0-D01 acceptance drifted: $anchor"
done

p4d01_compat="$(git show :acceptance/automation/d01_migration_compatibility.sh)"
for anchor in \
  'down-to 24' \
  '[[ "$rollback_waterline" = "24" && "$history_events" = "1" ]]' \
  '[[ "$final_waterline" = "35" && "$history_events" = "1" ]]' \
  'P4-W0-D01 migration compatibility: PASS (24/32/24/32, D01, L01, and current history preserved)'; do
  grep -Fq -- "$anchor" <<<"$p4d01_compat" || fail "P4-W0-D01 historical migration acceptance drifted: $anchor"
done

p4d01_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  '/api/admin/automation-conversion/agent-runs:' \
  'operationId: listAutomationTriggerRuns' \
  'x-legacy-mapping-ids: [LEGACY-API-0141]' \
  'x-aicrm-capability: config.overview.read' \
  'visibility: { type: string, enum: [masked] }'; do
  grep -Fq -- "$anchor" <<<"$p4d01_openapi" || fail "P4-W0-D01 OpenAPI drifted: $anchor"
done

p4d01_api_mapping="$(git show :docs/api-mapping.jsonl | sed -n '141p')"
[[ "$p4d01_api_mapping" = *'"mapping_id":"LEGACY-API-0141"'* &&
   "$p4d01_api_mapping" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* &&
   "$p4d01_api_mapping" = *'"disposition":"DEFERRED_POST_LAUNCH"'* ]] ||
  fail "P4-W0-D01 must preserve the frozen P1 Tier-B route authority"

p4d01_migration_mapping="$(git show :docs/migration-mapping.jsonl | sed -n '62p')"
[[ "$p4d01_migration_mapping" = *'"legacy_table":"automation_execution_trace"'* &&
   "$p4d01_migration_mapping" = *'"decision":"DEFER"'* &&
   "$p4d01_migration_mapping" = *'"implementation":"NOT_STARTED"'* &&
   "$p4d01_migration_mapping" = *'does not revive, copy, or target automation_execution_trace'* ]] ||
  fail "P4-W0-D01 retired automation trace mapping drifted"
[[ "$(git show :docs/feature-matrix.csv | tail -n +2 | wc -l | tr -d ' ')" = "293" ]] ||
  fail "P4-W0-D01 must preserve the exact 293-row feature matrix"
! git show :docs/feature-matrix.csv | grep -Eq 'agent-runs|automation_trigger_receipts' ||
  fail "P4-W0-D01 falsely added the API-only receipt to the 293 UI feature matrix"

p4d01_card="$(git show :docs/execution/slices/P4-W0-D01.md)"
for anchor in \
  '15 个手写/治理业务合同文件' \
  'slice_induced=7' \
  'SCOPE_FROZEN_REPAIR_ONLY' \
  '293 功能矩阵精确审计后没有' \
  'PRODUCTION_DATABASE_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4d01_card" || fail "P4-W0-D01 frozen slice card drifted: $anchor"
done
p4d01_recipe="$(make_target_recipe 'p4-w0-d01-automation-acceptance:')" ||
  fail "P4-W0-D01 acceptance target must be unique"
[[ "$p4d01_recipe" = *'P4W0D01_AUTOMATION_TEST_DATABASE_URL is required'* &&
   "$p4d01_recipe" = *'d01_migration_compatibility.sh'* &&
   "$p4d01_recipe" = *'./acceptance/automation'* ]] ||
  fail "P4-W0-D01 acceptance target lost migration, River, or business coverage"
grep -Fq 'P4W0D01_AUTOMATION_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-w0-d01-automation-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-W0-D01 acceptance is disconnected from application CI"
grep -Fq 'automation_trigger_receipts: unique_tag_trigger_receipt_per_source_event' <(git show :docs/architecture/table-ownership.yml) ||
  fail "P4-W0-D01 Automation receipt lost ownership"
grep -Fq 'event_deliveries: per_consumer_delivery_state_and_lease' <(git show :docs/architecture/table-ownership.yml) ||
  fail "P4-W0-D01 delivery state lost Events ownership"

p4l01_migration="$(git show :migrations/00026_stats_tag_applied.sql)"
for anchor in \
  'CREATE TABLE IF NOT EXISTS stats_daily' \
  'PRIMARY KEY (stat_date, metric_key, dims)' \
  'CREATE TABLE IF NOT EXISTS stats_event_receipts' \
  "consumer = 'stats.tag-applied.v1'" \
  "metric_key = 'customer.tag_applied'" \
  'PRIMARY KEY (event_id, consumer)' \
  'stats_daily_metric_date_idx' \
  '-- +goose Down' \
  'SELECT 1;'; do
  grep -Fq -- "$anchor" <<<"$p4l01_migration" || fail "P4-W0-L01 migration drifted: $anchor"
done
[[ "$(grep -Ec '^CREATE TABLE IF NOT EXISTS ' <<<"$p4l01_migration")" -eq 2 ]] ||
  fail "P4-W0-L01 migration must create exactly Stats projection and receipt tables"
! grep -Eiq 'DROP[[:space:]]+(TABLE|COLUMN|CONSTRAINT|INDEX)|REFERENCES[[:space:]]|river_job|event_deliveries|automation_trigger_receipts' <<<"$p4l01_migration" ||
  fail "P4-W0-L01 migration gained destructive rollback or a cross-domain dependency"

p4l01_queries="$(git show :internal/stats/store/queries/tag_applied.sql)"
for anchor in \
  '-- name: ReserveStatsEventReceipt :one' \
  'ON CONFLICT (event_id, consumer) DO NOTHING' \
  '-- name: GetMatchingStatsEventReceipt :one' \
  '-- name: IncrementStatsDaily :exec' \
  'SET value = stats_daily.value + EXCLUDED.value' \
  '-- name: GetStatsDaily :one'; do
  grep -Fq -- "$anchor" <<<"$p4l01_queries" || fail "P4-W0-L01 Stats SQLC drifted: $anchor"
done
! grep -Eiq '(INSERT[[:space:]]+INTO|UPDATE|DELETE[[:space:]]+FROM)[[:space:]]+(event_log|event_deliveries|automation_trigger_receipts|customers|customer_events)' <<<"$p4l01_queries" ||
  fail "P4-W0-L01 Stats SQLC crossed a domain ownership boundary"

p4l01_consumer="$(git show :internal/stats/store/tag_applied.go)"
for anchor in \
  'ConsumerStatsTagApplied' \
  'ReserveStatsEventReceipt' \
  'GetMatchingStatsEventReceipt' \
  'IncrementStatsDaily' \
  'consumer.deliveries.Complete' \
  'eventport.PoisonDelivery'; do
  grep -Fq -- "$anchor" <<<"$p4l01_consumer" || fail "P4-W0-L01 Stats consumer drifted: $anchor"
done
! grep -Eiq 'event_deliveries|river_job|automation_trigger_receipts|internal/(events|automation|contact)/store' <<<"$p4l01_consumer" ||
  fail "P4-W0-L01 Stats consumer bypassed public ports or crossed store ownership"

p4l01_contact="$(git show :internal/contact/app/customer_mutation_service.go)"
for anchor in 'ConsumerAutomationTagTrigger' 'ConsumerStatsTagApplied'; do
  grep -Fq -- "$anchor" <<<"$p4l01_contact" || fail "P4-W0-L01 producer lost named acceptance: $anchor"
done

p4l01_acceptance="$(git show :acceptance/stats/l01_integration_test.go)"
for anchor in \
  'TestL01SameEventCompletesAutomationAndStatsIndependently' \
  'TestL01StatsRetryPoisonOutcomeUnknownAndLeaseDoNotAffectAutomation' \
  'TestL01HistoricalMissingStatsDeliveryBackfillsOnce' \
  'TestL01ProducerRollsBackBothNamedAcceptances' \
  'TestL01S200KPlansUseStatsIndexes' \
  'TestL01StorageCatalogHasValidatedStatsOwnedTables'; do
  grep -Fq -- "$anchor" <<<"$p4l01_acceptance" || fail "P4-W0-L01 acceptance drifted: $anchor"
done

p4l01_compat="$(git show :acceptance/stats/l01_migration_compatibility.sh)"
for anchor in \
  'up-to 25' \
  'down-to 25' \
  '[[ "$rollback_waterline" = "25"' \
  '[[ "$final_waterline" = "35"' \
  'P4-W0-L01 migration compatibility: PASS (25/32/25/32, history preserved through current waterline)'; do
  grep -Fq -- "$anchor" <<<"$p4l01_compat" || fail "P4-W0-L01 historical migration acceptance drifted: $anchor"
done

p4l01_card="$(git show :docs/execution/slices/P4-W0-L01.md)"
for anchor in \
  '12 个手写/治理文件' \
  'stats_daily' \
  'L03 才交付 dashboard compatibility APIs' \
  '不猜 DTO' \
  'PRODUCTION_DATABASE_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4l01_card" || fail "P4-W0-L01 frozen slice card drifted: $anchor"
done
p4l01_recipe="$(make_target_recipe 'p4-w0-l01-stats-acceptance:')" ||
  fail "P4-W0-L01 acceptance target must be unique"
[[ "$p4l01_recipe" = *'P4W0L01_STATS_TEST_DATABASE_URL is required'* &&
   "$p4l01_recipe" = *'l01_migration_compatibility.sh'* &&
   "$p4l01_recipe" = *'./acceptance/stats'* ]] ||
  fail "P4-W0-L01 acceptance target lost migration, River, or Stats business coverage"
grep -Fq 'P4W0L01_STATS_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-w0-l01-stats-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-W0-L01 acceptance is disconnected from application CI"
grep -Fq 'stats_event_receipts: unique_projection_receipt_per_source_event' <(git show :docs/architecture/table-ownership.yml) ||
  fail "P4-W0-L01 receipt lost Stats ownership"
grep -Fq 'ConsumerStatsTagApplied      = "stats.tag-applied.v1"' <(git show :internal/events/port/port.go) ||
  fail "P4-W0-L01 named consumer identifier drifted"
[[ "$(git show :docs/feature-matrix.csv | tail -n +2 | wc -l | tr -d ' ')" = "293" ]] ||
  fail "P4-W0-L01 must preserve the exact 293-row feature matrix"
p4l01_feature="$(git show :docs/feature-matrix.csv | grep '^"LEGACY-S07-015"')"
[[ "$p4l01_feature" = *'"MIGRATE","NOT_STARTED","NOT_RUN"'* ]] ||
  fail "P4-W0-L01 falsely closed the future HXC dashboard UI flow"
p4l01_api_mapping="$(git show :docs/api-mapping.jsonl | sed -n '340p')"
[[ "$p4l01_api_mapping" = *'"mapping_id":"LEGACY-API-0340"'* &&
   "$p4l01_api_mapping" = *'"disposition":"DEFERRED_POST_LAUNCH"'* ]] ||
  fail "P4-W0-L01 falsely claimed the deferred dashboard compatibility API"
p4l01_migration_mapping="$(git show :docs/migration-mapping.jsonl | sed -n '285p')"
[[ "$p4l01_migration_mapping" = *'"legacy_table":"user_ops_hxc_dashboard_meta"'* &&
   "$p4l01_migration_mapping" = *'"decision":"REBUILD"'* &&
   "$p4l01_migration_mapping" = *'"implementation":"NOT_STARTED"'* ]] ||
  fail "P4-W0-L01 falsely claimed a legacy dashboard snapshot migration"

p4a01_migration="$(git show :migrations/00027_auth_oauth_states.sql)"
for anchor in \
  'CREATE TABLE admin_oauth_states' \
  'state_hash          BYTEA PRIMARY KEY' \
  "auth_provider = 'wecom'" \
  'idx_admin_oauth_states_expiry' \
  '-- +goose Down' \
  'DROP TABLE admin_oauth_states;'; do
  grep -Fq -- "$anchor" <<<"$p4a01_migration" || fail "P4-A01 OAuth state migration drifted: $anchor"
done
! grep -Eiq 'tenant|organization|workspace|REFERENCES[[:space:]]|river_job' <<<"$p4a01_migration" ||
  fail "P4-A01 OAuth state migration gained tenant or cross-domain semantics"

p4a01_queries="$(git show :internal/auth/store/queries/auth.sql)"
for anchor in \
  '-- name: InsertAdminOAuthState :exec' \
  '-- name: ClaimAdminOAuthState :one' \
  'DELETE FROM admin_oauth_states' \
  'state_hash = sqlc.arg(state_hash)' \
  'expires_at <= sqlc.arg(created_at)' \
  'expires_at > sqlc.arg(claimed_at)' \
  'RETURNING next_path;'; do
  grep -Fq -- "$anchor" <<<"$p4a01_queries" || fail "P4-A01 OAuth state SQLC drifted: $anchor"
done

p4a01_handler="$(git show :cmd/aicrm/legacy_auth.go)"
for anchor in \
  'CorpID() string' \
  'handler.states.Claim' \
  'handler.issuer.IssueVerified' \
  'writeHumanBrowserSession' \
  'LegacySessionCookieName' \
  'LegacyCSRFCookieName' \
  'CapabilityAuthSessionLogout' \
  'handler.auth.Invalidate' \
  'strings.ContainsAny(value, "\\#")' \
  'url.PathUnescape(value)'; do
  grep -Fq -- "$anchor" <<<"$p4a01_handler" || fail "P4-A01 human auth handler drifted: $anchor"
done
! grep -Fq 'TenantID() string' <<<"$p4a01_handler" || fail "P4-A01 introduced a tenant-named provider interface"
grep -Fq '"auth/getuserinfo": true' <(git show :scripts/ownership/main.go) ||
  fail "P4-A01 enterprise human identity read lost WeCom ownership classification"
grep -Fq 'lower == "https://qyapi.weixin.qq.com" && source == "wecom"' <(git show :scripts/ownership/main.go) ||
  fail "P4-A01 WeCom-owned production base URL classification drifted"

p4a01_acceptance="$(git show :cmd/aicrm/legacy_auth_integration_test.go)"
for anchor in \
  'TestA01HumanOAuthSessionRBACCSRFFullChainOnPostgreSQL' \
  "serverVersion != 160014" \
  'badLogout.Code != http.StatusForbidden' \
  'revokedResponse.Code != http.StatusUnauthorized' \
  'tokenCalls.Load() != 1' \
  'TestA01S200KOAuthClaimPlanHasNoIllegalSequentialScan' \
  'Seq Scan on admin_oauth_states' \
  'Index Scan using admin_oauth_states_pkey' \
  'Index Scan using idx_admin_oauth_states_expiry'; do
  grep -Fq -- "$anchor" <<<"$p4a01_acceptance" || fail "P4-A01 PG acceptance drifted: $anchor"
done

p4a01_redirect_tests="$(git show :cmd/aicrm/legacy_auth_test.go)"
for anchor in \
  '/\evil.example/path' \
  '/%5cevil.example/path' \
  '/%255cevil.example/path' \
  '/%2f%2fevil.example/path' \
  '/%252f%252fevil.example/path' \
  '/%0d%0aLocation:%20https://evil.example' \
  '/%2e%2e/evil' \
  '/admin#fragment'; do
  grep -Fq -- "$anchor" <<<"$p4a01_redirect_tests" || fail "P4-A01 permanent redirect negative drifted: $anchor"
done

p4a01_compat="$(git show :acceptance/auth/a01_migration_compatibility.sh)"
for anchor in \
  'down-to 26' \
  '[[ "$upgrade_waterline" = "27"' \
  '[[ "$rollback_waterline" = "26"' \
  '[[ "$final_waterline" = "27"' \
  'existing session history preserved; transient state reset'; do
  grep -Fq -- "$anchor" <<<"$p4a01_compat" || fail "P4-A01 migration compatibility drifted: $anchor"
done

p4a01_recipe="$(make_target_recipe 'p4-a01-auth-acceptance:')" ||
  fail "P4-A01 acceptance target must be unique"
[[ "$p4a01_recipe" = *'P4A01_AUTH_TEST_DATABASE_URL is required'* &&
   "$p4a01_recipe" = *'a01_migration_compatibility.sh'* &&
   "$p4a01_recipe" = *"-run '^TestA01' ./cmd/aicrm -args -p4-a01-database-url"* ]] ||
  fail "P4-A01 acceptance target lost migration or human auth coverage"
grep -Fq 'P4A01_AUTH_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-a01-auth-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-A01 acceptance is disconnected from application CI"
grep -Fq 'admin_oauth_states: hashed_one_time_nonce_and_safe_internal_next_only' <(git show :docs/architecture/table-ownership.yml) ||
  fail "P4-A01 OAuth state lost Auth ownership"

for mapping_id in LEGACY-API-0752 LEGACY-API-0753 LEGACY-API-0758 LEGACY-API-0759 LEGACY-API-0760 LEGACY-API-0761; do
  p4a01_route="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$p4a01_route" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* &&
     "$p4a01_route" = *'"disposition":"MIGRATE"'* ]] ||
    fail "P4-A01 forged or lost frozen route authority for $mapping_id"
done
for feature_id in LEGACY-S05-036 LEGACY-S05-037 LEGACY-S06-043; do
  p4a01_feature="$(git show :docs/feature-matrix.csv | grep -F "\"$feature_id\"")"
  [[ "$p4a01_feature" = *'"MIGRATE","IN_PROGRESS","NOT_RUN"'* && "$p4a01_feature" = *'P4-A01'* ]] ||
    fail "P4-A01 feature reconciliation drifted for $feature_id"
done
p4a01_migration_mapping="$(git show :docs/migration-mapping.jsonl | grep -F '"mapping_id":"LEGACY-T14-003"')"
[[ "$p4a01_migration_mapping" = *'"legacy_table":"admin_sso_states"'* &&
   "$p4a01_migration_mapping" = *'"decision":"DEFER"'* &&
   "$p4a01_migration_mapping" = *'"implementation":"NOT_STARTED"'* &&
   "$p4a01_migration_mapping" = *'fresh reset-runtime admin_oauth_states nonce rows'* ]] ||
  fail "P4-A01 legacy state migration disposition drifted"

p4a01_card="$(git show :docs/execution/slices/P4-A01.md)"
for anchor in \
  '单实例私有化' \
  'LEGACY-API-0752' \
  'LEGACY-S05-036' \
  'LEGACY-T14-003' \
  'slice_induced=3' \
  '未建设：tenant 产品能力' \
  'REAL_WECOM_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4a01_card" || fail "P4-A01 slice card drifted: $anchor"
done

p4si00b_migration="$(git show :migrations/00028_auth_wecom_corp_id.sql)"
for anchor in \
  'RENAME COLUMN provider_tenant_id TO wecom_corp_id;' \
  'RENAME CONSTRAINT ck_admin_users_provider_tenant TO ck_admin_users_wecom_corp_id;' \
  'RENAME CONSTRAINT uq_admin_users_provider_identity TO uq_admin_users_wecom_identity;' \
  '-- +goose Down' \
  'RENAME CONSTRAINT uq_admin_users_wecom_identity TO uq_admin_users_provider_identity;' \
  'RENAME COLUMN wecom_corp_id TO provider_tenant_id;'; do
  grep -Fq -- "$anchor" <<<"$p4si00b_migration" || fail "P4-SI00B forward rename migration drifted: $anchor"
done
[[ "$(grep -Ec '^ALTER TABLE admin_users$' <<<"$p4si00b_migration")" = "6" ]] ||
  fail "P4-SI00B migration must only rename Auth-owned admin_users identifiers"
! grep -Eiq 'ADD[[:space:]]+COLUMN|DROP[[:space:]]+COLUMN|DELETE[[:space:]]+FROM|TRUNCATE|UPDATE[[:space:]]+admin_users|INSERT[[:space:]]+INTO' <<<"$p4si00b_migration" ||
  fail "P4-SI00B migration gained data rewrite or destructive DDL"

p4si00b_port="$(git show :internal/auth/port/port.go)"
grep -Fq 'CorpID    string' <<<"$p4si00b_port" || fail "P4-SI00B VerifiedLogin lost CorpID"
! grep -Fq 'TenantID  string' <<<"$p4si00b_port" || fail "P4-SI00B VerifiedLogin restored TenantID"
p4si00b_queries="$(git show :internal/auth/store/queries/auth.sql)"
grep -Fq 'wecom_corp_id = sqlc.arg(wecom_corp_id)' <<<"$p4si00b_queries" ||
  fail "P4-SI00B Auth query lost WeCom CorpID account lookup"
! grep -Fq 'provider_tenant_id' <<<"$p4si00b_queries" || fail "P4-SI00B Auth query restored the legacy column name"
p4si00b_generated="$(git show :internal/auth/store/generated/auth.sql.go)"
for anchor in 'AND wecom_corp_id = $2' 'WecomCorpID       string `json:"wecom_corp_id"`' 'arg.WecomCorpID'; do
  grep -Fq -- "$anchor" <<<"$p4si00b_generated" || fail "P4-SI00B SQLC output drifted: $anchor"
done
p4si00b_repository="$(git show :internal/auth/store/repository.go)"
grep -Fq 'WecomCorpID: login.CorpID' <<<"$p4si00b_repository" ||
  fail "P4-SI00B repository no longer propagates provider CorpID"
grep -Fq 'CorpID() string' <<<"$p4a01_handler" || fail "P4-SI00B removed the A01 CorpID provider interface"

p4si00b_compat="$(git show :acceptance/auth/si00b_migration_compatibility.sh)"
for anchor in \
  'down-to 27' \
  '[[ "$upgrade_waterline" = "35"' \
  '[[ "$rollback_waterline" = "27"' \
  '[[ "$final_waterline" = "35"' \
  'ck_admin_users_wecom_corp_id' \
  'uq_admin_users_wecom_identity' \
  'count(DISTINCT wecom_corp_id)' \
  'CorpID accounts and existing session preserved'; do
  grep -Fq -- "$anchor" <<<"$p4si00b_compat" || fail "P4-SI00B migration compatibility drifted: $anchor"
done
p4si00b_recipe="$(make_target_recipe 'p4-si00b-auth-acceptance:')" ||
  fail "P4-SI00B acceptance target must be unique"
[[ "$p4si00b_recipe" = *'P4SI00B_AUTH_TEST_DATABASE_URL is required'* &&
   "$p4si00b_recipe" = *'si00b_migration_compatibility.sh'* &&
   "$p4si00b_recipe" = *'./internal/auth/app ./internal/auth/store'* &&
   "$p4si00b_recipe" = *'./acceptance/p2s09 ./acceptance/p2s16'* &&
   "$p4si00b_recipe" = *'TestA01|TestHumanOAuth|TestHumanAuth|TestFinalRouter'* ]] ||
  fail "P4-SI00B acceptance target lost the directly affected Auth chain"
grep -Fq 'P4SI00B_AUTH_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-si00b-auth-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-SI00B acceptance is disconnected from application CI"
grep -Fq 'goose -dir migrations postgres "$$P4A01_AUTH_TEST_DATABASE_URL" up' <<<"$p4a01_recipe" ||
  fail "P4-A01 acceptance does not return to current waterline after its frozen 26/27/26/27 proof"

p4si00b_card="$(git show :docs/execution/slices/P4-SI00B.md)"
for anchor in \
  'WeCom provider identity namespace' \
  '27→28→27→28' \
  'A01 provider interface 继续提供 `CorpID()`' \
  'slice_induced=0' \
  '未建设 tenant model/selector/switch/RBAC/column/compound index/cross-tenant test' \
  'LIVE_MIGRATION_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4si00b_card" || fail "P4-SI00B slice card drifted: $anchor"
done
for ledger_anchor in \
  '  - slice_id: P4-SI00B' \
  '    deployment_model: single_instance_single_enterprise_single_database_private_deployment' \
  '    slice_induced_correction_count: 0' \
  '    infra_induced_correction_count: 1' \
  '    verification_induced_correction_count: 5'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-SI00B ledger drifted: $ledger_anchor"
done

verify_index_sha256 migrations/00030_media_image_upload.sql \
  cae3c3beed5770fcc5d8476ea1cbf474c2fc1c5005f3d271976bd30522ac8b8a
verify_index_sha256 acceptance/media/doc.go \
  aa564242cdec34c035edd60e18f0708cce1b2eb3048774c98aad2a40d6973332
verify_index_sha256 acceptance/media/h01a1_integration_test.go \
  659bad19c862a668e48e0b1c0319ea99d5819870336b3267ccbabcdcae4a1bd3
verify_index_sha256 acceptance/media/h01a1_migration_compatibility.sh \
  4bfc1a45d6422169d25da95fe92fd2d01c8ce5cb09d30c3ee9b50943512c9ac8
verify_index_sha256 docs/execution/slices/P4-H01A1.md \
  7e7487a1b45e03f287c2dbeee6cc06822e4744412ac0255f63b78c4321e2a275
verify_index_sha256 internal/media/app/upload.go \
  8cdd6e22c5f752f39308cafac653cfc94f71f48e23c72e9c82f933b0e3d7372a
verify_index_sha256 internal/media/app/upload_test.go \
  72047cc4d1f7b857096fe3b92b788a88f8a08c58011e6434ddce2c0aadb1e145
verify_index_sha256 internal/media/domain/image.go \
  75c14b85a517431fea88860207c554782ca02f9ee120fdf7c290920544c0f7b4
verify_index_sha256 internal/media/domain/image_test.go \
  a5b41746ed8b4e3c33642bf379df1f2f2473cb893c32b77eeedc9c2151d1c524
verify_index_sha256 internal/media/port/port.go \
  7119786d2ca1ec60026a647e99a9aeb0e6784382803069fce6af66597abb238d
verify_index_sha256 internal/media/store/upload_repository.go \
  3a1ad2559f554c48be787e493a79e511f5a9fbc0fdc9253c39f63c73bfad1f62
verify_index_sha256 internal/media/store/queries/upload.sql \
  ac85c218b6df5babfcacb8a6d3c6a8be55f77a2fb3642f6d2340e52b5eb7dd90
verify_index_sha256 internal/media/store/generated/upload.sql.go \
  d3446cadeeccaaccfd21e5745f321013d415a5fb0019632b20e1dca0d034a345
verify_index_sha256 internal/media/store/generated/db.go \
  42aea8515b0a629ea2501a95f8acaf7e133d3aec3e5fea783c49690a589d5a37
verify_index_sha256 internal/media/store/generated/models.go \
  16b31e4a190c0ea7dde25da64e8ac81596e4420de335ca7a5d01b8286ec4143c
verify_index_sha256 internal/media/store/generated/querier.go \
  a6ff0873f4f55779064fea54215980573d618363ac85f18057576a0cb48651de
verify_index_sha256 cmd/aicrm/legacy_media_api_test.go \
  def7cfe2af50785d599051c5e3e5dc4eaebbb4ff3448c0d0a9dd8e7469e83c84

p4i01a_migration="$(git show :migrations/00029_product_catalog.sql)"
for anchor in \
  'CREATE TABLE public.products (' \
  'product_code TEXT NOT NULL UNIQUE' \
  'legacy_admin_projection JSONB NOT NULL' \
  'CREATE TABLE public.product_images (' \
  'CREATE TABLE public.product_catalog_counters (' \
  'CREATE TABLE public.product_operation_receipts (' \
  'product operation receipt must complete in its reservation transaction' \
  '-- +goose Down' \
  'DROP TABLE public.product_operation_receipts;' \
  'DROP TABLE public.products;'; do
  grep -Fq -- "$anchor" <<<"$p4i01a_migration" || fail "P4-I01A Product migration drifted: $anchor"
done
! grep -Eiq 'tenant|payment|refund|order|media' <<<"$p4i01a_migration" ||
  fail "P4-I01A Product migration gained excluded product semantics"

p4i01a_app="$(git show :internal/product/app/catalog.go)"
for anchor in \
  'func (s *Service) ListLegacy' \
  'func (s *Service) Create' \
  'CanonicalLegacyAdminProjection' \
  'subtle.ConstantTimeCompare(receipt.PayloadDigest[:], digest[:])' \
  'jsonEquivalent(canonical, receipt.ResultSnapshot)' \
  'eventport.EvProductCreated' \
  's.events.Append(tx'; do
  grep -Fq -- "$anchor" <<<"$p4i01a_app" || fail "P4-I01A Product application drifted: $anchor"
done
! grep -Eiq 'TenantID|tenant_id|payment_request|refund|Media' <<<"$p4i01a_app" ||
  fail "P4-I01A Product application gained excluded tenant or external-effect semantics"

p4i01a_queries="$(git show :internal/product/store/queries/catalog.sql)"
for anchor in \
  '-- name: ListProductsOffset :many' \
  '-- name: CountProducts :one' \
  'SELECT total_products FROM product_catalog_counters WHERE singleton = TRUE;' \
  '-- name: IncrementProductCount :one' \
  '-- name: ReserveProductOperationReceipt :one' \
  '-- name: CompleteProductOperationReceipt :one'; do
  grep -Fq -- "$anchor" <<<"$p4i01a_queries" || fail "P4-I01A Product SQLC query drifted: $anchor"
done

p4i01a_legacy="$(git show :cmd/aicrm/legacy_api.go)"
for anchor in \
  'func (handler *Handler) ListProducts' \
  'func (handler *Handler) GetProduct' \
  'func (handler *Handler) CreateProduct' \
  'StockQuantity: 0' \
  'values["fallback_used"] = false' \
  'values["real_external_call_executed"] = false' \
  'values["payment_request_executed"] = false' \
  'values["real_refund_executed"] = false' \
  'platformhttp.WriteError'; do
  grep -Fq -- "$anchor" <<<"$p4i01a_legacy" || fail "P4-I01A legacy adapter drifted: $anchor"
done
p4i01a_legacy_test="$(git show :cmd/aicrm/legacy_product_api_test.go)"
for anchor in \
  '/api/admin/wechat-pay/products?limit=5&offset=10' \
  '"real_wechat_pay_executed"' \
  '"provider_signature_verified"' \
  '"request_id"' \
  'platform error leaked legacy success field'; do
  grep -Fq -- "$anchor" <<<"$p4i01a_legacy_test" || fail "P4-I01A legacy blackbox drifted: $anchor"
done

p4i01a_compat="$(git show :acceptance/product/i01a_migration_compatibility.sh)"
for anchor in \
  'down-to 28' \
  'up-to 29' \
  '[[ "$upgrade_waterline" = "29"' \
  '[[ "$rollback_waterline" = "28"' \
  '[[ "$final_waterline" = "29"' \
  'baseline_admin_hash' \
  'baseline_session_hash' \
  'pre-existing Event/Auth/session history preserved'; do
  grep -Fq -- "$anchor" <<<"$p4i01a_compat" || fail "P4-I01A migration compatibility drifted: $anchor"
done
p4i01a_recipe="$(make_target_recipe 'p4-i01a-product-acceptance:')" ||
  fail "P4-I01A acceptance target must be unique"
[[ "$p4i01a_recipe" = *'P4I01A_PRODUCT_TEST_DATABASE_URL is required'* &&
   "$p4i01a_recipe" = *'i01a_migration_compatibility.sh'* &&
   "$p4i01a_recipe" = *'./internal/product/... ./cmd/aicrm'* ]] ||
  fail "P4-I01A acceptance target lost the directly affected Product chain"
grep -Fq 'P4I01A_PRODUCT_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-i01a-product-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-I01A acceptance is disconnected from application CI"

for ownership_anchor in \
  'tables: [products, product_images, product_catalog_counters, product_operation_receipts]' \
  'product_catalog_counters: exact_legacy_total_without_catalog_scan'; do
  grep -Fq -- "$ownership_anchor" <(git show :docs/architecture/table-ownership.yml) ||
    fail "P4-I01A Product ownership drifted: $ownership_anchor"
done
for mapping_id in LEGACY-API-0525 LEGACY-API-0526 LEGACY-API-0530; do
  p4i01a_route="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$p4i01a_route" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* &&
     "$p4i01a_route" = *'"disposition":"MIGRATE"'* ]] ||
    fail "P4-I01A forged or lost frozen route authority for $mapping_id"
done
p4i01a_migration_mapping="$(git show :docs/migration-mapping.jsonl | grep -F '"mapping_id":"LEGACY-T14-302"')"
[[ "$p4i01a_migration_mapping" = *'"decision":"ARCHIVE_ONLY"'* &&
   "$p4i01a_migration_mapping" = *'"implementation":"NOT_STARTED"'* &&
   "$p4i01a_migration_mapping" = *'"verification":"NOT_RUN"'* ]] ||
  fail "P4-I01A incorrectly closed the frozen historical Product migration"

p4i01a_card="$(git show :docs/execution/slices/P4-I01A.md)"
for anchor in \
  'LEGACY-API-0525/0526/0530' \
  'LEGACY-S07-164/165' \
  'LEGACY-T14-302' \
  'stock_quantity=0' \
  'fallback_used=false' \
  '28→29→28→29' \
  'slice_induced=10' \
  'verification_induced=18' \
  'https://github.com/qianlan33333-png/AI-CRM-v2/pull/208' \
  'REAL_PAYMENT_REFUND_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4i01a_card" || fail "P4-I01A slice card drifted: $anchor"
done
for evidence_anchor in \
  'TestI01AFourFailurePointsRollbackAllProductFacts' \
  'TestI01AConcurrentReplayAndProductCodeConflict' \
  'EXPLAIN (FORMAT JSON, COSTS OFF)'; do
  grep -Fq -- "$evidence_anchor" <(git show :acceptance/product/i01a_integration_test.go) ||
    fail "P4-I01A focused PG evidence drifted: $evidence_anchor"
done
for ledger_anchor in \
  '  - slice_id: P4-I01A' \
  '    status: SCOPE_FROZEN_REPAIR_ONLY_PR_REQUIRED_CI_PENDING' \
  '    pr_url: https://github.com/qianlan33333-png/AI-CRM-v2/pull/208' \
  '    slice_induced_correction_count: 10' \
  '    infra_induced_correction_count: 0' \
  '    verification_induced_correction_count: 18'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-I01A ledger drifted: $ledger_anchor"
done

for feature_id in LEGACY-S07-164 LEGACY-S07-165; do
  p4i01a_feature="$(git show :docs/feature-matrix.csv | grep -F "\"$feature_id\"")"
  [[ "$p4i01a_feature" = *'"MIGRATE","IMPLEMENTED","SYNTHETIC_PASS","APPROVED"'* &&
     "$p4i01a_feature" = *'https://github.com/qianlan33333-png/AI-CRM-v2/pull/208'* &&
     "$p4i01a_feature" = *'b068ae02b24856aec4283e7de19056e3a5581b44'* ]] ||
    fail "P4-I01A feature reconciliation drifted for $feature_id"
done

p4f01a_migration="$(git show :migrations/00031_survey_questionnaire_crud.sql)"
for anchor in \
  'CREATE TABLE public.questionnaires (' \
  'CREATE TABLE public.questionnaire_questions (' \
  'CREATE TABLE public.questionnaire_options (' \
  'CREATE TABLE public.questionnaire_catalog_counters (' \
  'CREATE TABLE public.questionnaire_operation_receipts (' \
  'DEFERRABLE INITIALLY DEFERRED' \
  'DROP TABLE public.questionnaire_operation_receipts;' \
  'DROP TABLE public.questionnaires;'; do
  grep -Fq -- "$anchor" <<<"$p4f01a_migration" || fail "P4-F01A migration drifted: $anchor"
done
! grep -Eiq 'tenant|workspace_id|organization_id|REFERENCES[[:space:]]+event_log' <<<"$p4f01a_migration" ||
  fail "P4-F01A migration gained tenant or cross-domain Event schema"

p4f01a_api="$(git show :cmd/aicrm/api.go)"
for anchor in \
  '"/api/admin/questionnaires"' \
  '{http.MethodGet, "/api/admin/questionnaires", authport.CapabilityQuestionnairesRead, false, http.HandlerFunc(legacy.ListQuestionnaires)}' \
  '{http.MethodPost, "/api/admin/questionnaires", authport.CapabilityQuestionnairesWrite, true, http.HandlerFunc(legacy.CreateQuestionnaire)}' \
  '"/api/admin/questionnaires/{questionnaire_id}"' \
  'authport.CapabilityQuestionnairesRead' \
  'authport.CapabilityQuestionnairesWrite'; do
  grep -Fq -- "$anchor" <<<"$p4f01a_api" || fail "P4-F01A route registration drifted: $anchor"
done
p4f01a_service="$(git show :internal/survey/app/service.go)"
for anchor in \
  'F02 assessment not yet available' \
  'eventport.EvSurveyCreated' \
  's.uow.Within' \
  'ActorScope'; do
  grep -Fq -- "$anchor" <<<"$p4f01a_service" || fail "P4-F01A app contract drifted: $anchor"
done

p4f01a_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  'operationId: listLegacyQuestionnaires' \
  'operationId: createLegacyQuestionnaire' \
  'operationId: getLegacyQuestionnaire' \
  'x-legacy-mapping-ids: [LEGACY-API-0423]' \
  'x-legacy-mapping-ids: [LEGACY-API-0424]' \
  'x-legacy-mapping-ids: [LEGACY-API-0427]' \
  'x-p4-decision-evidence: P4-F01A-2026-08-15' \
  'assessment_enabled: { type: boolean, enum: [false] }' \
  'score_rules: { type: array, maxItems: 0'; do
  grep -Fq -- "$anchor" <<<"$p4f01a_openapi" || fail "P4-F01A OpenAPI drifted: $anchor"
done

p4f01a_compat="$(git show :acceptance/survey/f01a_migration_compatibility.sh)"
for anchor in \
  'up-to 30' \
  '[[ "$waterline" = "31"' \
  'down-to 30' \
  '[[ "$rollback_waterline" = "30"' \
  'P4-F01A migration compatibility: PASS (30/31/30/31, Event/Auth/Product/Media history preserved)'; do
  grep -Fq -- "$anchor" <<<"$p4f01a_compat" || fail "P4-F01A migration compatibility drifted: $anchor"
done
p4f01a_recipe="$(make_target_recipe 'p4-f01a-survey-acceptance:')" ||
  fail "P4-F01A acceptance target must be unique"
[[ "$p4f01a_recipe" = *'P4F01A_SURVEY_TEST_DATABASE_URL is required'* &&
   "$p4f01a_recipe" = *'f01a_migration_compatibility.sh'* &&
   "$p4f01a_recipe" = *'./internal/survey/... ./internal/events/store ./internal/platform/http ./internal/auth/... ./cmd/aicrm'* ]] ||
  fail "P4-F01A acceptance target lost its directly affected Survey/Auth/Event chain"
grep -Fq 'P4F01A_SURVEY_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-f01a-survey-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-F01A acceptance is disconnected from application CI"

for ownership_anchor in \
  'tables: [questionnaires, questionnaire_questions, questionnaire_options, questionnaire_catalog_counters, questionnaire_operation_receipts]' \
  'questionnaire_catalog_counters: exact_legacy_total_without_questionnaire_scan'; do
  grep -Fq -- "$ownership_anchor" <(git show :docs/architecture/table-ownership.yml) ||
    fail "P4-F01A Survey ownership drifted: $ownership_anchor"
done
for mapping_id in LEGACY-API-0423 LEGACY-API-0424 LEGACY-API-0427; do
  p4f01a_route="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$p4f01a_route" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* &&
     "$p4f01a_route" = *'"disposition":"MIGRATE"'* ]] ||
    fail "P4-F01A forged or lost frozen route authority for $mapping_id"
done
for mapping_id in LEGACY-T14-237 LEGACY-T14-238 LEGACY-T14-243; do
  p4f01a_mapping="$(git show :docs/migration-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$p4f01a_mapping" = *'"implementation":"NOT_STARTED"'* &&
     "$p4f01a_mapping" = *'"verification":"NOT_RUN"'* ]] ||
    fail "P4-F01A forged historical import completion for $mapping_id"
done

p4f01a_card="$(git show :docs/execution/slices/P4-F01A.md)"
for anchor in \
  'LEGACY-API-0423/0424/0427' \
  'LEGACY-S07-116/117/123' \
  'LEGACY-T14-237/238/243' \
  'F02 assessment not yet available' \
  '30→31→30→31' \
  'slice_induced=3' \
  'verification_induced=10' \
  'https://github.com/qianlan33333-png/AI-CRM-v2/pull/210' \
  'REAL_PAYMENT_REFUND_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4f01a_card" || fail "P4-F01A slice card drifted: $anchor"
done
for ledger_anchor in \
  '  - slice_id: P4-F01A' \
  '    status: SCOPE_FROZEN_REPAIR_ONLY_PR_REQUIRED_CI_PENDING' \
  '    pr_url: https://github.com/qianlan33333-png/AI-CRM-v2/pull/210' \
  '    slice_induced_correction_count: 3' \
  '    infra_induced_correction_count: 1' \
  '    verification_induced_correction_count: 11'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-F01A ledger drifted: $ledger_anchor"
done
for feature_id in LEGACY-S07-116 LEGACY-S07-117 LEGACY-S07-123; do
  p4f01a_feature="$(git show :docs/feature-matrix.csv | grep -F "\"$feature_id\"")"
  [[ "$p4f01a_feature" = *'"MIGRATE","IMPLEMENTED","SYNTHETIC_PASS","APPROVED"'* &&
     "$p4f01a_feature" = *'https://github.com/qianlan33333-png/AI-CRM-v2/pull/210'* &&
     "$p4f01a_feature" = *'fd2fc48daa16e346e22a1ca1c058e1a76665162a'* ]] ||
    fail "P4-F01A feature reconciliation drifted for $feature_id"
done

p4c01_migration="$(git show :migrations/00032_contact_channel_catalog.sql)"
for anchor in \
  'ALTER TABLE public.channels' \
  'CREATE TABLE public.channel_operation_receipts' \
  'UNIQUE (operation, actor_scope, key_digest)' \
  'channel_operation_receipts_complete_before_commit' \
  'completed channel operation receipts are immutable' \
  'channels_status_updated_id_idx' \
  '-- +goose Down'; do
  grep -Fq -- "$anchor" <<<"$p4c01_migration" || fail "P4-C01 migration drifted: $anchor"
done
! grep -Eiq 'REFERENCES[[:space:]]+(public[.])?(event_log|river_job)|tenant|wecom_(corp|external)|provider_(user|tenant)' <<<"$p4c01_migration" ||
  fail "P4-C01 migration gained cross-domain, provider, or tenant schema"

p4c01_routes="$(git show :cmd/aicrm/api.go)"
for anchor in \
  '{http.MethodGet, "/api/admin/channels", authport.CapabilityChannelsRead, false, http.HandlerFunc(legacy.ListChannels)}' \
  '{http.MethodPost, "/api/admin/channels", authport.CapabilityChannelsWrite, true, http.HandlerFunc(legacy.CreateChannel)}' \
  '{http.MethodGet, "/api/admin/channels/{channel_id}", authport.CapabilityChannelsRead, false, http.HandlerFunc(legacy.GetChannel)}' \
  '{http.MethodPatch, "/api/admin/channels/{channel_id}", authport.CapabilityChannelsWrite, true, http.HandlerFunc(legacy.UpdateChannel)}'; do
  grep -Fq -- "$anchor" <<<"$p4c01_routes" || fail "P4-C01 route registration drifted: $anchor"
done

p4c01_service="$(git show :internal/contact/app/channel_catalog.go)"
for anchor in \
  'service.uow.Within(ctx' \
  'service.events.Append(tx' \
  'eventType := eventport.EvChannelCreated' \
  'eventType = eventport.EvChannelUpdated' \
  'ErrChannelConflict'; do
  grep -Fq -- "$anchor" <<<"$p4c01_service" || fail "P4-C01 app contract drifted: $anchor"
done
p4c01_query_plan="$(git show :tools/query-plan-gate/main.go)"
grep -Fq "INSERT INTO channels (name, code, config) SELECT 'channel-' || n, 'segment-plan-channel-' || n, jsonb_build_object('schema_version', 1)" <<<"$p4c01_query_plan" ||
  fail "P4-C01 query-plan direct-consumer fixture lost the channel projection"

p4c01_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  'operationId: listLegacyChannels' \
  'operationId: createLegacyChannel' \
  'operationId: getLegacyChannel' \
  'operationId: updateLegacyChannel' \
  'x-p4-decision-evidence: P4-C01-2026-08-15' \
  'real_external_call_executed: { type: boolean, enum: [false] }'; do
  grep -Fq -- "$anchor" <<<"$p4c01_openapi" || fail "P4-C01 OpenAPI drifted: $anchor"
done

p4c01_compat="$(git show :acceptance/contact/c01_migration_compatibility.sh)"
for anchor in \
  'down-to 31' \
  'P4-C01 migration compatibility: PASS (31/32/31/32, Contact channel/customer history preserved)'; do
  grep -Fq -- "$anchor" <<<"$p4c01_compat" || fail "P4-C01 migration compatibility drifted: $anchor"
done
p4c01_recipe="$(make_target_recipe 'p4-c01-channel-acceptance:')" ||
  fail "P4-C01 acceptance target must be unique"
[[ "$p4c01_recipe" = *'P4C01_CHANNEL_TEST_DATABASE_URL is required'* &&
   "$p4c01_recipe" = *'c01_migration_compatibility.sh'* &&
   "$p4c01_recipe" = *'./internal/contact/app ./internal/contact/store'* &&
   "$p4c01_recipe" = *'./internal/events/store ./internal/platform/http ./internal/auth/... ./cmd/aicrm'* ]] ||
  fail "P4-C01 acceptance target lost its directly affected Contact/Auth/Event chain"
grep -Fq 'P4C01_CHANNEL_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-c01-channel-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-C01 acceptance is disconnected from application CI"

p4c01_ownership="$(git show :docs/architecture/table-ownership.yml)"
for anchor in \
  'channels: single_instance_local_channel_resource_without_provider_truth' \
  'channel_operation_receipts: actor_scoped_transactional_create_update_idempotency_receipts'; do
  grep -Fq -- "$anchor" <<<"$p4c01_ownership" || fail "P4-C01 Contact ownership drifted: $anchor"
done
for mapping_id in LEGACY-API-0190 LEGACY-API-0191 LEGACY-API-0195 LEGACY-API-0196; do
  mapping_row="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$mapping_row" = *'"legacy_source_sha":"6cb989c071255437d75953dabb943318a74eb8f4"'* ]] ||
    fail "P4-C01 forged or lost frozen route authority for $mapping_id"
done
p4c01_migration_mapping="$(git show :docs/migration-mapping.jsonl | grep -F '"mapping_id":"LEGACY-T14-052"')"
[[ "$p4c01_migration_mapping" = *'"legacy_table":"automation_channel"'* &&
   "$p4c01_migration_mapping" = *'"implementation":"NOT_STARTED"'* &&
   "$p4c01_migration_mapping" = *'"verification":"NOT_RUN"'* ]] ||
  fail "P4-C01 forged historical import completion for LEGACY-T14-052"

p4c01_card="$(git show :docs/execution/slices/P4-C01.md)"
for anchor in \
  'LEGACY-API-0190/0191/0195/0196' \
  'LEGACY-S06-001/003/011' \
  'LEGACY-T14-052' \
  '31→32→31→32' \
  'slice_induced=1' \
  'REAL_WECOM_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4c01_card" || fail "P4-C01 slice card drifted: $anchor"
done

p4b02_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  'operationId: listLegacyWecomTags' \
  'operationId: createLegacyWecomTagGroup' \
  'operationId: updateLegacyWecomTagGroupPut' \
  'operationId: updateLegacyWecomTagGroupPatch' \
  'operationId: archiveLegacyWecomTagGroup' \
  'operationId: createLegacyWecomTag' \
  'operationId: updateLegacyWecomTagPut' \
  'operationId: updateLegacyWecomTagPatch' \
  'operationId: archiveLegacyWecomTag' \
  'x-p4-decision-evidence: P4-B02-2026-08-15' \
  'tag_limit: { type: integer, enum: [1000] }'; do
  grep -Fq -- "$anchor" <<<"$p4b02_openapi" || fail "P4-B02 OpenAPI drifted: $anchor"
done
for mapping_id in LEGACY-API-0555 LEGACY-API-0552 LEGACY-API-0553 LEGACY-API-0556 LEGACY-API-0562; do
  mapping_row="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$mapping_row" = *'"legacy_source_sha":"6cb989c071255437d75953dabb943318a74eb8f4"'* ]] ||
    fail "P4-B02 forged or lost frozen route authority for $mapping_id"
done
for feature_id in LEGACY-S05-010 LEGACY-S05-013 LEGACY-S05-014 LEGACY-S05-015 LEGACY-S05-016 LEGACY-S05-017 LEGACY-S05-018; do
  feature_row="$(git show :docs/feature-matrix.csv | grep -F "\"$feature_id\"")"
  [[ "$feature_row" = *'"MIGRATE","IMPLEMENTED","SYNTHETIC_PASS","APPROVED"'* && "$feature_row" = *'/pull/215'* ]] ||
    fail "P4-B02 feature matrix drifted for $feature_id"
done
for table_id in LEGACY-T14-148 LEGACY-T14-309 LEGACY-T14-310; do
  table_row="$(git show :docs/migration-mapping.jsonl | grep -F "\"mapping_id\":\"$table_id\"")"
  [[ "$table_row" = *'"target_schema_status":"PENDING_TARGET_SCHEMA"'* && "$table_row" = *'"implementation":"NOT_STARTED"'* && "$table_row" = *'"verification":"NOT_RUN"'* ]] ||
    fail "P4-B02 migration boundary drifted for $table_id"
done
p4b02_card="$(git show :docs/execution/slices/P4-B02.md)"
for anchor in '+5' '+7' 'slice_induced=7' 'SCOPE_FROZEN' 'REAL_WECOM_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4b02_card" || fail "P4-B02 slice card drifted: $anchor"
done
for ledger_anchor in \
  '  - slice_id: P4-B02' \
  '    status: SCOPE_FROZEN_REPAIR_ONLY_LOCAL_VERIFICATION' \
  '    slice_induced_correction_count: 6' \
  '    verification_induced_correction_count: 8' \
  '  - slice_id: P4-B02-R1' \
  '    status: REPAIR_ONLY_PR_OPEN_REQUIRED_CI_PENDING' \
  '    pr_url: https://github.com/qianlan33333-png/AI-CRM-v2/pull/215' \
  '    slice_induced_correction_count: 1' \
  '    verification_induced_correction_count: 11' \
  '  - slice_id: P4-B02-R2' \
  '    status: REPAIR_ONLY_PR_OPEN_REQUIRED_CI_PENDING' \
  '    base_sha: baae5a988a2248f088134068508d7768ff2dd262' \
  '    slice_induced_correction_count: 0' \
  '    verification_induced_correction_count: 3'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-B02 ledger drifted: $ledger_anchor"
done
for ledger_anchor in \
  '  - slice_id: P4-C01' \
  '    status: PR_OPEN_REQUIRED_CI_PENDING' \
  '    pr_url: https://github.com/qianlan33333-png/AI-CRM-v2/pull/213' \
  '    slice_induced_correction_count: 1' \
  '    verification_induced_correction_count: 10'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-C01 ledger drifted: $ledger_anchor"
done
for feature_id in LEGACY-S06-001 LEGACY-S06-003 LEGACY-S06-011; do
  p4c01_feature="$(git show :docs/feature-matrix.csv | grep -F "\"$feature_id\"")"
  [[ "$p4c01_feature" = *'"MIGRATE","IMPLEMENTED","SYNTHETIC_PASS","APPROVED"'* &&
     "$p4c01_feature" = *'https://github.com/qianlan33333-png/AI-CRM-v2/pull/213'* &&
     "$p4c01_feature" = *'edf1d85387a5646efad12d4dfd93eaf4a6ee24c6'* ]] ||
    fail "P4-C01 feature reconciliation drifted for $feature_id"
done

p4j01_migration="$(git show :migrations/00033_coupon_rules.sql)"
for anchor in \
  'CREATE TABLE public.coupons' \
  'CREATE TABLE public.coupon_targets' \
  'CREATE TABLE public.coupon_catalog_counters' \
  'CREATE TABLE public.coupon_operation_receipts' \
  'coupon_receipts_complete_before_commit' \
  'coupon_targets_claimed_frozen' \
  'coupons_claimed_rules_frozen' \
  '-- +goose Down'; do
  grep -Fq -- "$anchor" <<<"$p4j01_migration" || fail "P4-J01 migration drifted: $anchor"
done
! grep -Eiq 'REFERENCES[[:space:]]+(public[.])?(products|event_log|river_job)|tenant|payment|order|redemption' <<<"$p4j01_migration" ||
  fail "P4-J01 migration gained a cross-domain, tenant, or excluded commerce schema"

p4j01_routes="$(git show :cmd/aicrm/api.go)"
for anchor in \
  '{http.MethodGet, "/api/admin/coupons", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.ListCoupons)}' \
  '{http.MethodPost, "/api/admin/coupons", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.CreateCoupon)}' \
  '{http.MethodGet, "/api/admin/coupons/{coupon_id}", authport.CapabilityCouponsRead, false, http.HandlerFunc(legacy.GetCoupon)}' \
  '{http.MethodPut, "/api/admin/coupons/{coupon_id}", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.UpdateCoupon)}' \
  '{http.MethodPost, "/api/admin/coupons/{coupon_id}/publish", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.PublishCoupon)}' \
  '{http.MethodPost, "/api/admin/coupons/{coupon_id}/stop", authport.CapabilityCouponsWrite, true, http.HandlerFunc(legacy.StopCoupon)}'; do
  grep -Fq -- "$anchor" <<<"$p4j01_routes" || fail "P4-J01 route registration drifted: $anchor"
done

p4j01_service="$(git show :internal/coupon/app/service.go)"
for anchor in \
  'type ProductReader interface {' \
  's.uow.Within(ctx' \
  's.products.Get(ctx' \
  'p.Currency != "CNY"' \
  'p.PriceMinor <= c.DiscountAmountTotal' \
  'old.IssuedCount > 0 && !claimedUpdateAllowed' \
  'eventport.EvCouponPublished' \
  'eventport.EvCouponStopped'; do
  grep -Fq -- "$anchor" <<<"$p4j01_service" || fail "P4-J01 app contract drifted: $anchor"
done

p4j01_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  'operationId: listLegacyCoupons' \
  'operationId: createLegacyCoupon' \
  'operationId: getLegacyCoupon' \
  'operationId: updateLegacyCoupon' \
  'operationId: publishLegacyCoupon' \
  'operationId: stopLegacyCoupon' \
  'required: [name, discount_amount_total, total_issue_limit, claim_starts_at, claim_ends_at, validity_mode, target_refs]' \
  'create_replay_safe: { type: boolean, enum: [false] }' \
  'x-p4-decision-evidence: P4-J01-2026-08-15'; do
  grep -Fq -- "$anchor" <<<"$p4j01_openapi" || fail "P4-J01 OpenAPI drifted: $anchor"
done

p4j01_compat="$(git show :acceptance/coupon/j01_migration_compatibility.sh)"
for anchor in \
  'down-to 32' \
  'post_acceptance="$(snapshot)"' \
  'P4-J01 migration compatibility: PASS (32/33/32/33, pre-existing Event/Auth/session/Product history preserved)'; do
  grep -Fq -- "$anchor" <<<"$p4j01_compat" || fail "P4-J01 migration compatibility drifted: $anchor"
done
p4j01_recipe="$(make_target_recipe 'p4-j01-coupon-acceptance:')" ||
  fail "P4-J01 acceptance target must be unique"
[[ "$p4j01_recipe" = *'P4J01_COUPON_TEST_DATABASE_URL is required'* &&
   "$p4j01_recipe" = *'j01_migration_compatibility.sh'* &&
   "$p4j01_recipe" = *'./internal/coupon/... ./internal/product/...'* &&
   "$p4j01_recipe" = *'./internal/events/store ./internal/platform/http ./internal/auth/... ./cmd/aicrm'* ]] ||
  fail "P4-J01 acceptance target lost its directly affected Coupon/Product/Auth/Event chain"
grep -Fq 'P4J01_COUPON_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-j01-coupon-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-J01 acceptance is disconnected from application CI"

p4j01_ownership="$(git show :docs/architecture/table-ownership.yml)"
for anchor in \
  'coupon_targets: typed_standard_product_references_validated_through_product_read_port' \
  'coupon_operation_receipts: transaction_scoped_mutation_receipts_without_create_replay_claim'; do
  grep -Fq -- "$anchor" <<<"$p4j01_ownership" || fail "P4-J01 Coupon ownership drifted: $anchor"
done
for mapping_id in LEGACY-API-0285 LEGACY-API-0286 LEGACY-API-0289 LEGACY-API-0290 LEGACY-API-0294 LEGACY-API-0296; do
  mapping_row="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$mapping_row" = *'"legacy_source_sha":"6cb989c071255437d75953dabb943318a74eb8f4"'* &&
     "$mapping_row" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* ]] ||
    fail "P4-J01 forged or lost frozen route authority for $mapping_id"
done
for mapping_id in LEGACY-T14-143 LEGACY-T14-144 LEGACY-T14-145 LEGACY-T14-146; do
  mapping_row="$(git show :docs/migration-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$mapping_row" = *'"decision":"ARCHIVE_ONLY"'* && "$mapping_row" = *'"implementation":"NOT_STARTED"'* && "$mapping_row" = *'"verification":"NOT_RUN"'* ]] ||
    fail "P4-J01 forged historical import completion for $mapping_id"
done

p4j01_card="$(git show :docs/execution/slices/P4-J01.md)"
for anchor in \
  '12 个 JSON properties、7 个 required' \
  'LEGACY-API-0285/0286/0289/0290/0294/0296' \
  'LEGACY-S07-095/096/101/102' \
  'LEGACY-T14-143..146' \
  '32→33→32→33' \
  'slice_induced=6' \
  'REAL_CLAIM_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4j01_card" || fail "P4-J01 slice card drifted: $anchor"
done
for ledger_anchor in \
  '  - slice_id: P4-J01' \
  '    status: SCOPE_FROZEN_REPAIR_ONLY_PR_REQUIRED_CI_PENDING' \
  '    pr_url: https://github.com/qianlan33333-png/AI-CRM-v2/pull/214' \
  '    slice_induced_correction_count: 6' \
  '    verification_induced_correction_count: 8'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-J01 ledger drifted: $ledger_anchor"
done
for feature_id in LEGACY-S07-095 LEGACY-S07-096 LEGACY-S07-101 LEGACY-S07-102; do
  p4j01_feature="$(git show :docs/feature-matrix.csv | grep -F "\"$feature_id\"")"
  [[ "$p4j01_feature" = *'"MIGRATE","IMPLEMENTED","SYNTHETIC_PASS","APPROVED"'* &&
     "$p4j01_feature" = *'https://github.com/qianlan33333-png/AI-CRM-v2/pull/214'* &&
     "$p4j01_feature" = *'79cb843091e873160075215e411ce8a08e8aee70'* ]] ||
    fail "P4-J01 feature reconciliation drifted for $feature_id"
done

p4h03_migration="$(git show :migrations/00034_media_group_invite_library.sql)"
for anchor in \
  'CREATE TABLE public.media_group_invites' \
  'cover_image_id BIGINT REFERENCES public.media_images(id) ON DELETE RESTRICT' \
  'octet_length(title) BETWEEN 1 AND 128' \
  'octet_length(description) <= 512' \
  'CREATE TABLE public.media_group_invite_operation_receipts' \
  "operation IN ('create', 'update', 'archive')" \
  'media_group_invite_receipts_complete_before_commit' \
  '-- +goose Down'; do
  grep -Fq -- "$anchor" <<<"$p4h03_migration" || fail "P4-H03 Media migration drifted: $anchor"
done
! grep -Eiq 'tenant|chat_id|config_id|pic_url|provider|add_join_way|blob|upload' <<<"$p4h03_migration" ||
  fail "P4-H03 migration gained tenant, binding, provider, blob, or upload scope"

p4h03_routes="$(git show :cmd/aicrm/api.go)"
for anchor in \
  '{http.MethodGet, "/api/admin/group-invite-library", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.ListGroupInvites)}' \
  '{http.MethodPost, "/api/admin/group-invite-library", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.CreateGroupInvite)}' \
  '{http.MethodGet, "/api/admin/group-invite-library/{item_id}", authport.CapabilityMediaLibraryRead, false, http.HandlerFunc(legacy.GetGroupInvite)}' \
  '{http.MethodPut, "/api/admin/group-invite-library/{item_id}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.UpdateGroupInvite)}' \
  '{http.MethodDelete, "/api/admin/group-invite-library/{item_id}", authport.CapabilityMediaLibraryWrite, true, http.HandlerFunc(legacy.ArchiveGroupInvite)}'; do
  grep -Fq -- "$anchor" <<<"$p4h03_routes" || fail "P4-H03 route registration drifted: $anchor"
done

p4h03_service="$(git show :internal/media/app/group_invite.go)"
for anchor in \
  'type GroupInviteService struct {' \
  'service.uow.Within(ctx' \
  'service.images.ImageExists(tx' \
  'eventport.EvMediaGroupInviteCreated' \
  'eventport.EvMediaGroupInviteUpdated' \
  'eventport.EvMediaGroupInviteArchived' \
  'new(big.Rat).SetString'; do
  grep -Fq -- "$anchor" <<<"$p4h03_service" || fail "P4-H03 app contract drifted: $anchor"
done
! grep -Eiq 'add_join_way|provider|chat.?bind|blob|upload|tenant' <<<"$p4h03_service" ||
  fail "P4-H03 app gained provider, binding, blob, upload, or tenant scope"

p4h03_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  'operationId: listLegacyGroupInvites' \
  'operationId: createLegacyGroupInvite' \
  'operationId: getLegacyGroupInvite' \
  'operationId: updateLegacyGroupInvite' \
  'operationId: archiveLegacyGroupInvite' \
  'x-p4-decision-evidence: P4-H03-2026-08-15' \
  'x-utf8-max-bytes: 128' \
  'x-utf8-max-bytes: 512'; do
  grep -Fq -- "$anchor" <<<"$p4h03_openapi" || fail "P4-H03 OpenAPI drifted: $anchor"
done

p4h03_compat="$(git show :acceptance/media/h03_migration_compatibility.sh)"
for anchor in \
  'down-to 33' \
  "-run '^TestH03(CRUD|Four|Concurrent|S200K|Storage)'" \
  'P4-H03 migration compatibility: PASS (33/34/33/34, Event/Auth/Media history preserved)'; do
  grep -Fq -- "$anchor" <<<"$p4h03_compat" || fail "P4-H03 migration compatibility drifted: $anchor"
done
p4h03_recipe="$(make_target_recipe 'p4-h03-media-acceptance:')" ||
  fail "P4-H03 acceptance target must be unique"
[[ "$p4h03_recipe" = *'P4H03_MEDIA_TEST_DATABASE_URL is required'* &&
   "$p4h03_recipe" = *'h03_migration_compatibility.sh'* &&
   "$p4h03_recipe" = *'./internal/media/...'* &&
   "$p4h03_recipe" = *'./internal/events/store ./internal/platform/http ./internal/auth/... ./cmd/aicrm'* ]] ||
  fail "P4-H03 acceptance target lost the directly affected Media/Auth/Event chain"
grep -Fq 'P4H03_MEDIA_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-h03-media-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-H03 acceptance is disconnected from application CI"

p4h03_ownership="$(git show :docs/architecture/table-ownership.yml)"
for anchor in \
  'media_group_invites: local_only_group_invite_metadata_with_read_only_media_image_reference' \
  'media_group_invite_operation_receipts: actor_operation_business_key_scoped_transactional_mutation_receipts'; do
  grep -Fq -- "$anchor" <<<"$p4h03_ownership" || fail "P4-H03 Media ownership drifted: $anchor"
done
for mapping_id in LEGACY-API-0335 LEGACY-API-0336 LEGACY-API-0337 LEGACY-API-0338 LEGACY-API-0339; do
  mapping_row="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$mapping_row" = *'"legacy_source_sha":"6cb989c071255437d75953dabb943318a74eb8f4"'* &&
     "$mapping_row" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* ]] ||
    fail "P4-H03 forged or lost frozen route authority for $mapping_id"
  if [[ "$mapping_id" = LEGACY-API-0335 ]]; then
    [[ "$mapping_row" = *'"disposition":"MIGRATE"'* ]] || fail "P4-H03 0335 disposition drifted"
  else
    [[ "$mapping_row" = *'"disposition":"DEFERRED_POST_LAUNCH"'* ]] || fail "P4-H03 deferred P1 disposition drifted for $mapping_id"
  fi
done
p4h03_t14="$(git show :docs/migration-mapping.jsonl | grep -F '"mapping_id":"LEGACY-T14-183"')"
[[ "$p4h03_t14" = *'"decision":"DEFER"'* && "$p4h03_t14" = *'"implementation":"NOT_STARTED"'* && "$p4h03_t14" = *'"verification":"NOT_RUN"'* ]] ||
  fail "P4-H03 forged historical group invite import completion"

p4h03_card="$(git show :docs/execution/slices/P4-H03.md)"
for anchor in \
  'LEGACY-API-0335/0336/0337/0338/0339' \
  '33→34→33→34' \
  '+5 API' \
  '+0 feature matrix' \
  '+1 LEGACY-T14-183 physical target only' \
  'slice_induced=2' \
  'REAL_WECOM_NOT_EXECUTED'; do
  grep -Fq -- "$anchor" <<<"$p4h03_card" || fail "P4-H03 slice card drifted: $anchor"
done
for ledger_anchor in \
  '  - slice_id: P4-H03' \
  '    status: SCOPE_FROZEN_REPAIR_ONLY_PRE_PR_PASS_PR_PENDING' \
  '    slice_induced_correction_count: 2' \
  '    infra_induced_correction_count: 1' \
  '    verification_induced_correction_count: 7' \
  '  - slice_id: P4-H03-R1' \
  '    status: REPAIR_ONLY_PR_OPEN_REQUIRED_CI_PENDING' \
  '    pr_url: https://github.com/qianlan33333-png/AI-CRM-v2/pull/216' \
  '    verification_induced_correction_count: 2' \
  '    cumulative_corrections: [slice_induced=2, verification_induced=9, infra_induced=1, scope_induced=0, redline=0]' \
  '  - slice_id: P4-H03-R2' \
  '    status: REPAIR_ONLY_PR_CI_RESTART_PENDING' \
  '    verification_induced_correction_count: 2' \
  '    cumulative_corrections: [slice_induced=2, verification_induced=11, infra_induced=1, scope_induced=0, redline=0]'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-H03 ledger drifted: $ledger_anchor"
done
[[ "$(git show :docs/feature-matrix.csv | wc -l | tr -d ' ')" = "294" ]] ||
  fail "P4-H03 must preserve the exact 293-row feature matrix plus header"

verify_index_sha256 cmd/aicrm/legacy_config_settings.go \
  1c18fa75bee1073d353878b219d470a2294cce7fb174e76f02b5395d9efc1e8f
verify_index_sha256 cmd/aicrm/legacy_config_settings_test.go \
  98a5e37406274c4c4414ed5d0447e1209381b64722dba1103db0f05c84f41b30
verify_index_sha256 docs/execution/slices/P4-A02.md \
  2b6aa5e523bca13f57b7e8d01d80e37ec4947ec628e6a1c7d940e3bb9dc5c398
verify_index_sha256 internal/config/app/settings_compat.go \
  9635bcd5f4b018e6efe9ac0bac8d9fd89065ed690a6e2879ec9f998ca0d202a2
verify_index_sha256 internal/config/app/settings_compat_test.go \
  69b9821567b3d2564c0b454c159356f095d9604bbeee8f5d032f8201583e2e63
verify_index_sha256 internal/config/store/projection_repository.go \
  9e82e782785954320277e204ada1b928ef35af9a772da127bdcaeb1a7a17761c
verify_index_sha256 internal/config/store/queries/app_settings.sql \
  07d0941a2b35af46226851dc28020f900a0281ac39c2ab51b628c428be110761

p4a02_routes="$(git show :cmd/aicrm/api.go)"
for anchor in \
  '{http.MethodGet, "/admin/config/app-settings", authport.CapabilityConfigSettingsManage, false, http.HandlerFunc(legacy.AppSettingsPage)}' \
  '{http.MethodPost, "/admin/config/app-settings/save", authport.CapabilityConfigSettingsManage, true, http.HandlerFunc(legacy.SaveAppSettings)}' \
  '{http.MethodGet, "/api/admin/config/app-settings", authport.CapabilityConfigSettingsManage, false, http.HandlerFunc(legacy.AppSettingsResource)}'; do
  grep -Fq -- "$anchor" <<<"$p4a02_routes" || fail "P4-A02 route registration drifted: $anchor"
done
p4a02_auth="$(git show :internal/auth/app/policy.go)"
grep -Fq 'authport.CapabilityConfigSettingsManage:' <<<"$p4a02_auth" || fail "P4-A02 capability policy missing"
grep -A2 -F 'authport.CapabilityConfigSettingsManage:' <<<"$p4a02_auth" | grep -Fq 'admin: authport.ScopeGlobal' || fail "P4-A02 admin global capability drifted"
! grep -A2 -F 'authport.CapabilityConfigSettingsManage:' <<<"$p4a02_auth" | grep -Eq 'ops:|sales:' || fail "P4-A02 capability widened beyond admin"

p4a02_app="$(git show :internal/config/app/settings_compat.go)"
for anchor in \
  'type SecretConfiguredSnapshot struct {' \
  'type MaskedSettingRow struct {' \
  'Configured bool `json:"configured"`' \
  'Masked     bool `json:"masked"`' \
  'projection.Rows = make([]any, 0, len(catalog))' \
  'projection.AuditEntries = make([]AuditEntry, 0, len(audits))' \
  'RequestID: input.RequestID + ":" + rawKey' \
  'service.manager.Set(ctx, command)'; do
  grep -Fq -- "$anchor" <<<"$p4a02_app" || fail "P4-A02 app contract drifted: $anchor"
done
[[ "$(grep -Ec '^[[:space:]]*\{configport\.' <<<"$(grep -A13 '^var catalog' <<<"$p4a02_app")")" = "12" ]] || fail "P4-A02 catalog is not exactly twelve keys"
! grep -Eiq 'secret(port|value)|tenant|provider|river' <<<"$p4a02_app" || fail "P4-A02 app gained secret, tenant, provider, or River scope"
! grep -Fq 'row.Version =' <<<"$p4a02_app" || fail "P4-A02 non-secret version must remain fixed empty"

p4a02_sql="$(git show :internal/config/store/queries/app_settings.sql)"
for anchor in \
  "WHERE s.key IN ('wecom.corp_id', 'wecom.agent_id', 'outbound.rate_per_second', 'outbound.max_attempts')" \
  "WHERE key IN ('wecom.corp_id', 'wecom.agent_id', 'outbound.rate_per_second', 'outbound.max_attempts')" \
  'ORDER BY updated_at DESC, id DESC' \
  'LIMIT 10'; do
  grep -Fq -- "$anchor" <<<"$p4a02_sql" || fail "P4-A02 projection SQL drifted: $anchor"
done
! grep -Eiq 'new_value|request_id|SELECT[[:space:]]+old_value' <<<"$p4a02_sql" || fail "P4-A02 projection selected audit values or request IDs"
! grep -Eiq 'count\([^)]*audit|[[:space:]]AS[[:space:]]+version' <<<"$p4a02_sql" || fail "P4-A02 projection derived a forbidden version"

p4a02_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  'operationId: getLegacyAppSettingsPage' \
  'operationId: saveLegacyAppSettingsPage' \
  'operationId: getLegacyAppSettingsResource' \
  'x-aicrm-capability: config.settings.manage' \
  'x-aicrm-session-bound-csrf: required' \
  'version: { type: string, enum: [""] }' \
  'source_status: { type: string, enum: [next_read_model] }'; do
  grep -Fq -- "$anchor" <<<"$p4a02_openapi" || fail "P4-A02 OpenAPI drifted: $anchor"
done

for mapping_id in LEGACY-API-0026 LEGACY-API-0027 LEGACY-API-0253; do
  mapping_row="$(git show :docs/api-mapping.jsonl | grep -F "\"mapping_id\":\"$mapping_id\"")"
  [[ "$mapping_row" = *'"legacy_source_sha":"6cb989c071255437d75953dabb943318a74eb8f4"'* && "$mapping_row" = *'"disposition":"MIGRATE"'* && "$mapping_row" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* ]] || fail "P4-A02 forged or lost frozen route authority for $mapping_id"
done
p4a02_feature="$(git show :docs/feature-matrix.csv | grep -F '"LEGACY-S05-053"')"
[[ "$p4a02_feature" = *'"MIGRATE","IMPLEMENTED","SYNTHETIC_PASS","APPROVED"'* && "$p4a02_feature" = *'sha=05541a6dfbedbec0797a2cca19a43f41512512f6;pr=https://github.com/qianlan33333-png/AI-CRM-v2/pull/217'* && "$p4a02_feature" = *'command=go_test_race+acceptance_p2s03+generate_check+orval_check+repo_contract+gitleaks'* ]] || fail "P4-A02 feature matrix release evidence drifted or forged"
p4a02_t14="$(git show :docs/migration-mapping.jsonl | grep -F '"mapping_id":"LEGACY-T14-024"')"
[[ "$p4a02_t14" = *'"decision":"MANUAL_REENTRY"'* && "$p4a02_t14" = *'"implementation":"NOT_STARTED"'* && "$p4a02_t14" = *'"verification":"NOT_RUN"'* && "$p4a02_t14" = *'4_non_secret_keys'* && "$p4a02_t14" = *'legacy values imported=false'* && "$p4a02_t14" = *'legacy secrets imported=false'* && "$p4a02_t14" = *'legacy updated_at imported=false'* && "$p4a02_t14" = *'production_execution=false'* ]] || fail "P4-A02 T14 manual re-entry evidence drifted"
for ledger_anchor in \
  '  - slice_id: P4-A02' \
  '    mapping_status: exact_plus_3_API_0026_0027_0253_plus_1_S05_053_plus_1_T14_024_MANUAL_REENTRY_NOT_STARTED_NOT_RUN_mapping_status_only' \
  '    inherited_slice_induced_correction_count: 2' \
  '    inherited_verification_induced_correction_count: 2' \
  '    fresh_slice_induced_correction_count: 2' \
  '    fresh_verification_induced_correction_count: 11' \
  '  - slice_id: P4-A02-R1' \
  '    pr_url: https://github.com/qianlan33333-png/AI-CRM-v2/pull/217' \
  '    verification_induced_correction_count: 1' \
  '  - slice_id: P4-A02-R2' \
  '    infra_induced_correction_count: 1' \
  '    verification_induced_correction_count: 3' \
  '  - slice_id: P4-A02-R3' \
  '    verification_induced_correction_count: 2' \
  '    cumulative_corrections: [inherited_slice_induced=2, inherited_verification_induced=2, fresh_slice_induced=2, fresh_verification_induced=17, fresh_infra_induced=1, fresh_scope_induced=0, redline=0]' \
  '  - slice_id: P4-A02-R4' \
  '    verification_induced_correction_count: 1' \
  '    cumulative_corrections: [inherited_slice_induced=2, inherited_verification_induced=2, fresh_slice_induced=2, fresh_verification_induced=18, fresh_infra_induced=1, fresh_scope_induced=0, redline=0]'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-A02 ledger drifted: $ledger_anchor"
done

p4i03_migration="$(git show :migrations/00035_order_list_projection.sql)"
for anchor in \
  'CREATE TABLE public.order_list_projections (' \
  'CREATE TABLE public.order_list_projection_counters (' \
  'REFERENCING NEW TABLE AS inserted_rows' \
  'REFERENCING OLD TABLE AS deleted_rows' \
  'FOR EACH STATEMENT EXECUTE FUNCTION public.aicrm_order_list_projection_count_insert()' \
  'FOR EACH STATEMENT EXECUTE FUNCTION public.aicrm_order_list_projection_count_delete()' \
  'order_list_projections_merchant_order_trgm_idx' \
  'order_list_projections_mobile_trgm_idx' \
  '-- +goose Down'; do
  grep -Fq -- "$anchor" <<<"$p4i03_migration" || fail "P4-I03 migration drifted: $anchor"
done
! grep -Eiq 'REFERENCES[[:space:]]+(public[.])?(customers|products|event_log|river_job)|tenant_id|organization_id|workspace_id|payment_receipt|refund|callback' <<<"$p4i03_migration" ||
  fail "P4-I03 migration gained a cross-domain, tenant, payment, refund, or callback schema"

p4i03_routes="$(git show :cmd/aicrm/api.go)"
grep -Fq '{http.MethodGet, "/api/admin/orders", authport.CapabilityOrderRead, false, http.HandlerFunc(legacy.ListOrders)}' <<<"$p4i03_routes" ||
  fail "P4-I03 exact read-only route registration drifted"
! grep -Eq '\{http.Method(Post|Put|Patch|Delete), "/api/admin/orders' <<<"$p4i03_routes" ||
  fail "P4-I03 gained a forbidden order write/detail route"

p4i03_app="$(git show :internal/order/app/list.go)"
for anchor in \
  'service.uow.Within(ctx' \
  'service.customers.ReadCustomer(ctx' \
  'service.products.ReadProduct(ctx' \
  'fmt.Sprintf("%d.%02d"' \
  'page.HasMore = int64(filter.Offset)+int64(len(page.Items)) < page.Total'; do
  grep -Fq -- "$anchor" <<<"$p4i03_app" || fail "P4-I03 application contract drifted: $anchor"
done
! grep -Eiq 'tenant|provider call|payment|refund|callback|enqueue|eventport|river' <<<"$p4i03_app" ||
  fail "P4-I03 application gained excluded scope"
p4i03_queries="$(git show :internal/order/store/queries/orders.sql)"
! grep -Eiq '(^|[[:space:]])(customers|products|event_log|river_job)([[:space:]]|$)' <<<"$p4i03_queries" ||
  fail "P4-I03 Order store gained a cross-domain table read"

p4i03_openapi="$(git show :api/openapi.yaml)"
for anchor in \
  'operationId: listLegacyOrders' \
  'x-legacy-mapping-ids: [LEGACY-API-0405]' \
  'x-p4-decision-evidence: P4-I03-2026-08-15' \
  'x-aicrm-capability: order.read' \
  'required: [items, total, limit, has_more]' \
  'amount_yuan: { type: string, pattern:'; do
  grep -Fq -- "$anchor" <<<"$p4i03_openapi" || fail "P4-I03 OpenAPI drifted: $anchor"
done

p4i03_mapping="$(git show :docs/api-mapping.jsonl | grep -F '"mapping_id":"LEGACY-API-0405"')"
[[ "$p4i03_mapping" = *'"legacy_source_sha":"6cb989c071255437d75953dabb943318a74eb8f4"'* && "$p4i03_mapping" = *'"candidate_v2_operation_id":"PENDING_HUMAN_DESIGN"'* ]] ||
  fail "P4-I03 forged or lost frozen route authority"
git diff --cached --quiet -- docs/migration-mapping.jsonl docs/migration-mapping.md ||
  fail "P4-I03 T14 +0 contract was modified"

p4i03_compat="$(git show :acceptance/order/i03_migration_compatibility.sh)"
for anchor in \
  'down-to 34' \
  'up-to 35' \
  'P4-I03 migration compatibility: PASS (34/35/34/35, Event/Auth/session history preserved, no cross-domain FK)'; do
  grep -Fq -- "$anchor" <<<"$p4i03_compat" || fail "P4-I03 migration compatibility drifted: $anchor"
done
p4i03_recipe="$(make_target_recipe 'p4-i03-order-acceptance:')" || fail "P4-I03 acceptance target must be unique"
[[ "$p4i03_recipe" = *'P4I03_ORDER_TEST_DATABASE_URL is required'* && "$p4i03_recipe" = *'i03_migration_compatibility.sh'* && "$p4i03_recipe" = *'./internal/order/... ./internal/contact/store ./internal/product/store ./internal/auth/... ./cmd/aicrm'* ]] ||
  fail "P4-I03 acceptance target lost its directly affected Order/Contact/Product/Auth chain"
grep -Fq 'P4I03_ORDER_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" make p4-i03-order-acceptance' <(git show :.github/workflows/application-go.yml) ||
  fail "P4-I03 acceptance is disconnected from application CI"

for ledger_anchor in \
  '  - slice_id: P4-I03' \
  '    status: SCOPE_FROZEN_REPAIR_ONLY_PRE_PR_BLIND_REVIEW_PASS' \
  '    mapping_status: exact_plus_1_API_0405_plus_1_matrix_S07_174_plus_0_T14' \
  '    slice_induced_correction_count: 2' \
  '    slice_induced_threshold: 2' \
  '    hard_stop: false' \
  '    migration_correction_attribution: [slice_induced=1, verification_induced=0, infra_induced=0, scope_induced=0, redline=0, first_S200K_load_exceeded_180_seconds, repaired_S200K_load_4_6_seconds]' \
  '    verification_induced_correction_count: 12' \
  '    scope_induced_correction_count: 0'; do
  grep -Fq -- "$ledger_anchor" <<<"$slice_policy_ledger" || fail "P4-I03 ledger drifted: $ledger_anchor"
done

scripts/scan_sensitive_paths.sh

echo "repo-contract: PASS"
