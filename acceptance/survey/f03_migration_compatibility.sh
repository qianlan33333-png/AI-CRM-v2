#!/usr/bin/env bash
set -euo pipefail

: "${P4F03_SURVEY_TEST_DATABASE_URL:?P4F03_SURVEY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4F03_SURVEY_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 45
history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),
    (SELECT count(*) FROM questionnaires),
    (SELECT md5(COALESCE(string_agg(row_to_json(q)::text,E'\\n' ORDER BY id),'')) FROM questionnaires q),
    (SELECT count(*) FROM questionnaire_questions),
    (SELECT count(*) FROM questionnaire_operation_receipts),
    (SELECT count(*) FROM questionnaire_management_receipts)"
}
baseline="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 46
read -r waterline submissions_table answers_table tenant_columns <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (SELECT (to_regclass('public.questionnaire_submissions') IS NOT NULL)::int),
  (SELECT (to_regclass('public.questionnaire_submission_answers') IS NOT NULL)::int),
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'questionnaire_submission%' AND column_name ~* 'tenant|workspace|organization')")"
[[ "$submissions_table" = "1" && "$answers_table" = "1" && "$tenant_columns" = "0" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s -run '^TestF03' ./acceptance/survey -args -database-url "$database_url"
post_acceptance="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 45
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "45" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.questionnaire_submissions') IS NULL)::int")" = "1" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.questionnaire_submission_answers') IS NULL)::int")" = "1" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 46
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "46" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]
printf 'P4-F03 migration compatibility: PASS (45/46/45/46, Survey/Event history preserved)\n'
