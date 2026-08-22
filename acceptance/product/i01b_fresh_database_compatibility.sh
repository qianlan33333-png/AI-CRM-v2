#!/usr/bin/env bash
set -euo pipefail

: "${P4I01B_PRODUCT_TEST_DATABASE_URL:?P4I01B_PRODUCT_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4I01B_PRODUCT_TEST_DATABASE_URL"
temporary_database="aicrm_test_i01b"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s \
  -run '^TestI01BProductCASAndLocalEntitlementLifecycleUseOneUoW$' ./acceptance/product \
  -args -database-url "$database_url"
