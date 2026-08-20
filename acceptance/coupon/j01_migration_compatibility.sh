#!/usr/bin/env bash
set -euo pipefail
: "${P4J01_COUPON_TEST_DATABASE_URL:?P4J01_COUPON_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4J01_COUPON_TEST_DATABASE_URL"
MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
if [[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 32
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 32
fi
snapshot(){ psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT (SELECT count(*) FROM event_log),(SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),(SELECT count(*) FROM admin_users),(SELECT md5(COALESCE(string_agg(row_to_json(a)::text,E'\\n' ORDER BY id),'')) FROM admin_users a),(SELECT count(*) FROM admin_sessions),(SELECT md5(COALESCE(string_agg(row_to_json(s)::text,E'\\n' ORDER BY id),'')) FROM admin_sessions s),(SELECT count(*) FROM products),(SELECT md5(COALESCE(string_agg(row_to_json(p)::text,E'\\n' ORDER BY id),'')) FROM products p)"; }
baseline="$(snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 33
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "33" ]]
[[ "$(snapshot)" = "$baseline" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 32
[[ "$(snapshot)" = "$baseline" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 33
[[ "$(snapshot)" = "$baseline" ]]
# Current Product code requires the version column introduced after migration
# 33. Keep the historical 32/33 compatibility proof isolated above, then run
# the current Coupon acceptance against the latest schema.
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -timeout=360s -run '^TestJ01' ./acceptance/coupon -args -database-url "$database_url"
printf 'P4-J01 migration compatibility: PASS (32/33/32/33, current acceptance on latest schema, pre-existing Event/Auth/session/Product history preserved)\n'
