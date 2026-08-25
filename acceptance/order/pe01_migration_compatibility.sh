#!/usr/bin/env bash
set -euo pipefail

: "${P4PE01_TEST_DATABASE_URL:?P4PE01_TEST_DATABASE_URL is required}"
base_database_url="$P4PE01_TEST_DATABASE_URL"
[[ "$base_database_url" = *'/aicrm_test?sslmode=disable' ]]
database_url="${base_database_url%/aicrm_test?sslmode=disable}/aicrm_test_pe01_00079?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
down_output="$(mktemp -t aicrm-pe01-down.XXXXXX)"

cleanup() {
  rm -f "$down_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_pe01_00079 WITH (FORCE)' >/dev/null
}
trap cleanup EXIT
cleanup
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_pe01_00079' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '79' ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '75' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null

GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -timeout=240s -run '^TestPE01' ./acceptance/order -args -database-url "$database_url"

set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >"$down_output" 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back materialized PE01 financial or entitlement facts' "$down_output"
printf 'P4 PE01 PG16.14 acceptance: PASS (79 empty down/up, fake-provider query reconciliation, full-refund compensation, populated 55000 guard)\n'
