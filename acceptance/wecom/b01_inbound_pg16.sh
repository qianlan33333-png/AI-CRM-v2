#!/usr/bin/env bash
set -euo pipefail

: "${P4B01_WECOM_INBOUND_TEST_DATABASE_URL:?P4B01_WECOM_INBOUND_TEST_DATABASE_URL is required}"
base_database_url="$P4B01_WECOM_INBOUND_TEST_DATABASE_URL"
[[ "$base_database_url" = *'/aicrm_test?sslmode=disable' ]]
database_url="${base_database_url%/aicrm_test?sslmode=disable}/aicrm_test_b01_00077?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"

cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_b01_00077 WITH (FORCE)' >/dev/null; }
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_b01_00077' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id=77 AND is_applied)')" = 't' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 76 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c "SELECT to_regclass('public.wecom_contact_inbox') IS NULL")" = 't' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
P4B01_WECOM_INBOUND_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestB01WeComInboundPG16$' ./acceptance/wecom
set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 76 >/tmp/aicrm-b01-inbound-down.out 2>&1
task_rc=$?
set -e
[[ $task_rc -ne 0 ]]
grep -Fq 'cannot roll back populated CH03 entrant facts' /tmp/aicrm-b01-inbound-down.out
printf 'P4 B01 WeCom inbound PG16 acceptance: PASS (77 down/up, callback inbox idempotency, CH03 populated hard-stop, critical local job, no provider effect)\n'
