#!/usr/bin/env bash
set -euo pipefail

: "${P4GROUP_OPS_TEST_DATABASE_URL:?P4GROUP_OPS_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4GROUP_OPS_TEST_DATABASE_URL"
temporary_database="aicrm_test_groupops_00063"
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
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
}

waterline() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied"
}

expect_facts_reject_down() {
  local output
  if output="$("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down 2>&1)"; then
    printf '%s\n' "$output" >&2
    echo "expected Group Ops fact-preserving down to fail" >&2
    exit 1
  fi
  [[ "$output" == *"SQLSTATE 55000"* ]]
  [[ "$(waterline)" = "63" ]]
}

fresh_database
AICRM_GROUP_OPS_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s ./internal/groupops/... ./internal/contact/store
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO group_ops_plans (name, status, revision, created_by, updated_by, created_at, updated_at) VALUES ('migration guard plan', 'draft', 1, 1, 1, now(), now())"
expect_facts_reject_down
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c 'SELECT count(*) FROM group_ops_plans')" = "1" ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "DELETE FROM group_ops_plans WHERE name = 'migration guard plan'"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null

psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO group_ops_operation_receipts (operation, actor_scope, key_digest, payload_digest, state, result_snapshot, created_at, completed_at) VALUES ('plan_create', 'admin:1', decode(repeat('01', 32), 'hex'), decode(repeat('02', 32), 'hex'), 'completed', '{}'::jsonb, now(), now())"
expect_facts_reject_down
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c 'SELECT count(*) FROM group_ops_operation_receipts')" = "1" ]]

# Completed receipts are intentionally immutable. Recreate only this isolated
# test database, then prove empty-schema down/up remains reversible.
fresh_database
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(waterline)" = "63" ]]

printf 'P4 Group Ops migration compatibility: PASS (63 fact guard, empty down/up)\n'
