#!/usr/bin/env bash
set -euo pipefail

: "${P4EER_TEST_DATABASE_URL:?P4EER_TEST_DATABASE_URL is required}"
base_database_url="$P4EER_TEST_DATABASE_URL"
[[ "$base_database_url" = *'/aicrm_test?sslmode=disable' ]]
database_url="${base_database_url%/aicrm_test?sslmode=disable}/aicrm_test_eer_00075?sslmode=disable"
ch02_database_url="${base_database_url%/aicrm_test?sslmode=disable}/aicrm_test_eer_ch02_00090?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_eer_00075 WITH (FORCE)' >/dev/null
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_eer_ch02_00090 WITH (FORCE)' >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_eer_00075' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 75 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '75' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '74' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 75 >/dev/null
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -run '^TestExternalEffectsPG16' ./acceptance/externaleffects -args -external-effects-database-url "$database_url"
set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/tmp/aicrm-eer-down.out 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back populated external effects runtime' /tmp/aicrm-eer-down.out
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_eer_ch02_00090' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$ch02_database_url" up-to 90 >/dev/null
[[ "$(psql "$ch02_database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$ch02_database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '90' ]]
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -run '^TestCH02TerminalEvidencePG16$' ./acceptance/contact -args -ch02-store-database-url "$ch02_database_url"
printf 'P4 EER PG16 acceptance: PASS (75 down/up, CAS lease/fence, attempts, receipts, reconciliation; CH02 terminal evidence; no provider)\n'
