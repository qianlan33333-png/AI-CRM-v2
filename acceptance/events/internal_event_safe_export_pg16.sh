#!/usr/bin/env bash
set -euo pipefail

: "${P4EE01_TEST_DATABASE_URL:?P4EE01_TEST_DATABASE_URL is required}"
base_database_url="$P4EE01_TEST_DATABASE_URL"
database_url="${base_database_url/aicrm_test/aicrm_test_ee01_00073}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" MIGRATION_TEST_DATABASE_NAME="aicrm_test" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_ee01_00073 WITH (FORCE)' >/dev/null; }
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_ee01_00073' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = "160014" ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = "73" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -run '^TestInternalEventSafeExportPG16$' ./acceptance/events -args -ee01-database-url "$database_url"
printf 'P4 EE01 Internal Event Safe Export PG16 acceptance: PASS\n'
