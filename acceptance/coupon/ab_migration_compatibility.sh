#!/usr/bin/env bash
set -euo pipefail

: "${P4COUPONAB_TEST_DATABASE_URL:?P4COUPONAB_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4COUPONAB_TEST_DATABASE_URL"
temporary_database="aicrm_test_coupon_ab"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null; }
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 35

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(u)::text,E'\\n' ORDER BY id),'')) FROM admin_users u),
    (SELECT count(*) FROM admin_sessions),
    (SELECT md5(COALESCE(string_agg(row_to_json(s)::text,E'\\n' ORDER BY id),'')) FROM admin_sessions s),
    (SELECT count(*) FROM products),
    (SELECT md5(COALESCE(string_agg(row_to_json(p)::text,E'\\n' ORDER BY id),'')) FROM products p),
    (SELECT count(*) FROM coupons),
    (SELECT md5(COALESCE(string_agg(row_to_json(c)::text,E'\\n' ORDER BY id),'')) FROM coupons c),
    (SELECT count(*) FROM coupon_targets),
    (SELECT md5(COALESCE(string_agg(row_to_json(t)::text,E'\\n' ORDER BY coupon_id, target_ref),'')) FROM coupon_targets t),
    (SELECT count(*) FROM coupon_operation_receipts),
    (SELECT md5(COALESCE(string_agg(row_to_json(r)::text,E'\\n' ORDER BY id),'')) FROM coupon_operation_receipts r)"
}

baseline="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 36
read -r waterline claims payment_sessions sidebar_grants target_index customer_fks <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.coupon_claims') IS NOT NULL)::int,
  (to_regclass('public.coupon_payment_identity_sessions') IS NOT NULL)::int,
  (to_regclass('public.coupon_sidebar_grants') IS NOT NULL)::int,
  (to_regclass('public.coupon_targets_target_ref_coupon_id') IS NOT NULL)::int,
  (SELECT count(*) FROM pg_constraint WHERE conrelid='coupon_claims'::regclass AND contype='f' AND confrelid='customers'::regclass)")"
[[ "$waterline" = "36" && "$claims" = "1" && "$payment_sessions" = "1" && "$sidebar_grants" = "1" && "$target_index" = "1" && "$customer_fks" = "0" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 35
read -r waterline claims payment_sessions sidebar_grants target_index <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.coupon_claims') IS NOT NULL)::int,
  (to_regclass('public.coupon_payment_identity_sessions') IS NOT NULL)::int,
  (to_regclass('public.coupon_sidebar_grants') IS NOT NULL)::int,
  (to_regclass('public.coupon_targets_target_ref_coupon_id') IS NOT NULL)::int")"
[[ "$waterline" = "35" && "$claims" = "0" && "$payment_sessions" = "0" && "$sidebar_grants" = "0" && "$target_index" = "0" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 36
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "36" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

# The application acceptance uses the current Product repository, whose closed
# projection includes columns added after the historical Coupon A+B migration.
# Restore the database to the repository's latest waterline before exercising
# current application code; the 35/36 rollback contract has already been
# verified above without mutating its preserved history.
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=360s -run '^TestP4CouponAB' ./acceptance/coupon -args -database-url "$database_url"

printf 'P4 Coupon A+B migration compatibility: PASS (35/36/35/36 history preserved; current-waterline application acceptance passed; customer identity has no cross-domain FK)\n'
