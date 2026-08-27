#!/usr/bin/env bash
set -euo pipefail

: "${P4I03_ORDER_TEST_DATABASE_URL:?P4I03_ORDER_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4I03_ORDER_TEST_DATABASE_URL"
temporary_database="aicrm_test_i03_order"
database_url="${base_database_url/aicrm_test/$temporary_database}"
history_migrations="$(mktemp -d)"
cp migrations/00109_order_v1_history.sql "$history_migrations/"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null; rm -f "$history_migrations/00109_order_v1_history.sql"; rmdir "$history_migrations"; }
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 34

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(u)::text,E'\\n' ORDER BY id),'')) FROM admin_users u),
    (SELECT count(*) FROM admin_sessions),
    (SELECT md5(COALESCE(string_agg(row_to_json(s)::text,E'\\n' ORDER BY id),'')) FROM admin_sessions s)"
}

baseline="$(history_snapshot)"
[[ "$baseline" =~ ^[0-9]+\ [0-9a-f]{32}\ [0-9]+\ [0-9a-f]{32}\ [0-9]+\ [0-9a-f]{32}$ ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 36
read -r waterline projections counters functions cross_fks <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  max(version_id),
  (to_regclass('public.order_list_projections') IS NOT NULL)::int,
  (to_regclass('public.order_list_projection_counters') IS NOT NULL)::int,
  ((to_regprocedure('public.aicrm_order_list_projection_count_insert()') IS NOT NULL AND to_regprocedure('public.aicrm_order_list_projection_count_delete()') IS NOT NULL))::int,
  (SELECT count(*) FROM pg_constraint WHERE conrelid='order_list_projections'::regclass AND contype='f')
  FROM goose_db_version WHERE is_applied")"
[[ "$waterline" = "36" && "$projections" = "1" && "$counters" = "1" && "$functions" = "1" && "$cross_fks" = "0" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

# Current order queries require 109; apply only this Order-owned additive
# migration so the original 34/36 history round-trip remains independently tested.
"$go_command" tool -modfile="$tools_mod" goose -dir "$history_migrations" postgres "$database_url" up
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=360s -run '^TestI03' ./acceptance/order -args -database-url "$database_url"
"$go_command" tool -modfile="$tools_mod" goose -dir "$history_migrations" postgres "$database_url" down

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 34
read -r waterline projections counters <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),(to_regclass('public.order_list_projections') IS NOT NULL)::int,(to_regclass('public.order_list_projection_counters') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$waterline" = "34" && "$projections" = "0" && "$counters" = "0" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 36
read -r waterline projections counters <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),(to_regclass('public.order_list_projections') IS NOT NULL)::int,(to_regclass('public.order_list_projection_counters') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$waterline" = "36" && "$projections" = "1" && "$counters" = "1" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

printf 'P4-I03 migration compatibility: PASS (34/36/34/36, Event/Auth/session history preserved, no cross-domain FK)\n'
