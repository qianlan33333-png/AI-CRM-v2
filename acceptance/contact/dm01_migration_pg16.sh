#!/usr/bin/env bash
set -euo pipefail

: "${P4_DM01_TEST_DATABASE_URL:?P4_DM01_TEST_DATABASE_URL is required}"
base_database_url="$P4_DM01_TEST_DATABASE_URL"
database_url="${base_database_url/aicrm_test/aicrm_test_dm01_00072}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_dm01_00072 WITH (FORCE)' >/dev/null; }
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_dm01_00072' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = "160014" ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = "72" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO legacy_contact_identity_import_runs (source_manifest_sha256, source_repository_sha, snapshot_id, mode, upper_watermark, hmac_key_version, state) VALUES (decode(repeat('00', 32), 'hex'), '2b7a80126d7becb6f95cf1ec5945dcb78a42f531', 'acceptance', 'full', now(), 1, 'imported')" >/dev/null
if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/tmp/aicrm-dm01-down.log 2>&1; then
  echo 'DM01 materialized import unexpectedly allowed migration rollback' >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' /tmp/aicrm-dm01-down.log
printf 'DM01 migration PG16 acceptance: PASS\n'
