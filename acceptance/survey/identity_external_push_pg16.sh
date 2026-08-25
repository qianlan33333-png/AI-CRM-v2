#!/usr/bin/env bash
set -euo pipefail

: "${P4SURVEY_PUSH_TEST_DATABASE_URL:?P4SURVEY_PUSH_TEST_DATABASE_URL is required}"
base_database_url="$P4SURVEY_PUSH_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
temporary_database='aicrm_test_survey_push_00082'
database_url="${base_database_url%"$database_suffix"}/$temporary_database?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-survey-push-down-guard.XXXXXX")"

MIGRATION_TEST_DATABASE_URL="$base_database_url" MIGRATION_TEST_DATABASE_NAME='aicrm_test' \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  rm -f "$guard_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

goose=("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url")
"${goose[@]}" up-to 82 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=82 AND is_applied')" = '1' ]]

"${goose[@]}" down-to 81 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=82 AND is_applied')" = '0' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT (to_regclass('public.questionnaire_submission_external_push_bindings') IS NULL)::int")" = '1' ]]
"${goose[@]}" up-to 82 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=82 AND is_applied')" = '1' ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestSurveyIdentityExternalPushPG16$' \
  ./acceptance/survey -args -database-url "$database_url"

set +e
"${goose[@]}" down-to 81 >"$guard_output" 2>&1
guard_status=$?
set -e
[[ "$guard_status" -ne 0 ]]
grep -Fq 'cannot roll back populated survey external push facts' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=82 AND is_applied')" = '1' ]]

printf 'P4 Survey identity external-push PG16.14 acceptance: PASS (82 empty down/up, H5 verified identity, fake unknown, manual reconcile, PII-free detail; no provider)\n'
