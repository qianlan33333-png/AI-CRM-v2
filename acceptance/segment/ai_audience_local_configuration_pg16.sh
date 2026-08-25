#!/usr/bin/env bash
set -euo pipefail

: "${P4AIAUDIENCE_TEST_DATABASE_URL:?P4AIAUDIENCE_TEST_DATABASE_URL is required}"

base_database_url="$P4AIAUDIENCE_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
database_name='aicrm_test_ai_audience_00084'
database_url="${base_database_url%"$database_suffix"}/$database_name?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-ai-audience-00084-down.XXXXXX")"

MIGRATION_TEST_DATABASE_URL="$base_database_url" MIGRATION_TEST_DATABASE_NAME='aicrm_test' \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  rm -f "$guard_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $database_name WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $database_name" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$database_name" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

goose=("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url")
"${goose[@]}" up-to 84 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 84 AND is_applied')" = '1' ]]

CI_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestLocalConfigurationSQLRepositoryPG16' ./internal/segment/legacyaudience

"${goose[@]}" down-to 83 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 84 AND is_applied')" = '0' ]]
"${goose[@]}" up-to 84 >/dev/null

psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "
  INSERT INTO public.ai_audience_local_configuration_receipts
    (operation, actor_id, key_digest, payload_digest, state, result_json, created_at, completed_at)
  VALUES
    ('configuration_version_put', 1, decode(repeat('aa', 32), 'hex'), decode(repeat('bb', 32), 'hex'),
     'completed', '{}'::jsonb, now(), now())"

if "${goose[@]}" down-to 83 >"$guard_output" 2>&1; then
  printf 'expected populated AI Audience 00084 down guard to fail\n' >&2
  exit 1
fi
rg -q 'cannot roll back populated AI Audience local configuration closure facts' "$guard_output"
rg -q 'SQLSTATE 55000' "$guard_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 84 AND is_applied')" = '1' ]]

printf 'P4 AI Audience local configuration PG16.14: PASS (exact 84, empty 84/83/84, repository concurrency, populated receipt rollback guard; no provider)\n'
