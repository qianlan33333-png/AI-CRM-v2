#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/ownership/main.go"
go_bin="${GO:-go}"
[[ "$go_bin" == /* ]] || go_bin="$(type -P "$go_bin" || true)"
[[ -x "$go_bin" && -f "$checker" && ! -L "$checker" ]] || exit 1
test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-ownership.XXXXXX")"
trap 'rm -rf "$test_root"' EXIT
seed() {
  local root="$1"
  mkdir -p "$root/docs/architecture" "$root/internal/contact/store/queries" \
    "$root/internal/segment/store/queries" "$root/internal/outbound/worker" \
    "$root/internal/wecom/store" "$root/internal/platform/store" \
    "$root/internal/events/store/queries" \
    "$root/internal/media/store/queries" \
    "$root/internal/product/store/queries" "$root/internal/hxc/store/queries" \
    "$root/internal/radar/store/queries" "$root/internal/survey/store/queries" \
    "$root/internal/automation/store/queries" "$root/internal/stats/store/queries" \
    "$root/acceptance/fixtures" "$root/acceptance/contactfixture" \
    "$root/acceptance/automationfixture" "$root/acceptance/mediafixture" \
    "$root/acceptance/datamigration"
  cp "$script_dir/../docs/architecture/table-ownership.yml" "$root/docs/architecture/"
  printf '%s\n' 'INSERT INTO customers (id) VALUES (1);' >"$root/internal/contact/store/queries/write.sql"
  printf '%s\n' 'INSERT INTO media_image_delete_receipts (id) VALUES (1);' >"$root/internal/media/store/queries/write.sql"
  printf '%s\n' "SELECT 'UPDATE identities'; -- DELETE FROM tags" 'SELECT * FROM customers;' >"$root/internal/segment/store/queries/read.sql"
  printf '%s\n' 'package worker' 'const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/message/send"' >"$root/internal/outbound/worker/client.go"
  printf '%s\n' 'package store' 'const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/externalcontact/get"' >"$root/internal/wecom/store/client.go"
  printf '%s\n' 'package fixtures' \
    'const ddl = "CREATE TABLE acceptance_fixtures.fixture_probe (id bigint PRIMARY KEY)"' \
    'const dml = "INSERT INTO acceptance_fixtures.fixture_probe (id) VALUES (1)"' \
    >"$root/acceptance/fixtures/probe.go"
  printf '%s\n' 'package contactfixture' \
    'const dml = "INSERT INTO channels (name) VALUES ($1)"' \
    >"$root/acceptance/contactfixture/customer.go"
  printf '%s\n' 'package automationfixture' \
    'const dml = "INSERT INTO automation_agent_configurations (agent_name) VALUES ($1)"' \
    >"$root/acceptance/automationfixture/agent.go"
  printf '%s\n' 'package mediafixture' \
    'const dml = "INSERT INTO media_images (name) VALUES ($1)"' \
    >"$root/acceptance/mediafixture/image.go"
  printf '%s\n' 'package datamigration' \
    'const dml = "INSERT INTO data_migration_runs (id) VALUES ($1)"' \
    >"$root/acceptance/datamigration/harness.go"
  printf '%s\n' \
    'INSERT INTO event_log (event_type) VALUES ($1)' \
    'ON CONFLICT (idempotency_key) DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key;' \
    'SELECT id FROM event_log FOR UPDATE SKIP LOCKED;' \
    >"$root/internal/events/store/queries/upsert.sql"
  printf '%s\n' \
    'INSERT INTO stage_operation_receipts DEFAULT VALUES;' \
    'INSERT INTO tag_catalog_operation_receipts DEFAULT VALUES;' \
    >"$root/internal/contact/store/queries/current-tables.sql"
  printf '%s\n' \
    'INSERT INTO ai_audience_package_groups DEFAULT VALUES;' \
    'INSERT INTO ai_audience_operation_receipts DEFAULT VALUES;' \
    'INSERT INTO ai_audience_package_automation_bindings DEFAULT VALUES;' \
    'INSERT INTO ai_audience_package_senders DEFAULT VALUES;' \
    'INSERT INTO ai_audience_local_configuration_receipts DEFAULT VALUES;' \
    >"$root/internal/segment/store/queries/current-tables.sql"
  printf '%s\n' \
    'INSERT INTO product_local_entitlements DEFAULT VALUES;' \
    'INSERT INTO entitlement_operation_receipts DEFAULT VALUES;' \
    'INSERT INTO service_period_member_views DEFAULT VALUES;' \
    'INSERT INTO service_period_member_grid_collaborators DEFAULT VALUES;' \
    >"$root/internal/product/store/queries/current-tables.sql"
  echo 'INSERT INTO hxc_sender_config_receipts DEFAULT VALUES;' >"$root/internal/hxc/store/queries/current-tables.sql"
  printf '%s\n' \
    'INSERT INTO media_attachments DEFAULT VALUES;' \
    'INSERT INTO media_attachment_blobs DEFAULT VALUES;' \
    'INSERT INTO media_attachment_mutation_receipts DEFAULT VALUES;' \
    >"$root/internal/media/store/queries/current-tables.sql"
  printf '%s\n' \
    'INSERT INTO radar_links DEFAULT VALUES;' \
    'INSERT INTO radar_link_idempotency_records DEFAULT VALUES;' \
    >"$root/internal/radar/store/queries/current-tables.sql"
  printf '%s\n' \
    'INSERT INTO questionnaire_operations DEFAULT VALUES;' \
    'INSERT INTO questionnaire_operations_receipts DEFAULT VALUES;' \
    'INSERT INTO questionnaire_external_push_test_runs DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_definitions DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_definition_questions DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_definition_options DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_submission_receipts DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_submissions DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_submission_answers DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_submission_rate_windows DEFAULT VALUES;' \
    'INSERT INTO questionnaire_public_management_receipts DEFAULT VALUES;' \
    >"$root/internal/survey/store/queries/current-tables.sql"
}
run_checker() {
  (cd / && env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE \
    GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
    "$go_bin" run "$checker" -root "$1")
}
positive="$test_root/positive"
seed "$positive"
[[ "$(run_checker "$positive")" == "ownership-lint: PASS" ]] || exit 1
reject() {
  local name="$1" expected="$2" root="$test_root/$1" output status
  seed "$root"
  mutate "$name" "$root"
  set +e; output="$(run_checker "$root" 2>&1)"; status=$?; set -e
  [[ "$status" -ne 0 && "$output" == *"$expected"* ]] || {
    echo "ownership-lint-tests: accepted or misdiagnosed $name: $output" >&2
    exit 1
  }
}
mutate() {
  local name="$1" root="$2"
  case "$name" in
    contact-identity) echo 'UPDATE identities SET customer_id = 1;' >"$root/internal/contact/store/queries/write.sql" ;;
    segment-write) echo 'DELETE FROM customers;' >"$root/internal/segment/store/queries/read.sql" ;;
    platform-write) echo 'INSERT INTO event_log DEFAULT VALUES;' >"$root/internal/platform/store/write.sql" ;;
    contact-event-update) echo 'UPDATE event_log SET dispatched = true;' >"$root/internal/contact/store/queries/write.sql" ;;
    automation-event-delivery) echo 'UPDATE event_deliveries SET status = '\''completed'\'';' >"$root/internal/automation/store/queries/write.sql" ;;
    stats-event-delivery) echo 'UPDATE event_deliveries SET status = '\''completed'\'';' >"$root/internal/stats/store/queries/write.sql" ;;
    acceptance-event-delivery) mkdir -p "$root/acceptance/automation"; echo 'package automation; const dml = "INSERT INTO event_deliveries (event_id, consumer) VALUES (1, '\''automation.tag-trigger.v1'\'')"' >"$root/acceptance/automation/direct_event_write.go" ;;
    contact-auth-session) echo 'UPDATE admin_sessions SET revoked_reason = '\''bypass'\'';' >"$root/internal/contact/store/queries/write.sql" ;;
    contact-media-delete-receipt) echo 'INSERT INTO media_image_delete_receipts (id) VALUES (1);' >"$root/internal/contact/store/queries/write.sql" ;;
    automationfixture-contact-write) echo 'package automationfixture; const dml = "INSERT INTO customers (name) VALUES ($1)"' >"$root/acceptance/automationfixture/agent.go" ;;
    mediafixture-automation-write) echo 'package mediafixture; const dml = "INSERT INTO automation_agent_configurations (agent_name) VALUES ($1)"' >"$root/acceptance/mediafixture/image.go" ;;
    unknown-table) echo 'TRUNCATE TABLE ONLY mystery_table;' >"$root/internal/contact/store/queries/write.sql" ;;
    update-unknown-table) echo 'UPDATE mystery_table AS target SET id = 2;' >"$root/internal/contact/store/queries/write.sql" ;;
    public-fixture) printf '%s\n' 'package fixtures' 'const ddl = "CREATE TABLE public.mystery_table (id bigint PRIMARY KEY)"' >"$root/acceptance/fixtures/probe.go" ;;
    acceptance-unowned-customer-write) mkdir -p "$root/acceptance/identity"; printf '%s\n' 'package identity' 'const dml = "INSERT INTO customers (name) VALUES ($1)"' >"$root/acceptance/identity/customer.go" ;;
    outbound-read) echo 'package worker; const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/externalcontact/get"' >"$root/internal/outbound/worker/client.go" ;;
    wecom-write) echo 'package store; const endpoint = "/cgi-bin/message/send"' >"$root/internal/wecom/store/client.go" ;;
    contact-endpoint) mkdir -p "$root/internal/contact/app"; echo 'package app; const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/message/send"' >"$root/internal/contact/app/client.go" ;;
    contact-sdk) mkdir -p "$root/internal/contact/app"; echo 'package app; import _ "example.com/wecomsdk"' >"$root/internal/contact/app/client.go" ;;
    unknown-operation) echo 'package worker; const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/unknown/write"' >"$root/internal/outbound/worker/client.go" ;;
    fifo) mkfifo "$root/internal/contact/unexpected" ;;
  esac
}
reject contact-identity 'table write ownership violation'; reject segment-write 'table write ownership violation'
reject platform-write 'table write ownership violation'; reject contact-event-update 'table write ownership violation'
reject automation-event-delivery 'table write ownership violation'
reject stats-event-delivery 'table write ownership violation'
reject acceptance-event-delivery 'table write ownership violation'
reject contact-auth-session 'table write ownership violation'
reject contact-media-delete-receipt 'table write ownership violation'
reject automationfixture-contact-write 'table write ownership violation'
reject mediafixture-automation-write 'table write ownership violation'
reject unknown-table 'write to unknown table'
reject update-unknown-table 'write to unknown table'
reject public-fixture 'write to unknown table'
reject acceptance-unowned-customer-write 'table write ownership violation'
reject outbound-read 'WeCom operation ownership violation'; reject wecom-write 'WeCom operation ownership violation'
reject contact-endpoint 'WeCom operation ownership violation'; reject contact-sdk 'external WeCom client import forbidden'
reject unknown-operation 'unknown WeCom operation'
reject fifo 'symlink or special path forbidden'
echo "ownership-lint-tests: PASS"
