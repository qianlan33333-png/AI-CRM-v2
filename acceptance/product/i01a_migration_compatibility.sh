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

protected_product_id="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
  "INSERT INTO products (
     product_code, name, description, price_minor, currency, stock_quantity,
     created_by, created_at, updated_at, legacy_admin_projection
   ) VALUES (
     'i01a-rollback-probe', 'I01A rollback probe', '', 0, 'CNY', 0,
     1, now(), now(), '{\"schema_version\":1}'::jsonb
   ) RETURNING id")"
[[ "$protected_product_id" =~ ^[1-9][0-9]*$ ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "UPDATE products SET version=2 WHERE id=${protected_product_id} AND version=1" >/dev/null
if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 49; then
  printf 'migration 00050 unexpectedly accepted a versioned product fact\n' >&2
  exit 1
fi
read -r protected_waterline version_column protected_product_version <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='products' AND column_name='version'),
            (SELECT version FROM products WHERE id=${protected_product_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$protected_waterline" = "50" && "$version_column" = "1" && "$protected_product_version" = "2" ]]

# Restore only this reversible compatibility marker. The real I01B lifecycle
# facts run as the final database acceptance entry, after every historical
# down-migration check, and are never deleted to make an old schema fit.
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "UPDATE products SET version=1 WHERE id=${protected_product_id} AND version=2" >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 49
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 50
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "DELETE FROM products WHERE id=${protected_product_id}" >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 58
read -r final_product_waterline local_lifecycle_column lifecycle_constraint receipt_constraint <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='products' AND column_name='local_lifecycle'),
            (SELECT count(*) FROM pg_constraint
              WHERE conrelid='public.products'::regclass AND conname='products_local_lifecycle'),
            (SELECT count(*) FROM pg_constraint
              WHERE conrelid='public.product_operation_receipts'::regclass AND conname='product_operation_receipts_operation')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$final_product_waterline" = "58" && "$local_lifecycle_column" = "1" && "$lifecycle_constraint" = "1" && "$receipt_constraint" = "1" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s \
  -run '^TestI01A(Create|Event|S200K|Storage).*' \
  ./acceptance/product -args -database-url "$database_url"

printf 'P4-I01A migration compatibility: PASS (28/29/28/29/50/49/50/58, versioned facts make rollback fail closed, Event/Auth/session history preserved)\n'
