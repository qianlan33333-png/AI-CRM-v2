#!/usr/bin/env bash
set -euo pipefail

: "${P4CUSTOMER_SAFE_EXPORT_TEST_DATABASE_URL:?P4CUSTOMER_SAFE_EXPORT_TEST_DATABASE_URL is required}"
base_database_url="$P4CUSTOMER_SAFE_EXPORT_TEST_DATABASE_URL"
database_url="${base_database_url/aicrm_test/aicrm_test_customer_safe_export_00071}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_customer_safe_export_00071 WITH (FORCE)' >/dev/null; }
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_customer_safe_export_00071' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = "160014" ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = "71" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
P4CUSTOMER_SAFE_EXPORT_ACCEPTANCE_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -count=1 -race -run '^TestCustomerSafeExportLocalCorePG16$' ./internal/contact/store
if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/tmp/aicrm-customer-safe-export-down.log 2>&1; then
  echo 'customer safe export facts unexpectedly allowed migration rollback' >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' /tmp/aicrm-customer-safe-export-down.log
printf 'P4 Customer Safe Export Local Core PG16 acceptance: PASS\n'
