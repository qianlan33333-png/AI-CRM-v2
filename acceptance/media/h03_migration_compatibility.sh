#!/usr/bin/env bash
set -euo pipefail

: "${P4H03_MEDIA_TEST_DATABASE_URL:?P4H03_MEDIA_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4H03_MEDIA_TEST_DATABASE_URL"
temporary_database="aicrm_test_h03"
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
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 33

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT COALESCE(max(id),0) FROM event_log),
    (SELECT md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),'')) FROM event_log),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM admin_users value),
    (SELECT count(*) FROM media_images),
    (SELECT COALESCE(max(id),0) FROM media_images),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM media_images value)"
}

prefix_snapshot() {
  [[ "$1" =~ ^[0-9]+$ && "$2" =~ ^[0-9]+$ ]]
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log WHERE id <= $1),
    (SELECT md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),'')) FROM event_log WHERE id <= $1),
    (SELECT count(*) FROM media_images WHERE id <= $2),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM media_images value WHERE id <= $2)"
}

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s -run '^TestH03MigrationHistoryFixture$' \
  ./acceptance/media -args -database-url "$database_url"
read -r base_events base_max_event base_event_hash base_admins base_admin_hash base_images base_max_image base_image_hash <<<"$(history_snapshot)"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 34
read -r waterline library receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),
  (to_regclass('public.media_group_invites') IS NOT NULL)::int,
  (to_regclass('public.media_group_invite_operation_receipts') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$waterline" = "34" && "$library" = "1" && "$receipts" = "1" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s -run '^TestH03(CRUD|Four|Concurrent|S200K|Storage)' \
  ./acceptance/media -args -database-url "$database_url"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 33
read -r rollback_waterline library receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT max(version_id),
  (to_regclass('public.media_group_invites') IS NOT NULL)::int,
  (to_regclass('public.media_group_invite_operation_receipts') IS NOT NULL)::int FROM goose_db_version WHERE is_applied")"
[[ "$rollback_waterline" = "33" && "$library" = "0" && "$receipts" = "0" ]]
read -r events max_event event_hash admins admin_hash images max_image image_hash <<<"$(history_snapshot)"
read -r prefix_events prefix_event_hash prefix_images prefix_image_hash <<<"$(prefix_snapshot "$base_max_event" "$base_max_image")"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" && "$admins" = "$base_admins" && "$admin_hash" = "$base_admin_hash" && "$prefix_images" = "$base_images" && "$prefix_image_hash" = "$base_image_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 34
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "34" ]]
read -r prefix_events prefix_event_hash prefix_images prefix_image_hash <<<"$(prefix_snapshot "$base_max_event" "$base_max_image")"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" && "$prefix_images" = "$base_images" && "$prefix_image_hash" = "$base_image_hash" ]]

printf 'P4-H03 migration compatibility: PASS (33/34/33/34, Event/Auth/Media history preserved)\n'
