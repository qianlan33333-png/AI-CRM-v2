#!/usr/bin/env bash
set -euo pipefail

: "${P4_DM01_TEST_DATABASE_URL:?P4_DM01_TEST_DATABASE_URL is required}"
base_database_url="$P4_DM01_TEST_DATABASE_URL"
database_url="$(python3 - "$base_database_url" <<'PY'
import sys
from urllib.parse import urlsplit, urlunsplit

parsed = urlsplit(sys.argv[1])
if parsed.path != '/aicrm_test_dm01_target':
    raise SystemExit('unexpected DM01 migration base database')
print(urlunsplit((parsed.scheme, parsed.netloc, '/aicrm_test_dm01_00072', parsed.query, parsed.fragment)))
PY
)"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
down_log="$(mktemp -t aicrm-dm01-down.XXXXXX)"
trap 'rm -f "$down_log"' EXIT

MIGRATION_TEST_DATABASE_URL="$base_database_url" MIGRATION_TEST_DATABASE_NAME=aicrm_test_dm01_target GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_dm01_00072 WITH (FORCE)' >/dev/null; }
finish() { cleanup; rm -f "$down_log"; }
trap finish EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_dm01_00072' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 72 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = "160014" ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = "72" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 72 >/dev/null
for protected_state in imported reconciling reconciled; do
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO legacy_contact_identity_import_runs (source_manifest_sha256, source_repository_sha, snapshot_id, mode, upper_watermark, hmac_key_version, state) VALUES (decode(repeat('00', 32), 'hex'), '2b7a80126d7becb6f95cf1ec5945dcb78a42f531', 'acceptance-$protected_state', 'full', now(), 1, '$protected_state')" >/dev/null
  if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >"$down_log" 2>&1; then
    echo "DM01 $protected_state run unexpectedly allowed migration rollback" >&2
    exit 1
  fi
  grep -Fq 'SQLSTATE 55000' "$down_log"
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c 'DELETE FROM legacy_contact_identity_import_runs' >/dev/null
done
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "WITH run AS (INSERT INTO legacy_contact_identity_import_runs (source_manifest_sha256, source_repository_sha, snapshot_id, mode, upper_watermark, hmac_key_version, state) VALUES (decode(repeat('11', 32), 'hex'), '2b7a80126d7becb6f95cf1ec5945dcb78a42f531', 'acceptance-checkpoint', 'full', now(), 1, 'importing') RETURNING id) INSERT INTO legacy_contact_identity_import_checkpoints(run_id,source_table,final_source_key_hmac,payload_hmac,field_digest,upper_bound_empty) SELECT id,'contacts',decode(repeat('22',32),'hex'),decode(repeat('33',32),'hex'),decode(repeat('44',32),'hex'),true FROM run" >/dev/null
if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >"$down_log" 2>&1; then
  echo 'DM01 materialized checkpoint unexpectedly allowed migration rollback' >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' "$down_log"
printf 'DM01 migration PG16 acceptance: PASS\n'
