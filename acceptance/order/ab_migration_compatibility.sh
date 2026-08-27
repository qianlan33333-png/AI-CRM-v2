#!/usr/bin/env bash
set -euo pipefail

: "${P4ORDERAB_TEST_DATABASE_URL:?P4ORDERAB_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4ORDERAB_TEST_DATABASE_URL"
temporary_database="aicrm_test_order_ab"
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
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 38

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(u)::text,E'\\n' ORDER BY id),'')) FROM admin_users u),
    (SELECT count(*) FROM admin_sessions),
    (SELECT md5(COALESCE(string_agg(row_to_json(s)::text,E'\\n' ORDER BY id),'')) FROM admin_sessions s),
    (SELECT count(*) FROM order_list_projections),
    (SELECT md5(COALESCE(string_agg(row_to_json(o)::text,E'\\n' ORDER BY id),'')) FROM order_list_projections o)"
}

baseline="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 40
read -r waterline receipts exports effects refunds no_auto_retry outcome_unknown_fk <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.order_operation_receipts') IS NOT NULL)::int,
  (to_regclass('public.order_export_jobs') IS NOT NULL)::int,
  (to_regclass('public.order_external_effects') IS NOT NULL)::int,
  (to_regclass('public.order_refunds') IS NOT NULL)::int,
  (SELECT count(*) FROM pg_constraint WHERE conrelid='order_external_effects'::regclass AND conname='order_external_effects_no_auto_retry'),
  (SELECT count(*) FROM pg_constraint WHERE conrelid='order_external_effects'::regclass AND contype='f' AND confrelid='order_list_projections'::regclass)")"
[[ "$waterline" = "40" && "$receipts" = "1" && "$exports" = "1" && "$effects" = "1" && "$refunds" = "1" && "$no_auto_retry" = "1" && "$outcome_unknown_fk" = "1" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

# Keep the original order-history fingerprints at waterline 40 while testing
# the current query contract with the isolated Order-owned 109 addition.
"$go_command" tool -modfile="$tools_mod" goose -dir "$history_migrations" postgres "$database_url" up
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=360s -run '^TestP4OrderAB' ./acceptance/order -args -database-url "$database_url"
"$go_command" tool -modfile="$tools_mod" goose -dir "$history_migrations" postgres "$database_url" down

post_acceptance="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 38
read -r waterline receipts exports effects refunds <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.order_operation_receipts') IS NOT NULL)::int,
  (to_regclass('public.order_export_jobs') IS NOT NULL)::int,
  (to_regclass('public.order_external_effects') IS NOT NULL)::int,
  (to_regclass('public.order_refunds') IS NOT NULL)::int")"
[[ "$waterline" = "38" && "$receipts" = "0" && "$exports" = "0" && "$effects" = "0" && "$refunds" = "0" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 40
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "40" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 42
current_waterline="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")"
[[ "$current_waterline" = "42" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]

printf 'P4 Order A+B migration compatibility: PASS (38/40/38/40/42, Event/Auth/session/order history preserved, no provider execution)\n'
