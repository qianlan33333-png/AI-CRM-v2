#!/usr/bin/env bash
set -euo pipefail

: "${SEGMENT_CRUD_TEST_DATABASE_URL:?SEGMENT_CRUD_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$SEGMENT_CRUD_TEST_DATABASE_URL"
temporary_database="aicrm_test_p3_s05b_segment_crud"
database_url="${base_database_url/aicrm_test/$temporary_database}"
advisory_lock_key=1395668290 # ASCII "S05B"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  local exit_code=$? cleanup_code=0
  trap - EXIT
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null || cleanup_code=$?
  printf 'SELECT pg_advisory_unlock(%s);\n\\q\n' "$advisory_lock_key" >&"$lock_input_fd" || cleanup_code=$?
  wait "$lock_pid" || cleanup_code=$?
  if ((exit_code != 0)); then
    exit "$exit_code"
  fi
  exit "$cleanup_code"
}

coproc lock_session { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -At; }
lock_pid=$lock_session_PID
lock_output_fd=${lock_session[0]}
lock_input_fd=${lock_session[1]}
printf 'SELECT 1 FROM (SELECT pg_advisory_lock(%s)) AS acquired;\n' "$advisory_lock_key" >&"$lock_input_fd"
read -r lock_acquired <&"$lock_output_fd"
[[ "$lock_acquired" = "1" ]]
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null

MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  SEGMENT_CRUD_TEST_DATABASE_URL="$database_url" \
  "$go_command" test -race -count=1 -timeout=45s -run '^TestSegmentCRUDReceiptAndRuntimeFlow$' ./acceptance/segment

printf 'P3-S05B Segment CRUD acceptance: PASS (exact dedicated database; shared database unchanged)\n'
