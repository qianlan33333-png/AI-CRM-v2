#!/usr/bin/env bash
set -euo pipefail

: "${P4I01A_PRODUCT_TEST_DATABASE_URL:?P4I01A_PRODUCT_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4I01A_PRODUCT_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

has_migration_table="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
  "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")"
if [[ "$has_migration_table" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 28
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 28
fi

auth_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT
       (SELECT count(*) FROM admin_users),
       (SELECT md5(COALESCE(string_agg(row_to_json(u)::text, E'\\n' ORDER BY id), '')) FROM admin_users u),
       (SELECT count(*) FROM admin_sessions),
       (SELECT md5(COALESCE(string_agg(row_to_json(s)::text, E'\\n' ORDER BY id), '')) FROM admin_sessions s)"
}

read -r baseline_admin_count baseline_admin_hash baseline_session_count baseline_session_hash <<<"$(auth_snapshot)"
[[ "$baseline_admin_count" =~ ^[0-9]+$ && "$baseline_admin_hash" =~ ^[0-9a-f]{32}$ ]]
[[ "$baseline_session_count" =~ ^[0-9]+$ && "$baseline_session_hash" =~ ^[0-9a-f]{32}$ ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s -run '^TestI01AMigrationHistoryFixture$' \
  ./acceptance/product -args -database-url "$database_url"

read -r history_id history_key <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT id,idempotency_key FROM event_log
      WHERE event_type='i01a.product.migration_fixture' ORDER BY id DESC LIMIT 1"
)"
[[ "$history_id" =~ ^[1-9][0-9]*$ && "$history_key" = i01a-migration-history-* ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 29
read -r upgrade_waterline products images counters receipts history_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (to_regclass('public.products') IS NOT NULL)::int,
            (to_regclass('public.product_images') IS NOT NULL)::int,
            (to_regclass('public.product_catalog_counters') IS NOT NULL)::int,
            (to_regclass('public.product_operation_receipts') IS NOT NULL)::int,
            (SELECT count(*) FROM event_log WHERE id=${history_id} AND idempotency_key='${history_key}')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$upgrade_waterline" = "29" && "$products" = "1" && "$images" = "1" && "$counters" = "1" && "$receipts" = "1" && "$history_count" = "1" ]]
read -r admin_count admin_hash session_count session_hash <<<"$(auth_snapshot)"
[[ "$admin_count" = "$baseline_admin_count" && "$admin_hash" = "$baseline_admin_hash" ]]
[[ "$session_count" = "$baseline_session_count" && "$session_hash" = "$baseline_session_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 28
read -r rollback_waterline products images counters receipts history_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (to_regclass('public.products') IS NOT NULL)::int,
            (to_regclass('public.product_images') IS NOT NULL)::int,
            (to_regclass('public.product_catalog_counters') IS NOT NULL)::int,
            (to_regclass('public.product_operation_receipts') IS NOT NULL)::int,
            (SELECT count(*) FROM event_log WHERE id=${history_id} AND idempotency_key='${history_key}')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$rollback_waterline" = "28" && "$products" = "0" && "$images" = "0" && "$counters" = "0" && "$receipts" = "0" ]]
[[ "$history_count" = "1" ]]
read -r admin_count admin_hash session_count session_hash <<<"$(auth_snapshot)"
[[ "$admin_count" = "$baseline_admin_count" && "$admin_hash" = "$baseline_admin_hash" ]]
[[ "$session_count" = "$baseline_session_count" && "$session_hash" = "$baseline_session_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 29
read -r final_waterline products images counters receipts history_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (to_regclass('public.products') IS NOT NULL)::int,
            (to_regclass('public.product_images') IS NOT NULL)::int,
            (to_regclass('public.product_catalog_counters') IS NOT NULL)::int,
            (to_regclass('public.product_operation_receipts') IS NOT NULL)::int,
            (SELECT count(*) FROM event_log WHERE id=${history_id} AND idempotency_key='${history_key}')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$final_waterline" = "29" && "$products" = "1" && "$images" = "1" && "$counters" = "1" && "$receipts" = "1" && "$history_count" = "1" ]]
read -r admin_count admin_hash session_count session_hash <<<"$(auth_snapshot)"
[[ "$admin_count" = "$baseline_admin_count" && "$admin_hash" = "$baseline_admin_hash" ]]
[[ "$session_count" = "$baseline_session_count" && "$session_hash" = "$baseline_session_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 50
read -r current_waterline version_column entitlements entitlement_receipts history_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='products' AND column_name='version'),
            (to_regclass('public.product_local_entitlements') IS NOT NULL)::int,
            (to_regclass('public.entitlement_operation_receipts') IS NOT NULL)::int,
            (SELECT count(*) FROM event_log WHERE id=${history_id} AND idempotency_key='${history_key}')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$current_waterline" = "50" && "$version_column" = "1" && "$entitlements" = "1" && "$entitlement_receipts" = "1" && "$history_count" = "1" ]]
read -r admin_count admin_hash session_count session_hash <<<"$(auth_snapshot)"
[[ "$admin_count" = "$baseline_admin_count" && "$admin_hash" = "$baseline_admin_hash" ]]
[[ "$session_count" = "$baseline_session_count" && "$session_hash" = "$baseline_session_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 49
read -r rollback_current_waterline version_column entitlements entitlement_receipts history_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='products' AND column_name='version'),
            (to_regclass('public.product_local_entitlements') IS NOT NULL)::int,
            (to_regclass('public.entitlement_operation_receipts') IS NOT NULL)::int,
            (SELECT count(*) FROM event_log WHERE id=${history_id} AND idempotency_key='${history_key}')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$rollback_current_waterline" = "49" && "$version_column" = "0" && "$entitlements" = "0" && "$entitlement_receipts" = "0" && "$history_count" = "1" ]]
read -r admin_count admin_hash session_count session_hash <<<"$(auth_snapshot)"
[[ "$admin_count" = "$baseline_admin_count" && "$admin_hash" = "$baseline_admin_hash" ]]
[[ "$session_count" = "$baseline_session_count" && "$session_hash" = "$baseline_session_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 50
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "50" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s \
  -run '^(TestI01A(Create|Event|S200K|Storage).*|TestI01BProductCASAndLocalEntitlementLifecycleUseOneUoW)$' \
  ./acceptance/product -args -database-url "$database_url"

printf 'P4-I01A/I01B migration compatibility: PASS (28/29/28/29/50/49/50, Event/Auth/session history preserved)\n'
