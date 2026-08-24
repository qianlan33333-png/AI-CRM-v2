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
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO public.contact_owner_reassignment_previews(id,actor_id,idempotency_key_digest,payload_digest,preview_hash,rows,created_at,expires_at) VALUES ('cor_abcdefghijklmnopqrstuv',1,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'[]','2026-08-24T00:00:00Z','2026-08-24T00:15:00Z')" >/dev/null
if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/tmp/aicrm-owner-reassignment-down.log 2>&1; then
  echo 'owner reassignment facts unexpectedly allowed migration rollback' >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' /tmp/aicrm-owner-reassignment-down.log
printf 'P4 Contact Owner Reassignment Local Core PG16 acceptance: PASS\n'
