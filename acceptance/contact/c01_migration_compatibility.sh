#!/usr/bin/env bash
set -euo pipefail
: "${P4C01_CHANNEL_TEST_DATABASE_URL:?P4C01_CHANNEL_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4C01_CHANNEL_TEST_DATABASE_URL"
temporary_database="aicrm_test_c01_channel"
database_url="${base_database_url/aicrm_test/$temporary_database}"
MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 31
marker="c01-history-$(date +%s)-$$"
history_id="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "INSERT INTO channels(name,code,config) VALUES('历史渠道','$marker','{\"preserved\":true}'::jsonb) RETURNING id")"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO customers(name,channel_id) VALUES('历史客户',$history_id)" >/dev/null
history_shape(){ psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT c.id,c.name,c.code,(SELECT count(*) FROM customers x WHERE x.channel_id=c.id) FROM channels c WHERE c.code='$marker'"; }
before="$(history_shape)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 32
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "32" ]]
[[ "$(history_shape)" = "$before" ]]
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -timeout=360s -run '^TestC01(Create|Event|S200K)' ./acceptance/contact -args -database-url "$database_url"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 31
[[ "$(history_shape)" = "$before" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 32
[[ "$(history_shape)" = "$before" ]]
printf 'P4-C01 migration compatibility: PASS (31/32/31/32, Contact channel/customer history preserved)\n'
