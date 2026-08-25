#!/usr/bin/env bash
set -euo pipefail

: "${P4AUTOMATIONRULES_TEST_DATABASE_URL:?P4AUTOMATIONRULES_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4AUTOMATIONRULES_TEST_DATABASE_URL"
temporary_database="aicrm_test_p4_a01_00080"
database_url="${base_database_url/aicrm_test/$temporary_database}"
down_output="$(mktemp -t aicrm-a01-down.XXXXXX)"

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
  rm -f "$down_output"
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null

MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 80 AND is_applied)')" = 't' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 79 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 80 AND is_applied)')" = 'f' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 78 AND is_applied)')" = 't' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestA01TagRuleOutboundMessageUsesEERAndRiverWithoutProviderClaim$' ./acceptance/automation \
  -args -database-url "$database_url"

set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 79 >"$down_output" 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back populated automation runtime' "$down_output"
printf 'P4 A01 Automation runtime PG16 acceptance: PASS (80 down/up, EER outbound, unknown/manual reconcile, populated guard)\n'
