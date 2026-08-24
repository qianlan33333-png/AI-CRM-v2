#!/usr/bin/env bash
set -euo pipefail

: "${P4SIDEBAR_TEST_DATABASE_URL:?P4SIDEBAR_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4SIDEBAR_TEST_DATABASE_URL"
temporary_database="aicrm_test_sidebar_00069"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null

[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SHOW server_version_num")" = "160014" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "69" ]]

P4SIDEBAR_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=120s -run '^TestSidebarProfilePostgreSQL16' ./internal/contact/store

if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/tmp/aicrm-sidebar-down.log 2>&1; then
  echo "completed sidebar receipt unexpectedly allowed migration 69 rollback" >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' /tmp/aicrm-sidebar-down.log

printf 'P4 S05 Sidebar Local Core PG16 acceptance: PASS (fresh migration 69, receipt/CAS/race/replay, fact-preserving rollback guard)\n'
