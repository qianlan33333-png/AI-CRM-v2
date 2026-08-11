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
  internal/events/store/appender.go
  internal/events/store/appender_test.go
  internal/events/store/queries/event_log.sql
  internal/events/store/generated/db.go
  internal/events/store/generated/event_log.sql.go
  internal/events/store/generated/models.go
  internal/events/store/generated/querier.go
  internal/identity/port/port.go
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
  tools/query-plan-gate/main.go
  tools/query-plan-gate/main_test.go
  scripts/build_slice_bundle.sh
  scripts/check_arch_imports.go
  scripts/ownership/main.go scripts/test_ownership.sh
  scripts/sourcepolicy/main.go scripts/test_source_policy.sh
  scripts/check_slice_inputs.sh scripts/test_slice_inputs.sh
  scripts/check_generated_sources.sh
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
  docs/architecture/port-contracts.md
  docs/architecture/table-ownership.yml
  docs/governance/limitations.md
  docs/execution/slice-card-template.md
  docs/execution/slice-ledger.yml
  docs/execution/slices/P0-S02.md
  docs/execution/slices/P0-S03.md
  docs/execution/slices/P0-S04.md
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
100644 web/src/customers.ts
100644 web/src/customers.test.ts
100644 web/src/customers-ui.tsx
100644 web/src/customers-ui.test.tsx
100644 web/src/customers-list.css
100644 web/src/shell.css
100644 web/src/api/generated/health.ts
100644 .github/workflows/application-go.yml
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
100644 internal/events/store/appender.go
100644 internal/events/store/appender_test.go
100644 internal/events/store/queries/event_log.sql
100644 internal/events/store/generated/db.go
100644 internal/events/store/generated/event_log.sql.go
100644 internal/events/store/generated/models.go
100644 internal/events/store/generated/querier.go
100644 internal/identity/port/port.go
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
  b68f5be334c8d7dff7eec1e2df35ff60522a8700bbfa6ec91d4d8671159c5a44
verify_index_sha256 CONTRIBUTING.md \
  851670c7ae917f3e7a3b03d9bec30d687afcb61ccf868fe26f6b547fc8a6273f
verify_index_sha256 .github/CODEOWNERS \
  bb2c40eaad8b8b3dd83cd2d81f58360717ab6dbaeb773afe6d65b7ae18e4f5cb
verify_index_sha256 go.mod \
  fc223e80d21cc5b2f20bcfcfb5bc219d993993c1b480621442b6f8c692071a97
verify_index_sha256 go.sum \
  411aa7f8fff51ca54e7b0c3f84323a2bcbec8541a9a16cb01c3e46af1c24dee7
verify_index_sha256 package.json \
  a0ce7f09b7397cca843f74b00a7c1d2ee2d019bf287019490d0de09c8460f68f
verify_index_sha256 package-lock.json \
  64f32f2bc22dbde74f3e0e82fbfa91c1160621fc1a771832a0a0b06fb11e2892
verify_index_sha256 web/src/api/generated/health.ts \
  e5b1e51c092e94ebf98b8b16454c486ea458602d7ac58d19e89ec6b6d9c8a5c6
verify_index_sha256 .github/workflows/application-go.yml \
  768648239f7c492efca6c050d2eecc3f9db0d10b8e3efa44d259f0ec16c16781
verify_index_sha256 .github/workflows/repo-contract.yml \
  300a14e1c96209efe09e98d319c446962d24eaf7f5a33ecbc6bf1e16d81d4883
verify_index_sha256 .github/workflows/secret-scan.yml \
  e3077f509e0cfe5a9b70c4064cc666f53258c62cda590f191ea401d1734d02fe
verify_index_sha256 .gitleaks.toml \
  b220c3b1e00671ed5d45f796b341a586a533659b7eecadf4906516769414ff74
verify_index_sha256 scripts/test_gitleaks_config.sh \
  2c4da4f3e1fc926910a516593513c8f1e2f51445879bd7a9a5574ce47396dcf3
verify_index_sha256 docs/execution/slices/M0-7.md \
  0b9cd7cbd3ae679b57b54361d8d7d9f0ff34e1568f55bf118505a048c9e229a4
verify_index_sha256 scripts/check_generated_sources.sh \
  af8650c82d52b1fc90bcfedd441f4bd43ef05798894f06e75bac32d35fb26b63
verify_index_sha256 scripts/test_gitless_generated_check.sh \
  a1c2ecdbad13520ff52d1cc5219363621529c4c74fd2ba8cd53cb3dbb6c6c9ca
verify_index_sha256 scripts/generated-sources.sha256 \
  3b55f31a750438148f1f0fa0948f72e1b18f0bd1834ca334adb34be49fc48c3b
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
  49fe144941c712ee47ef9b99c603e5eef70194a1039cd5cef87a6cdb1657a283
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
  ad7a93fc7c0ad095e90b7817f6268eb3a60971d577a95c6859fbfc1108f52f44
verify_index_sha256 migrations/00002_event_log.sql \
  ffae249b7d5398d0bdacdb72078663b9646d0af908aee2c259a9d476dce73b62
verify_index_sha256 internal/events/port/port.go \
  a149222cb0dd2dde8e7e2be092cd74921396fce475cb8608997c1d6d6510bff9
verify_index_sha256 internal/events/store/queries/event_log.sql \
  96d351b2faf428ae291064a21bb23f9a32be4580ae3f65203eed988e17757e21
verify_index_sha256 internal/events/store/appender.go \
  820730005a9835051fbacaa31ca6b161f17feb99543a127195a6e1ad059964b9
verify_index_sha256 internal/events/store/appender_test.go \
  ef4cab40e75b1630acab2c576fb8f89071bb2a9bac032fa43414286371045a4f
verify_index_sha256 internal/events/store/generated/db.go \
  08295f6e16bef8e5d3ea40cf12f296ddd5a25965c44a447a35eb6ddc9ef08ca8
verify_index_sha256 internal/events/store/generated/event_log.sql.go \
  35f1c0c866751e3dcce63771b0076a8f35044cd5fe822b8338b99889fe62299b
verify_index_sha256 internal/events/store/generated/models.go \
  f1bdaa47b54cc769e5d2911263163f58b3bfab594c189adeda5f3311853cd2e3
verify_index_sha256 internal/events/store/generated/querier.go \
  3d57b59f2cd541f2179f6dbfad8211facb80b14ce141de82fa6bfd968df48e4c
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
  c1b6543f57eb5e1455fec3a4b2907180fff6700c6aba0a9a51a036adf4eb6fb8
verify_index_sha256 internal/config/store/generated/settings.sql.go \
  bcd88d6a1bc79fa2cbefdf2454ea53f8649b314d53d0300c93022bebad124e1f
verify_index_sha256 acceptance/p2s03/doc.go \
  1f235f5da9ab144143ba8bc2523c1e1e0c1679a961d778b0bbf22802c5c2bb10
verify_index_sha256 acceptance/p2s03/settings_integration_test.go \
  f9d6aa321d7763f95badb4cd90188efc9119f9f27e728730240b6cc328c08abc
verify_index_sha256 docs/execution/slices/SEC-01.md \
  94947cc722e3898c156004491758fafe550bdbb3188dc69aa2a7553bfe77ab92
verify_index_sha256 scripts/check_arch_imports.go \
  6d06c304c397fd7966ffa7a52d70a888323465e511a7c9db63521b35e0881224
verify_index_sha256 scripts/test_arch_imports.sh \
  2905b7e910816cecb685f0b5e34476c67100a62a21ea1b6e015a6e58a884aa74
verify_index_sha256 scripts/ownership/main.go \
  94d56f1479ee25eb13643ed97565a73fd5e774178510b118d172d7fd1dbac22e
verify_index_sha256 scripts/test_ownership.sh \
  5a887619857112b7ab55c72bc417c6d51f7804722dfed33c9b56b8d93787ebeb
verify_index_sha256 acceptance/fixtures/postgres.go \
  e9e04301d41b57d59eb49f74d75767803a2847c41ee5fab8c383adb586de670b
verify_index_sha256 acceptance/fixtures/postgres_test.go \
  60b0d65c2fa950765166b5ad1cc65f6a5d962f496d50ce6231c34520377fecf3
verify_index_sha256 docs/execution/slices/P2-00.md \
  7f625dc6dd0017266faaf779a79ca093bf600bb4b51adc61660751be86b16022
verify_index_sha256 scripts/sourcepolicy/main.go \
  350924119f5f190d1e399d2e84f8f163d5c5ea7b0dbfc2a0652ba9b7a3c077c0
verify_index_sha256 scripts/test_source_policy.sh \
  ea5b70241c85adeed28bd6b4f0ad1f887630615b882aac209af4e42e15cc184e
verify_index_sha256 AGENTS.md \
  6d7bbe6739e98fd878d9fa7550726841f616f0190778b03587025a0cc025173f
verify_index_sha256 scripts/check_slice_inputs.sh \
  b7b1711da73974b0a89c79bab020e519095fc8aaf36f737b036027ec3a08cb25
verify_index_sha256 scripts/test_slice_inputs.sh \
  32ec32f2c3f0c7ec7c29aabfada4bc4c1f29dc39a37fc0aacfb51e30d304d8a7
verify_index_sha256 docs/execution/slices/M0-1.md \
  1bb201f3550ef638ea85f8f7f5de585b57825177c400a7f5cab3094ef68f6043
verify_index_sha256 docs/execution/slices/M0-2.md \
  97d9acda27150c905af7bc52eeca623034ae1c529d89aac56c253f18d426df59
verify_index_sha256 tools/go.mod \
  6a64133379331817837ab27823bcf7c672d3a300b2cf9d3c2c79c56f7f90e7ef
verify_index_sha256 tools/go.sum \
  2515f9dd3dbc17c77f98550be06a8cdf072538e6d8eb296077b6ad91120d2753
verify_index_sha256 tools/query-plan-gate/main.go \
  f36e5be67a56a4d969206d870fe4a0104c9387a6caeacf6ac96ac7eeb1686b15
verify_index_sha256 tools/query-plan-gate/main_test.go \
  c61d43bc6e10ec992ab68385c0f0280559f4c44cb8a0ad7d05d9516b511e9798
verify_index_sha256 scripts/test_query_plan_gate.sh \
  61a1ce22acc6358b697c50e191c02a6d2e8a0fe20b9d00c2070cececdc8bb497
verify_index_sha256 docs/execution/slices/M0-3.md \
  c14caaed56bc85a386ece43639053b10556d3d4eae25816fbefe334430d6ba0f
verify_index_sha256 docs/spec/AI-CRM-v2-执行方案.md \
  210f6d3c9d0434cba6426ab71fc1cc64bc3a6d3a1a184e55af5f1273c21a8099
verify_index_sha256 docs/spec/AI-CRM-v2-执行方案-v2-至P3.md \
  816f04447e1af046d4fe6ef24b436aa062b535decc32d6a463055121dd3f6a46
verify_index_sha256 docs/spec/AI-CRM-v2-重构详细设计.md \
  b9260c4fe20c26395a2af4d75e2f1aec184b6f1175c998c535b34f5383503745
verify_index_sha256 docs/spec/SHA256SUMS \
  c2cb503d3504f2c21d0c643aea9991851d0ad4db6a2137c61e31634a07850478
verify_index_sha256 tools/snapshot-gate/main.go \
  425cb0ea7702d9aeb817687487f97db27b7e3c03b8a5a95df722aedd8390992c
verify_index_sha256 tools/snapshot-gate/main_test.go \
  77771f548652fc2ffe556b8f8fd31a8f394cc0e90d3e57cb7014711894a29d9b
verify_index_sha256 acceptance/snapshots/catalog.v1.json \
  e6fe9ca10b662c11804d7715ccd847a0b2826652b22237768d5d292e5bba6d62
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
  67e594387f982151b0cf488640975ae7e25a1bf456a3fca3f869c6c208a775af
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
  d7b9aa9ccb7679c2e4e1b1d6e1a9a3aba147fc7c205f1b62e9f11a3e490d8011
verify_index_sha256 tools/p1-reconciliation/main.go \
  2b1162a4a423b9f106b512d162a5ebc4d3bc5fded125caaa69bc0d7b823ade99
verify_index_sha256 tools/p1-reconciliation/main_test.go \
  ecf7d359ce9bac04949e78edc20484a2a759adcd606c8ba971e5e4976ae1f703
verify_index_sha256 docs/execution/slices/P1-C03.md \
  cd9e0441d79b9e1887030087bb4dd800a0a3ca3529275008083d00c577572ffc
verify_index_sha256 api/openapi.yaml \
  be1077aca278d582babce70b4f12cc4b2e78e363efb55c6b2bdd6040f4b073b8
verify_index_sha256 api/oapi-codegen.yaml \
  78abf754fe91788d5cbdab2286ba66dc32d5e13ed1735ffeee9119e473fd4a2b
verify_index_sha256 api/oapi-codegen-p1-candidate.yaml \
  307c9ae17d2e7ff9b315d35f720caf4a58af65bbd9531337b8061e4046fca452
verify_index_sha256 internal/api/generated/server.gen.go \
  8d893a61822a423198e81d12b007f13ec1844b19ef129677a389203aa9e0fd42
verify_index_sha256 internal/api/candidate/generated/server.gen.go \
  95f8fd7ad3331a6fa7433c334b170f0cb111e433ffc6755a49149bebda88fd7b
verify_index_sha256 tools/openapi-contract/main.go \
  bd3e6cdae992ded3e2119bc3e9b532c804b6273845a2f97313c2a1dcaaccbd16
verify_index_sha256 tools/openapi-contract/main_test.go \
  1f9bb4a30bdf02bfbaf48573b31f596e1287c6f6a287c28649583ac2751ac0bd
verify_index_sha256 acceptance/p1s11/contracts_test.go \
  231f4300b4f248fe902ab5ea77a66636776e7544738e8fbad3408e11b9a7f15a
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
verify_index_sha256 docs/execution/slices/P3-C01A.md \
  cb271879f61d4f901c020641faf22880b3903032252402b8b5a637c6c9aaa2d5
verify_index_sha256 internal/contact/http/customer_list_handler.go \
  b8673e84e4aebe2bb349759c1db60f9ca65ec34632e90a232eb48c3d9e061b18
verify_index_sha256 internal/contact/http/customer_list_handler_test.go \
  eaef2b6013f82ff8aa7f4161f535b6a098c8e8a72fa0823f38a4eb670756c1e9
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
  a098212cf3c8f9c2a250d18ee06049e50b269d7419978c0977e8fff075756281
verify_index_sha256 internal/contact/app/customer_mutation_service.go \
  9f99976cf74dbb2a13c0c8f903f163f458da5e5bc912031c00daf192875af307
verify_index_sha256 internal/contact/app/customer_mutation_service_test.go \
  f54c07eff903579f936d3f008e1b0b096005887ae48dbed079218bb4962cc64c
verify_index_sha256 internal/contact/store/customer_mutation_repository.go \
  fea8a0925963b810a582cc515a5503544ae52b21e421ed92bf7388fdea67ed77
verify_index_sha256 internal/contact/store/customer_mutation_repository_test.go \
  9a9a96a76c05eec8cdde07eb6d0ce46289c4bf941dffbca927017c22e73c65f8
verify_index_sha256 internal/contact/store/queries/customer_mutations.sql \
  2c6d972ac26cc4adddf24a9f2419dcee18a2bbf8cc1885c9cd89c5b113902f39
verify_index_sha256 internal/contact/store/generated/customer_mutations.sql.go \
  bdb9d819b1704a54459ffb2f33169eb2c2cfa492e9e77fd3bf8404d0e9141096
verify_index_sha256 docs/execution/slices/P3-C02A.md \
  d5f8cf552a6ed81b042120cb0070b0fae061447b0552f88cf0ec3454b6cc1c07
verify_index_sha256 docs/evidence/slices/P3-C02A-sqlc-store.md \
  1c4c82565b6531e35bd7b657c583fa1fab4591b871550af145ecab4d966db40b
verify_index_sha256 docs/evidence/slices/P3-C02A-service-tests.md \
  019b694aa844264ea8cbe6f686060df33dc430248ddcc567dd7f02181bd21c1d
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
verify_index_sha256 docs/execution/slices/P1-S11.md \
  5866fe52a0039f310c10add3d8cfa77eaba9d748dcf518d71df04dac2354a872
verify_index_sha256 internal/auth/port/port.go \
  ba396d41036414f472f450270b994215308c61ec93e9241628cc825c6714c9eb
verify_index_sha256 internal/contact/port/port.go \
  ffc24ba99eae2f845b773d5cda572779901f8246ea7a7a0eb976feb571e5e27b
verify_index_sha256 internal/identity/port/port.go \
  321d6518b3e5fec57f3591307334e9fac67c06018bec727790f45e0e55ab5627
verify_index_sha256 internal/platform/port/uow.go \
  f8f9b381c9cdbcabbeea9403e8379c33464b7356522abdb383d0e09a6f5996c1
verify_index_sha256 internal/platform/store/uow.go \
  46591bbf2833b97ce06d9cc1513ca2aadc70f21de05544e836b453d35da51b7e
verify_index_sha256 internal/platform/store/uow_test.go \
  4524e09f7200b7b445cdab73be7ee921d619adc624de3b790f4fcd395017b0d7
verify_index_sha256 cmd/aicrm/main.go \
  52fe62cdda6653e597ca338c4cb9a47605b47fb15c21410f6156f6d05691d180
verify_index_sha256 cmd/aicrm/components.go \
  70e2c4ef4073da4f5f562b4966e4db42998cc8e11c16e34dcdae30356f70704f
verify_index_sha256 cmd/aicrm/components_test.go \
  b81bf5c6370a3e89dbd99308d7ad31cdb03e716e76c77f412544ab32318a56e0
verify_index_sha256 cmd/aicrm/scheduler.go \
  b31c60bfd11ee2a70546d6bfe0420e7a957d1baefa3a6ecb48a6d8013ad41c76
verify_index_sha256 cmd/aicrm/scheduler_test.go \
  775381cd93cdcc96307b947e57541529ebca147dfedd524673c7bc54e090d15a
verify_index_sha256 internal/config/load.go \
  3df220675a71df7c798681c43e0ebc300342b7396fb60a5b867faed787c81b84
verify_index_sha256 internal/config/schema.go \
  9f0a9a98edb39a5b06ac58433f17036592810116180a9f4dd24c1686fac46bf7
verify_index_sha256 internal/config/schema_test.go \
  bcd594e45894bbe3499cd9e85225ea476ef54299e328da49a61a333cc126e480
verify_index_sha256 acceptance/p0s01/process_blackbox.sh \
  dca96d9df61c3c67e2254d59e22c300850b58841d93eca70b3f1743df294ce6b
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
  37f55bc45d7afcc45c13c6c3de0d95cddb512a8fb5cc186f9b69b6a258659fa7
verify_index_sha256 internal/events/dispatcher/dispatcher.go \
  1f326e3ac2fd692680b353266265018edd4a6edbca58506fac0f3e001212b0ba
verify_index_sha256 internal/events/dispatcher/dispatcher_test.go \
  baae4dfd41d343885d6e30a11bbd49079164636a92f9107f2365e404f9080f78
verify_index_sha256 internal/events/dispatcher/jobs.go \
  5c9ecd33343c3b382fe3b0af0a0314ec5c9147c7ae3a357e1e26f594ee2d3a39
verify_index_sha256 acceptance/p2s08/doc.go \
  8c7efd59df7d54ec7c2f032beb4da78baa87ddccf874c852a22a9d21218ccb50
verify_index_sha256 acceptance/p2s08/gateway_blackbox_test.go \
  e522f404c6dde64fc0f339af29e92e1352090b914912c8443e2eb5daa1c99956
verify_index_sha256 internal/platform/http/errors.go \
  21b6da0d4a110b9564f3324d00414ed59405c7577cd3379d00749812f941bf9f
verify_index_sha256 internal/platform/http/gateway.go \
  41242be737a46bba1036996eff8ceffe173e7af27934d00ded41c3c800ed127b
verify_index_sha256 internal/platform/http/gateway_test.go \
  b9f0a58b7e33195b6ad2fdff0ea74a34a31f75e3bd2eb0ffd9aa17552991f528
verify_index_sha256 docs/execution/slices/P2-08.md \
  f9c10d171a4256764945f333ba2c591aca6c3824938f1980d849403b15b9e7f4
verify_index_sha256 docs/evidence/slices/P2-08-http-tests.md \
  a61b50a0d040373515216d96fe1707a14be7a9852e8a1b9e64ba94c07ae35fcc
verify_index_sha256 migrations/00004_auth.sql \
  777e6634e63db30a4f0ab2e3c17afd0cc98235863753a74efc4871c379e797c3
verify_index_sha256 migrations/00005_contact_core.sql \
  226c660db0a572a97c23322b6f1dd0e694f210033d7ab4992b59bd8349ef0432
verify_index_sha256 internal/auth/app/service.go \
  134595b199e6074160d6a3a6fe127a2c162ae97a364375e17e133e37d4c4d070
verify_index_sha256 internal/auth/app/service_test.go \
  30ea2a41875b7bd1702693d4d9791c500fdc8412420a1bc68d958f5a1839487e
verify_index_sha256 internal/auth/http/handler.go \
  2859f6e0215a486ae4c0e66551f6a2096e1e6385b56fd9c4ff99b00d2feb611f
verify_index_sha256 internal/auth/store/repository.go \
  3d1fa186097474f8905bc94c60eebca238dd6d194b3e2de14c4c00bb334a907b
verify_index_sha256 internal/auth/store/queries/auth.sql \
  1b922fdea16ededcf81151f9ebb8c0f3a21c3d307a942b23f47a19def418ebbd
verify_index_sha256 internal/auth/store/generated/auth.sql.go \
  2ef674e81977dfe0c8fc61121f27aeb9603f0a7c82f3ea3771e1be2de6f8729e
verify_index_sha256 internal/auth/store/generated/db.go \
  121194f70f0c6bd5ff393988502dac16cb3ebd421032d65a6de2d8f176c2f832
verify_index_sha256 internal/auth/store/generated/models.go \
  8ad356407329c9294431184c7e4f75c836c185912bbb7fc90b61c49009ab853e
verify_index_sha256 internal/auth/store/generated/querier.go \
  0a747dae9400ab1e4e993fa75560f1d860cf2af2e916164f44df756b6e21c762
verify_index_sha256 acceptance/p2s09/doc.go \
  719f525dc416a2d9c7e6360a2446519b641b8f17cd2a56d521642dac0a67da0e
verify_index_sha256 acceptance/p2s09/session_integration_test.go \
  3ea3cff9dd9a50f6406edfd4383b9027b4f4812bd391290195794fdf79c45dce
verify_index_sha256 docs/execution/slices/P2-09.md \
  bd75e25e956d1f1abaccf5e1e4b4e705bee5981f2908c33626193d7a86794160
verify_index_sha256 docs/evidence/slices/P2-09-auth-service-tests.md \
  5edbbf1d8c4d10761a4a91bf2e2c8cf7206be786226fdb889ce481e049199f36
verify_index_sha256 internal/auth/port/port_test.go \
  29f21e47b1b9afb39e2cff39e73a2fd1e801f04c1c4e87682439f753e43956de
verify_index_sha256 internal/auth/app/policy.go \
  0b95b4c304623c9f8740603d30a19fc71d5183e9cba637c2e0d9eadc95c0d397
verify_index_sha256 internal/auth/app/policy_test.go \
  507ca4a43d89b5e8e33d685b4b6e6aac9d11cc15817d9d65062f2d0098da82a6
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
  4940772716260e6ea0dbe1d8f33f06adc218a3eafe664e313cb77018bac28588
verify_index_sha256 cmd/aicrm/api_test.go \
  1f17dd26ddc2bbd314dc97ee90d3aa16de5c17342dbe244cb71e6852903e34c8
verify_index_sha256 acceptance/p2s11/doc.go \
  735a2c1eb929a5046d53d60a522b9b46f9c822dc20c85846eb358d2b80f15a5d
verify_index_sha256 acceptance/p2s11/gateway_router_test.go \
  3f5cfb5e929d5ff103b67b1d9497cd041e35a0895944f4da80ef4704521dec36
verify_index_sha256 docs/execution/slices/P2-11.md \
  621f0fa454d672f0bab5f8589d737db91e1b3a7137407a680cc141fccaf7f34b
verify_index_sha256 docs/evidence/slices/P2-11-gateway-tests.md \
  db10c68cc987690f3a812ce5966c499597fc90aaafef9b4b04cdd0dd6eba1be6
verify_index_sha256 web/src/main.tsx \
  799e39ea92a77fa4687fc16fe18e572ee6724768a53bd05b09de18c207624b6c
verify_index_sha256 web/src/main.test.tsx \
  d26ca2f058a911e48950e83f7ca6dd39b616e82bfad3aa84b325506a4e788bd9
verify_index_sha256 web/src/customers.ts \
  a4dddd6400879040c8269b2e69620db20401d42890050464ab815d4e139772a0
verify_index_sha256 web/src/customers.test.ts \
  5a5d6e31f3b6c2097a614bb06177ccdc23a76114faef2dda64fede1261879103
verify_index_sha256 web/src/customers-ui.tsx \
  a9b6d658a5c15b32fae61aa51f0ec9d7f6d72a2fb2330bc7fc7e7481b8ef25e0
verify_index_sha256 web/src/customers-ui.test.tsx \
  87c3b414351ff52237fbee09403a6d311157fcd36a9d90944d4f25723f3e4650
verify_index_sha256 web/src/customers-list.css \
  e2e9522f30b1cd44606667f4372bb5fb76b143111a36bac36bf625ed3e6a8b3e
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
  755625464a65ef6a71d93d5261e1255b9f0d0becf5678aa015604ff5581c4d49
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
  3091eb04593e0277076ca5aaa74f44023d2b0cbe591e0ceba9e4b2067f046bba
verify_index_sha256 acceptance/p2s16/snapshot.go \
  f2ca9b322bc23e4738891fc8e8a7bb840b362852d2d7f8deb381236744b6d852
verify_index_sha256 acceptance/p2s16/snapshot_test.go \
  0e6cdd93bd1c110b2e53fb932c3ce6afe089fbb507aa054902c2d0f0ccd918f6
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
  4c77bb954881911f0ec29c4fbe4817b9401bbbc5360dc56279cd3f1490c0a64a
verify_index_sha256 internal/contact/store/generated/models.go \
  9459ba27d0397425970580f71f26f1871214fcd1cbb0b1eb48bb1143a97ec956
verify_index_sha256 internal/contact/store/generated/querier.go \
  dde6acc09d703ea0b0754a5fe075e3115fe9fc400d6f5b56734c2f32fa864447
verify_index_sha256 internal/contact/store/generated/stages.sql.go \
  24abe8b30311c9a7134c8daab59b487caae03f72a6d1ab50d587c536eb5046f5
verify_index_sha256 internal/contact/store/repository.go \
  e3ca8720cf6789a7b6aab4603513622b7feff27a42fb2f86ca394250951c7bab
verify_index_sha256 acceptance/p2s14/doc.go \
  372bee4c9610e5cd4ab9696e2efe65fed8fa42868e3e86ab632a8883d7f09869
verify_index_sha256 acceptance/p2s14/stages_store_integration_test.go \
  c9768333556d987e7e01c522d2229362438114900af5191420415ca8b9d40143
verify_index_sha256 web/src/auth.ts \
  dddcd144109e5e86b3df79ce4cc76ed09cdede90ce879e0c458b71f6f404c14f
verify_index_sha256 web/src/auth.test.ts \
  1c53880d61fd71f8ee0e0737db2f376998a105cb745d350257680d7790fe3598
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
  8b709c1b9aa3e511ada07889889a34f9c8be0a932dcea36e3e96490d371bc6f0
verify_index_sha256 docs/adr/ADR-001.md \
  e4da265cf5ffd9962d1f77f2410538e09e8b41d1cb37c1584c3d57265480e28d
verify_index_sha256 docs/adr/ADR-010.md \
  5fdbd62214938e6485322a84858c488a8bac812e00f312318186a5b3ec9b72dc
verify_index_sha256 docs/adr/ADR-011.md \
  3fb1954942b0de9da1989276d535af090c0dcd22841437dbb7d6e49e54b7f92d
verify_index_sha256 docs/execution/slices/M0-5.md \
  c5a4f1991b8f3ecbb1a3a024c6131aea5fc3d6813ddeb34b323faf2948229609
verify_index_sha256 docs/execution/slices/M0-6.md \
  96f5131c60d2eec508557f03ba1322af88c2002a259ec8d455569024d2013125
verify_index_sha256 docs/architecture/canonical.md \
  ac61872a4ad45e368ba8ebf40ec3da2f6e07399c1ad8487ce695f987275861f2
verify_index_sha256 docs/architecture/table-ownership.yml \
  10b7cfa37bcf19371284ded7841a2a9cd5dbd25cdbd9689c81f4cfc815dc206a
verify_index_sha256 scripts/test_repo_contract.sh \
  a7cb14663ce2f9919db061f00b5bc00a61bf7c5896d036d2aca3f7f5f2191ea1
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
  969e834043c841f533c3405da13f2048178cb31982ac0619aec04f77ed600340
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
verify_index_sha256 scripts/verify_repo_receipts.pl \
  d28a528cfc1aa8d8a5c6fa62b652699059bb632e8640fbd868ba0e3967881e27

scripts/verify_repo_receipts.pl receipts "${receipt_arguments[@]}"

makefile="$(git show ':Makefile')"; alternate="$(find . -maxdepth 1 \( -name GNUmakefile -o -name makefile \) -print -quit)"; [[ -z "$alternate" ]] || fail "alternate Make entrypoint is forbidden: ${alternate#./}"
! grep -Eq '^[[:space:]]*(-?include|sinclude)([[:space:]]|$)' <<<"$makefile" && ! awk 'index($0,"$") && substr($0,1,1) != "\t" && $0 !~ /^[[:space:]]*#/ { bad=1 } END { exit bad ? 0 : 1 }' <<<"$makefile" ||
  fail "Makefile must not construct or import rules dynamically"
printf '%s\n' "$makefile" | grep -Eq '^p0-s03-contract:[[:space:]]*$' ||
  fail "Makefile is missing the P0-S03 contract target"
p0_s03_contract_recipe="$(
  awk '
    /^p0-s03-contract:[[:space:]]*$/ { capture = 1; next }
    capture && /^[^[:space:]]/ { exit }
    capture { print }
  ' <<<"$makefile"
)"
printf '%s\n' "$p0_s03_contract_recipe" |
  grep -Fqx $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s03/test_contract.sh' ||
  fail "P0-S03 contract target must run the gitless contract tests"
printf '%s\n' "$makefile" |
  grep -Eq '^p0-s03-acceptance:[[:space:]]+p0-s03-contract([[:space:]]|$)' ||
  fail "P0-S03 acceptance target must depend on the contract target"
ci_go_target="$(printf '%s\n' "$makefile" | grep -E '^ci-go:[[:space:]]' || true)"
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
  [[ "$(grep -Fc 'GOWORK=off $(GO) list ./...' <<<"$recipe" || true)" = "1" ]] ||
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
  'bootstrap-tools: missing Go 1.26.5; install versions from .tool-versions' \
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
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./openapi-contract' \
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
      if ($0 != "  contents: read") invalid_permission = 1
    }
    END {
      exit !(saw_permissions && permission_entries == 1 && !invalid_permission)
    }
  ' "$workflow" ||
    fail "$workflow must declare only top-level contents: read"

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
grep -Fq 'EvCustomerUpdated = "customer.updated"' <<<"$p3c02a_events" ||
  fail "P3-C02A customer.updated event contract drifted"

p3c02a_openapi="$(git show :api/openapi.yaml)"
[[ "$(grep -Fc 'gender: { type: integer, format: int32, minimum: -32768, maximum: 32767, nullable: true }' <<<"$p3c02a_openapi")" -eq 2 ]] ||
  fail "P3-C02A gender bounds must remain aligned with PostgreSQL SMALLINT"

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

scripts/scan_sensitive_paths.sh

echo "repo-contract: PASS"
