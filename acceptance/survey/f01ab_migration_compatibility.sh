#!/usr/bin/env bash
set -euo pipefail

: "${P4F01AB_SURVEY_TEST_DATABASE_URL:?P4F01AB_SURVEY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4F01AB_SURVEY_TEST_DATABASE_URL"
temporary_database="aicrm_test_f01ab"
database_url="${base_database_url/aicrm_test/$temporary_database}"

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
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 36
history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),
    (SELECT count(*) FROM questionnaires),
    (SELECT md5(COALESCE(string_agg(row_to_json(q)::text,E'\\n' ORDER BY id),'')) FROM questionnaires q),
    (SELECT count(*) FROM questionnaire_operation_receipts),
    (SELECT md5(COALESCE(string_agg(row_to_json(r)::text,E'\\n' ORDER BY id),'')) FROM questionnaire_operation_receipts r)"
}
baseline="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 37
read -r waterline management_table tenant_columns <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (SELECT (to_regclass('public.questionnaire_management_receipts') IS NOT NULL)::int),
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'questionnaire%' AND column_name ~* 'tenant|workspace|organization')")"
[[ "$waterline" = "37" && "$management_table" = "1" && "$tenant_columns" = "0" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s -run '^TestF01BManagement' ./acceptance/survey -args -database-url "$database_url"
post_acceptance="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 36
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "36" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.questionnaire_management_receipts') IS NULL)::int")" = "1" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 37
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "37" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]
printf 'P4-F01 A+B migration compatibility: PASS (36/37/36/37, Survey/Event history preserved)\n'
