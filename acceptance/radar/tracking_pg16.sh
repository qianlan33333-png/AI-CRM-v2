#!/usr/bin/env bash
set -euo pipefail

: "${P4_RADAR_TRACKING_TEST_DATABASE_URL:?P4_RADAR_TRACKING_TEST_DATABASE_URL is required}"
base_database_url="$P4_RADAR_TRACKING_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
database_name='aicrm_test_radar_00081'
database_url="${base_database_url%"$database_suffix"}/$database_name?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-radar-00081-down.XXXXXX")"

MIGRATION_TEST_DATABASE_URL="$base_database_url" MIGRATION_TEST_DATABASE_NAME='aicrm_test' \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  rm -f "$guard_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $database_name WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $database_name" >/dev/null

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 81 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 81 AND is_applied')" = '1' ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 81 AND is_applied')" = '0' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 81 >/dev/null

GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestRadarLocalTrackingPostgreSQL16$' ./acceptance/radar \
  -args -radar-tracking-database-url "$database_url"

set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >"$guard_output" 2>&1
guard_status=$?
set -e
[[ "$guard_status" -ne 0 ]]
grep -Fq 'cannot roll back populated radar local tracking' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 81 AND is_applied')" = '1' ]]

printf 'P4 Radar Local Tracking Core PG16.14 acceptance: PASS (empty down/up, local receipts, concurrent replay/conflict, filters/stats/sidebar, PII minimization, fact guard)\n'
