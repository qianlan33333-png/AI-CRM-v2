#!/usr/bin/env bash
set -euo pipefail

: "${P4MINIPROGRAMLIBRARY_TEST_DATABASE_URL:?P4MINIPROGRAMLIBRARY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4MINIPROGRAMLIBRARY_TEST_DATABASE_URL"
temporary_database="aicrm_test_miniprogram_library"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null; }
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 44

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT COALESCE(max(id),0) FROM event_log),
    (SELECT md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),'')) FROM event_log),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM admin_users value),
    (SELECT count(*) FROM media_images),
    (SELECT COALESCE(max(id),0) FROM media_images),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM media_images value),
    (SELECT count(*) FROM media_image_blobs),
    (SELECT md5(COALESCE(string_agg(image_id::text||':'||encode(checksum,'hex'),E'\\n' ORDER BY image_id),'')) FROM media_image_blobs),
    (SELECT count(*) FROM media_image_upload_receipts),
    (SELECT COALESCE(max(id),0) FROM media_image_upload_receipts),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM media_image_upload_receipts value)"
}

prefix_snapshot() {
  [[ "$1" =~ ^[0-9]+$ && "$2" =~ ^[0-9]+$ && "$3" =~ ^[0-9]+$ ]]
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log WHERE id <= $1),
    (SELECT md5(COALESCE(string_agg(id::text||':'||idempotency_key,E'\\n' ORDER BY id),'')) FROM event_log WHERE id <= $1),
    (SELECT count(*) FROM media_images WHERE id <= $2),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM media_images value WHERE id <= $2),
    (SELECT count(*) FROM media_image_blobs WHERE image_id <= $2),
    (SELECT md5(COALESCE(string_agg(image_id::text||':'||encode(checksum,'hex'),E'\\n' ORDER BY image_id),'')) FROM media_image_blobs WHERE image_id <= $2),
    (SELECT count(*) FROM media_image_upload_receipts WHERE id <= $3),
    (SELECT md5(COALESCE(string_agg(row_to_json(value)::text,E'\\n' ORDER BY id),'')) FROM media_image_upload_receipts value WHERE id <= $3)"
}

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=120s \
  -run '^TestH01A1(MigrationHistoryFixture|UploadReplayActorIsolationAndEventShareOneUoW)$' \
  ./acceptance/media -args -database-url "$database_url"

read -r base_events base_max_event base_event_hash base_admins base_admin_hash \
  base_images base_max_image base_image_hash base_blobs base_blob_hash \
  base_upload_receipts base_max_upload_receipt base_upload_receipt_hash <<<"$(history_snapshot)"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 45
read -r waterline cache_table library_table receipt_table preflight_table ledger_table thumbnail_fk_index imported_rows <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    max(version_id),
    (to_regclass('public.media_thumbnail_cache_entries') IS NOT NULL)::int,
    (to_regclass('public.media_miniprograms') IS NOT NULL)::int,
    (to_regclass('public.media_miniprogram_operation_receipts') IS NOT NULL)::int,
    (to_regclass('public.media_miniprogram_import_preflights') IS NOT NULL)::int,
    (to_regclass('public.media_miniprogram_import_ledger') IS NOT NULL)::int,
    (to_regclass('public.media_miniprograms_thumbnail_image_id_idx') IS NOT NULL)::int,
    (SELECT count(*) FROM media_miniprogram_import_preflights) + (SELECT count(*) FROM media_miniprogram_import_ledger)
    FROM goose_db_version WHERE is_applied GROUP BY 2,3,4,5,6,7"
)"
[[ "$cache_table" = "1" && "$library_table" = "1" && "$receipt_table" = "1" && "$preflight_table" = "1" && "$ledger_table" = "1" && "$thumbnail_fk_index" = "1" && "$imported_rows" = "0" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s -run '^TestMiniProgramR1' \
  ./acceptance/media -args -database-url "$database_url"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 44
read -r rollback_waterline cache_table library_table receipt_table preflight_table ledger_table <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    max(version_id),
    (to_regclass('public.media_thumbnail_cache_entries') IS NOT NULL)::int,
    (to_regclass('public.media_miniprograms') IS NOT NULL)::int,
    (to_regclass('public.media_miniprogram_operation_receipts') IS NOT NULL)::int,
    (to_regclass('public.media_miniprogram_import_preflights') IS NOT NULL)::int,
    (to_regclass('public.media_miniprogram_import_ledger') IS NOT NULL)::int
    FROM goose_db_version WHERE is_applied GROUP BY 2,3,4,5,6"
)"
[[ "$rollback_waterline" = "44" && "$cache_table" = "0" && "$library_table" = "0" && "$receipt_table" = "0" && "$preflight_table" = "0" && "$ledger_table" = "0" ]]

read -r prefix_events prefix_event_hash prefix_images prefix_image_hash prefix_blobs prefix_blob_hash \
  prefix_upload_receipts prefix_upload_receipt_hash <<<"$(prefix_snapshot "$base_max_event" "$base_max_image" "$base_max_upload_receipt")"
read -r _ _ _ admins admin_hash _ _ _ _ _ _ _ _ <<<"$(history_snapshot)"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" &&
   "$admins" = "$base_admins" && "$admin_hash" = "$base_admin_hash" &&
   "$prefix_images" = "$base_images" && "$prefix_image_hash" = "$base_image_hash" &&
   "$prefix_blobs" = "$base_blobs" && "$prefix_blob_hash" = "$base_blob_hash" &&
   "$prefix_upload_receipts" = "$base_upload_receipts" && "$prefix_upload_receipt_hash" = "$base_upload_receipt_hash" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 45
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "45" ]]
read -r prefix_events prefix_event_hash prefix_images prefix_image_hash prefix_blobs prefix_blob_hash \
  prefix_upload_receipts prefix_upload_receipt_hash <<<"$(prefix_snapshot "$base_max_event" "$base_max_image" "$base_max_upload_receipt")"
[[ "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" &&
   "$prefix_images" = "$base_images" && "$prefix_image_hash" = "$base_image_hash" &&
   "$prefix_blobs" = "$base_blobs" && "$prefix_blob_hash" = "$base_blob_hash" &&
   "$prefix_upload_receipts" = "$base_upload_receipts" && "$prefix_upload_receipt_hash" = "$base_upload_receipt_hash" ]]

printf 'P4 Mini Program Library migration compatibility: PASS (44/45/44/45, Event/Auth/Media history preserved, historical import not executed)\n'
