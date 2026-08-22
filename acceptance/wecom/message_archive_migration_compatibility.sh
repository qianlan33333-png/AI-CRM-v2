#!/usr/bin/env bash
set -euo pipefail

: "${P4MESSAGEARCHIVE_TEST_DATABASE_URL:?P4MESSAGEARCHIVE_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4MESSAGEARCHIVE_TEST_DATABASE_URL"
temporary_database="aicrm_test_message_archive"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null; }
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 37
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 39

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),
    (SELECT count(*) FROM customers),
    (SELECT md5(COALESCE(string_agg(row_to_json(c)::text,E'\\n' ORDER BY id),'')) FROM customers c),
    (SELECT count(*) FROM identities),
    (SELECT md5(COALESCE(string_agg(row_to_json(i)::text,E'\\n' ORDER BY id),'')) FROM identities i)"
}

baseline="$(history_snapshot)"
[[ "$baseline" =~ ^[0-9]+\ [0-9a-f]{32}\ [0-9]+\ [0-9a-f]{32}\ [0-9]+\ [0-9a-f]{32}$ ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 40
read -r waterline records receipts constraints invalid_constraints indexes invalid_indexes foreign_keys tenant_columns content_masked <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.wecom_message_archive_records') IS NOT NULL)::int,
  (to_regclass('public.wecom_message_archive_sync_receipts') IS NOT NULL)::int,
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass)),
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass) AND NOT convalidated),
  (SELECT count(*) FROM pg_index WHERE indrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass)),
  (SELECT count(*) FROM pg_index WHERE indrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('wecom_message_archive_records'::regclass,'wecom_message_archive_sync_receipts'::regclass) AND contype='f'),
  (SELECT count(*) FROM information_schema.columns WHERE table_name IN ('wecom_message_archive_records','wecom_message_archive_sync_receipts') AND column_name ILIKE '%tenant%'),
  (SELECT count(*) FROM information_schema.columns WHERE table_name='wecom_message_archive_records' AND column_name='content_masked')")"
[[ "$waterline" = "40" && "$records" = "1" && "$receipts" = "1" && "$constraints" -ge 15 && "$invalid_constraints" = "0" && "$indexes" -ge 4 && "$invalid_indexes" = "0" && "$foreign_keys" = "0" && "$tenant_columns" = "0" && "$content_masked" = "1" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestP4MessageArchive' ./acceptance/wecom -args -p4-message-archive-database-url "$database_url"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 39
read -r waterline records receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.wecom_message_archive_records') IS NOT NULL)::int,
  (to_regclass('public.wecom_message_archive_sync_receipts') IS NOT NULL)::int")"
[[ "$waterline" = "39" && "$records" = "0" && "$receipts" = "0" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 40
read -r waterline records receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.wecom_message_archive_records') IS NOT NULL)::int,
  (to_regclass('public.wecom_message_archive_sync_receipts') IS NOT NULL)::int")"
[[ "$waterline" = "40" && "$records" = "1" && "$receipts" = "1" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

printf 'P4 message archive migration compatibility: PASS (39/40/39/40, Event/Customer/Identity history preserved, no tenant or cross-domain FK)\n'
