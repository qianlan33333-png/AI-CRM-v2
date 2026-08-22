#!/usr/bin/env bash
set -euo pipefail

: "${P4H01A1_MEDIA_TEST_DATABASE_URL:?P4H01A1_MEDIA_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4H01A1_MEDIA_TEST_DATABASE_URL"
temporary_database="aicrm_test_h01a1"
database_url="${base_database_url/aicrm_test/$temporary_database}"

# This historical proof owns a separate, freshly created acceptance database;
# it never resets the shared manifest database or its local-lifecycle facts.
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
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 29

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT COALESCE(max(id),0) FROM event_log),
    (SELECT md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),'')) FROM event_log),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(u)::text,E'\\n' ORDER BY id),'')) FROM admin_users u),
    (SELECT count(*) FROM products),
    (SELECT md5(COALESCE(string_agg(row_to_json(p)::text,E'\\n' ORDER BY id),'')) FROM products p)"
}

event_prefix_snapshot() {
	[[ "$1" =~ ^[0-9]+$ ]]
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    count(*),md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),''))
    FROM event_log WHERE id <= $1"
}

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s -run '^TestH01A1MigrationHistoryFixture$' \
  ./acceptance/media -args -database-url "$database_url"
read -r base_events base_max_event base_event_hash base_admins base_admin_hash base_products base_product_hash <<<"$(history_snapshot)"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 30
read -r waterline images blobs receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),
  (to_regclass('public.media_images') IS NOT NULL)::int,
  (to_regclass('public.media_image_blobs') IS NOT NULL)::int,
  (to_regclass('public.media_image_upload_receipts') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$waterline" = "30" && "$images" = "1" && "$blobs" = "1" && "$receipts" = "1" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s -run '^TestH01A1(Upload|Event|S200K|Storage)' \
  ./acceptance/media -args -database-url "$database_url"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 29
read -r rollback_waterline images blobs receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),
  (to_regclass('public.media_images') IS NOT NULL)::int,
  (to_regclass('public.media_image_blobs') IS NOT NULL)::int,
  (to_regclass('public.media_image_upload_receipts') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$rollback_waterline" = "29" && "$images" = "0" && "$blobs" = "0" && "$receipts" = "0" ]]
read -r events max_event event_hash admins admin_hash products product_hash <<<"$(history_snapshot)"
read -r prefix_events prefix_event_hash <<<"$(event_prefix_snapshot "$base_max_event")"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" && "$admins" = "$base_admins" && "$admin_hash" = "$base_admin_hash" && "$products" = "$base_products" && "$product_hash" = "$base_product_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 30
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "30" ]]
read -r events max_event event_hash admins admin_hash products product_hash <<<"$(history_snapshot)"
read -r prefix_events prefix_event_hash <<<"$(event_prefix_snapshot "$base_max_event")"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" && "$admins" = "$base_admins" && "$admin_hash" = "$base_admin_hash" && "$products" = "$base_products" && "$product_hash" = "$base_product_hash" ]]

printf 'P4-H01A1 migration compatibility: PASS (29/30/29/30, Event/Auth/Product history preserved)\n'
