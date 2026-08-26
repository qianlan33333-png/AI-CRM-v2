#!/usr/bin/env bash
set -euo pipefail

: "${P4AIAUDIENCEINBOUNDWEBHOOK_TEST_DATABASE_URL:?P4AIAUDIENCEINBOUNDWEBHOOK_TEST_DATABASE_URL is required}"
base_database_url="$P4AIAUDIENCEINBOUNDWEBHOOK_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
database_name='aicrm_test_ai_audience_00101'
database_url="${base_database_url%"$database_suffix"}/$database_name?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-ai-audience-00101-down.XXXXXX")"

cleanup() {
  rm -f "$guard_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $database_name WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $database_name" >/dev/null
goose=("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url")
"${goose[@]}" up-to 101 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
CI_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestAIAudienceInboundWebhookPG16$' ./acceptance/segment
if "${goose[@]}" down-to 100 >"$guard_output" 2>&1; then
  printf 'expected populated AI Audience inbound webhook guard to fail\n' >&2
  exit 1
fi
grep -Fq 'cannot roll back populated AI Audience inbound webhook facts' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"
printf 'P4 AI Audience inbound webhook PG16.14: PASS (101, receipt and transport replay conflicts, unknown package, same-UoW event rollback, populated down guard, no Provider)\n'
