#!/usr/bin/env bash
set -euo pipefail

: "${P4OUTBOUNDCAMPAIGNHANDOFF_TEST_DATABASE_URL:?P4OUTBOUNDCAMPAIGNHANDOFF_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4OUTBOUNDCAMPAIGNHANDOFF_TEST_DATABASE_URL"
temporary_database="aicrm_test_p4_outbound_00068"
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
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestCampaignHandoff' ./acceptance/outbound \
  -args -campaign-handoff-database-url "$database_url"

printf 'P4 Outbound Campaign Handoff acceptance: PASS (00001/00068, 67/68 empty down/up, facts guard)\n'
