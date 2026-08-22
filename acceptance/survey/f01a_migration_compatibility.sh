#!/usr/bin/env bash
set -euo pipefail

: "${P4F01A_SURVEY_TEST_DATABASE_URL:?P4F01A_SURVEY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4F01A_SURVEY_TEST_DATABASE_URL"
temporary_database="aicrm_test_f01a"
database_url="${base_database_url/aicrm_test/$temporary_database}"

# This historical proof owns a separate, freshly created acceptance database;
# it never rolls shared manifest facts back across later migration guards.
MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 30

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT COALESCE(max(id),0) FROM event_log),
    (SELECT md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),'')) FROM event_log),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(u)::text,E'\\n' ORDER BY id),'')) FROM admin_users u),
    (SELECT count(*) FROM products),
    (SELECT md5(COALESCE(string_agg(row_to_json(p)::text,E'\\n' ORDER BY id),'')) FROM products p),
    (SELECT count(*) FROM media_images),
    (SELECT md5(COALESCE(string_agg(row_to_json(m)::text,E'\\n' ORDER BY id),'')) FROM media_images m)"
}

event_prefix_snapshot() {
  [[ "$1" =~ ^[0-9]+$ ]]
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    count(*),md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),''))
    FROM event_log WHERE id <= $1"
}

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s -run '^TestF01AMigrationHistoryFixture$' \
  ./acceptance/survey -args -database-url "$database_url"
read -r base_events base_max_event base_event_hash base_admins base_admin_hash base_products base_product_hash base_media base_media_hash <<<"$(history_snapshot)"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 31
read -r waterline questionnaires questions options counters receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),
  (to_regclass('public.questionnaires') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_questions') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_options') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_catalog_counters') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_operation_receipts') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$waterline" = "31" && "$questionnaires" = "1" && "$questions" = "1" && "$options" = "1" && "$counters" = "1" && "$receipts" = "1" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s -run '^TestF01A(Create|Event|S200K|Storage)' \
  ./acceptance/survey -args -database-url "$database_url"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 30
read -r rollback_waterline questionnaires questions options counters receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),
  (to_regclass('public.questionnaires') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_questions') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_options') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_catalog_counters') IS NOT NULL)::int,
  (to_regclass('public.questionnaire_operation_receipts') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$rollback_waterline" = "30" && "$questionnaires" = "0" && "$questions" = "0" && "$options" = "0" && "$counters" = "0" && "$receipts" = "0" ]]
read -r events max_event event_hash admins admin_hash products product_hash media media_hash <<<"$(history_snapshot)"
read -r prefix_events prefix_event_hash <<<"$(event_prefix_snapshot "$base_max_event")"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" ]]
[[ "$admins" = "$base_admins" && "$admin_hash" = "$base_admin_hash" && "$products" = "$base_products" && "$product_hash" = "$base_product_hash" && "$media" = "$base_media" && "$media_hash" = "$base_media_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 31
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "31" ]]
read -r events max_event event_hash admins admin_hash products product_hash media media_hash <<<"$(history_snapshot)"
read -r prefix_events prefix_event_hash <<<"$(event_prefix_snapshot "$base_max_event")"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" ]]
[[ "$admins" = "$base_admins" && "$admin_hash" = "$base_admin_hash" && "$products" = "$base_products" && "$product_hash" = "$base_product_hash" && "$media" = "$base_media" && "$media_hash" = "$base_media_hash" ]]

printf 'P4-F01A migration compatibility: PASS (30/31/30/31, Event/Auth/Product/Media history preserved)\n'
