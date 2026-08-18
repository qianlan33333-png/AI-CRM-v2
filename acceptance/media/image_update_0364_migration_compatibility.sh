#!/usr/bin/env bash
set -euo pipefail

: "${P4IMAGEUPDATE_TEST_DATABASE_URL:?P4IMAGEUPDATE_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4IMAGEUPDATE_TEST_DATABASE_URL"

# The manifest self-test supplies this inert sentinel while substituting its
# executors. It must validate manifest dispatch without starting PostgreSQL.
if [[ "$database_url" = "postgres://fixture" && -n "${CI_ACCEPTANCE_TEST_LOG:-}" ]]; then
  exit 0
fi

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

if [[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 46
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 46
fi

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=120s \
  -run '^TestH01A1UploadReplayActorIsolationAndEventShareOneUoW$' \
  ./acceptance/media -args -database-url "$database_url"

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
	(SELECT COALESCE(max(id),0) FROM event_log),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM event_log value),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM admin_users value),
    (SELECT count(*) FROM media_images),
    (SELECT COALESCE(max(id),0) FROM media_images),
    (SELECT md5(COALESCE(string_agg((to_jsonb(value) - 'enabled')::text,E'\\n' ORDER BY id),'')) FROM media_images value),
    (SELECT count(*) FROM media_image_blobs),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY image_id),'')) FROM media_image_blobs value),
    (SELECT count(*) FROM media_image_upload_receipts),
    (SELECT COALESCE(max(id),0) FROM media_image_upload_receipts),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM media_image_upload_receipts value)"
}

base_history="$(history_snapshot)" || { echo "failed to snapshot pre-47 history" >&2; exit 1; }
read -r base_events base_max_event base_event_hash base_admins base_admin_hash base_images base_max_image base_image_hash base_blobs base_blob_hash base_receipts base_max_receipt base_receipt_hash <<<"$base_history"

prefix_snapshot() {
  [[ "$1" =~ ^[0-9]+$ && "$2" =~ ^[0-9]+$ && "$3" =~ ^[0-9]+$ ]]
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log WHERE id <= $1),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM event_log value WHERE id <= $1),
    (SELECT count(*) FROM media_images WHERE id <= $2),
    (SELECT md5(COALESCE(string_agg((to_jsonb(value) - 'enabled')::text,E'\\n' ORDER BY id),'')) FROM media_images value WHERE id <= $2),
    (SELECT count(*) FROM media_image_blobs WHERE image_id <= $2),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY image_id),'')) FROM media_image_blobs value WHERE image_id <= $2),
    (SELECT count(*) FROM media_image_upload_receipts WHERE id <= $3),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM media_image_upload_receipts value WHERE id <= $3)"
}

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 47
read -r waterline enabled_column old_rows_enabled <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='media_images' AND column_name='enabled'),
  (SELECT count(*) FROM media_images WHERE id <= $base_max_image AND enabled)")"
[[ "$waterline" = "47" && "$enabled_column" = "1" && "$old_rows_enabled" = "$base_images" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s \
  -run '^TestImageUpdate0364PostgreSQL' \
  ./acceptance/media -args -database-url "$database_url"

reset_image_id="$base_max_image"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "UPDATE media_images SET enabled=FALSE WHERE id=$reset_image_id"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 46
read -r rollback_waterline enabled_column <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='media_images' AND column_name='enabled')")"
[[ "$rollback_waterline" = "46" && "$enabled_column" = "0" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 47
read -r final_waterline enabled_column reset_enabled <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='media_images' AND column_name='enabled'),
  (SELECT enabled::int FROM media_images WHERE id=$reset_image_id)")"
[[ "$final_waterline" = "47" && "$enabled_column" = "1" && "$reset_enabled" = "1" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s \
  -run '^TestImageUpdate0364PostgreSQL' \
  ./acceptance/media -args -database-url "$database_url"

prefix_history="$(prefix_snapshot "$base_max_event" "$base_max_image" "$base_max_receipt")" || { echo "failed to snapshot historical prefix" >&2; exit 1; }
read -r prefix_events prefix_event_hash prefix_images prefix_image_hash prefix_blobs prefix_blob_hash prefix_receipts prefix_receipt_hash <<<"$prefix_history"
final_history="$(history_snapshot)" || { echo "failed to snapshot final history" >&2; exit 1; }
read -r final_events _ final_event_hash final_admins final_admin_hash final_images _ final_image_hash final_blobs final_blob_hash final_receipts _ final_receipt_hash <<<"$final_history"
[[ "$final_admins" = "$base_admins" && "$final_admin_hash" = "$base_admin_hash" &&
   "$final_images" -ge "$base_images" && "$final_blobs" -ge "$base_blobs" && "$final_receipts" -ge "$base_receipts" &&
   "$final_events" -ge "$base_events" && "$prefix_events" = "$base_events" && "$prefix_event_hash" = "$base_event_hash" &&
   "$prefix_images" = "$base_images" && "$prefix_image_hash" = "$base_image_hash" &&
   "$prefix_blobs" = "$base_blobs" && "$prefix_blob_hash" = "$base_blob_hash" &&
   "$prefix_receipts" = "$base_receipts" && "$prefix_receipt_hash" = "$base_receipt_hash" &&
   -n "$final_event_hash" && -n "$final_image_hash" && -n "$final_blob_hash" && -n "$final_receipt_hash" ]]

printf 'P4 Image Update migration compatibility: PASS (46/47/46/47, historical Event/Auth/Media facts preserved, enabled reset to true after Down/Up)\n'
