#!/usr/bin/env bash
set -euo pipefail

: "${P4EXTERNALPUSH_TEST_DATABASE_URL:?P4EXTERNALPUSH_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4EXTERNALPUSH_TEST_DATABASE_URL"
temporary_database="aicrm_test_i01b"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 87
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "87" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.product_external_push_configurations') IS NOT NULL)::int, (to_regclass('public.product_external_push_test_bindings') IS NOT NULL)::int")" = $'1|1' ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 80
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "80" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.product_external_push_configurations') IS NULL)::int, (to_regclass('public.product_external_push_test_bindings') IS NULL)::int")" = $'1|1' ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 87
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "87" ]]
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=120s -run '^TestCommerceExternalPushPG16RoundTrip$' \
  ./acceptance/product -args -database-url "$database_url"

if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 80; then
  printf 'migration 00087 unexpectedly rolled back populated product external-push facts\n' >&2
  exit 1
fi
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "87" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.product_external_push_configurations') IS NOT NULL)::int, (to_regclass('public.product_external_push_test_bindings') IS NOT NULL)::int")" = $'1|1' ]]

printf 'P4 Commerce External Push 00087 PG16.14: PASS (exact 87, empty down/up, populated guard, local config readback, accepted-only EER)\n'
