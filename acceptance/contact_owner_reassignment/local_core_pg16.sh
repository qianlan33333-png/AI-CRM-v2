#!/usr/bin/env bash
set -euo pipefail

: "${P4CONTACT_OWNER_REASSIGNMENT_TEST_DATABASE_URL:?P4CONTACT_OWNER_REASSIGNMENT_TEST_DATABASE_URL is required}"
base_database_url="$P4CONTACT_OWNER_REASSIGNMENT_TEST_DATABASE_URL"
database_url="${base_database_url/aicrm_test/aicrm_test_contact_owner_reassignment_00070}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_contact_owner_reassignment_00070 WITH (FORCE)' >/dev/null; }
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_contact_owner_reassignment_00070' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = "160014" ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = "70" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
P4CONTACT_OWNER_REASSIGNMENT_ACCEPTANCE_DATABASE_URL="$database_url" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -count=1 -race -run '^TestContactOwnerReassignmentLocalCorePG16$' ./internal/contact/store
if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/tmp/aicrm-owner-reassignment-down.log 2>&1; then
  echo 'owner reassignment facts unexpectedly allowed migration rollback' >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' /tmp/aicrm-owner-reassignment-down.log
printf 'P4 Contact Owner Reassignment Local Core PG16 acceptance: PASS\n'
