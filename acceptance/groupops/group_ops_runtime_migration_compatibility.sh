#!/usr/bin/env bash
set -euo pipefail

: "${P4GROUP_OPS_TEST_DATABASE_URL:?P4GROUP_OPS_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4GROUP_OPS_TEST_DATABASE_URL"
temporary_database="aicrm_test_groupops_00085"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT

fresh_database() {
  cleanup
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 85 >/dev/null
}

waterline() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied"
}

expect_facts_reject_down() {
  local output
  if output="$("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down 2>&1)"; then
    printf '%s\n' "$output" >&2
    echo "expected populated Group Ops runtime down to fail" >&2
    exit 1
  fi
  [[ "$output" == *"cannot roll back populated group ops runtime facts"* ]]
  [[ "$(waterline)" = "85" ]]
}

fresh_database
[[ "$(waterline)" = "85" ]]
AICRM_GROUP_OPS_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s ./internal/groupops/... ./internal/externaleffects/...

fresh_database
[[ "$(waterline)" = "85" ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "INSERT INTO group_ops_directory_groups (chat_reference, owner_staff_id, display_name, member_count, source_digest, refreshed_at) VALUES ('guard-group', 1, 'Guard Group', 0, 'sha256:' || repeat('0', 64), now())"
expect_facts_reject_down
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "DELETE FROM group_ops_directory_groups WHERE chat_reference = 'guard-group'"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 84 >/dev/null
[[ "$(waterline)" = "84" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 85 >/dev/null
[[ "$(waterline)" = "85" ]]

printf 'P4 Group Ops runtime migration compatibility: PASS (PG16.14, exact 85, populated guard, empty 84/85 down/up)\n'
