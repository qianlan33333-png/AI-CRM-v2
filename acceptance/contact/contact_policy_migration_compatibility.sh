#!/usr/bin/env bash
set -euo pipefail

: "${P4CONTACTPOLICY_TEST_DATABASE_URL:?P4CONTACTPOLICY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4CONTACTPOLICY_TEST_DATABASE_URL"
temporary_database="aicrm_test_contact_policy_00065"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT

fresh_database() {
  cleanup
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
}

MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

waterline() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied"
}

prepare_contact_policy_waterline() {
  latest_waterline="$(waterline)"
  [[ "$latest_waterline" -ge "65" ]]
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 65 >/dev/null
  [[ "$(waterline)" = "65" ]]
}

restore_latest_waterline() {
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to "$latest_waterline" >/dev/null
  [[ "$(waterline)" = "$latest_waterline" ]]
}

expect_facts_reject_down() {
  local output
  if output="$("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down 2>&1)"; then
    printf '%s\n' "$output" >&2
    echo "expected customer contact policy fact-preserving down to fail" >&2
    exit 1
  fi
  [[ "$output" == *"SQLSTATE 55000"* ]]
  [[ "$(waterline)" = "65" ]]
}

fresh_database
prepare_contact_policy_waterline
P4CONTACTPOLICY_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestContactPolicy' ./internal/contact/store
restore_latest_waterline

fresh_database
prepare_contact_policy_waterline
customer_id="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "INSERT INTO customers(name) VALUES('contact policy migration guard') RETURNING id")"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO customer_contact_policies(customer_id,reason_code,created_at,updated_at) VALUES($customer_id,'operator_hold',now(),now())" >/dev/null
expect_facts_reject_down
restore_latest_waterline

fresh_database
prepare_contact_policy_waterline
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO customer_contact_policy_operation_receipts(operation,actor_scope,key_digest,payload_digest,state,result_snapshot,created_at,completed_at) VALUES('customer_contact_policy.clear','customer_contact_policy:actor:1',decode(repeat('01',32),'hex'),decode(repeat('02',32),'hex'),'completed','{}'::jsonb,now(),now())" >/dev/null
expect_facts_reject_down
restore_latest_waterline

fresh_database
prepare_contact_policy_waterline
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
[[ "$(waterline)" = "64" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 65 >/dev/null
[[ "$(waterline)" = "65" ]]
restore_latest_waterline

printf 'P4 Contact Policy migration compatibility: PASS (65 facts guard, 64/65 empty down/up, restored %s)\n' "$latest_waterline"
