#!/usr/bin/env bash
set -euo pipefail

: "${P4OUTBOUNDCAMPAIGNDISPATCH_TEST_DATABASE_URL:?P4OUTBOUNDCAMPAIGNDISPATCH_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4OUTBOUNDCAMPAIGNDISPATCH_TEST_DATABASE_URL"
temporary_database="aicrm_test_p4_outbound_00078"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null

MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '78' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '77' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestCampaignDispatchPG16' ./acceptance/outbound \
  -args -campaign-dispatch-database-url "$database_url"
set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/tmp/aicrm-c01-dispatch-down.out 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back populated outbound campaign external dispatch facts' /tmp/aicrm-c01-dispatch-down.out
printf 'P4 C01 Outbound dispatch PG16 acceptance: PASS (78 down/up, fake receipt, unknown/manual reconcile; no delivery proof)\n'
