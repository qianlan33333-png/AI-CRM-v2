#!/usr/bin/env bash
set -euo pipefail

: "${P4AIAUDIENCEOPMEMBER_TEST_DATABASE_URL:?P4AIAUDIENCEOPMEMBER_TEST_DATABASE_URL is required}"

base_database_url="$P4AIAUDIENCEOPMEMBER_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
database_name='aicrm_test_ai_audience_00100'
database_url="${base_database_url%"$database_suffix"}/$database_name?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-ai-audience-00100-down.XXXXXX")"

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
"${goose[@]}" up-to 100 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 100 AND is_applied')" = '1' ]]

CI_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestAIAudienceOperationMemberSyncPG16$' ./acceptance/segment

psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE $database_name WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $database_name" >/dev/null
"${goose[@]}" up-to 100 >/dev/null
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "
  INSERT INTO public.ai_audience_operation_member_projection
    (sender_userid, display_name, synced_at)
  VALUES ('guard-user', 'Guard User', now())"
if "${goose[@]}" down-to 99 >"$guard_output" 2>&1; then
  printf 'expected populated Audience operation-member projection guard to fail\n' >&2
  exit 1
fi
grep -Fq 'cannot roll back populated AI Audience operation-member projection facts' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"

psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE $database_name WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $database_name" >/dev/null
"${goose[@]}" up-to 100 >/dev/null
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "
  INSERT INTO public.ai_audience_local_configuration_receipts
    (operation, actor_id, key_digest, payload_digest, state, result_json, created_at, completed_at)
  VALUES
    ('operation_members_sync', 1, decode(repeat('aa', 32), 'hex'), decode(repeat('bb', 32), 'hex'),
     'completed', jsonb_build_object('provider_read_executed', true), now(), now())"
if "${goose[@]}" down-to 99 >"$guard_output" 2>&1; then
  printf 'expected populated Audience operation-member receipt guard to fail\n' >&2
  exit 1
fi
grep -Fq 'cannot roll back populated AI Audience operation-member projection facts' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"

printf 'P4 AI Audience operation-member PG16.14: PASS (exact 100, Provider-stub sync, SQLC projection, receipt/event replay, populated projection and receipt down guards)\n'
