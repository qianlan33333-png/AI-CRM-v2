#!/usr/bin/env bash
set -euo pipefail

: "${P4_WECHAT_SHOP_MATERIAL_TEST_DATABASE_URL:?P4_WECHAT_SHOP_MATERIAL_TEST_DATABASE_URL is required}"
base_database_url="$P4_WECHAT_SHOP_MATERIAL_TEST_DATABASE_URL"
[[ "$base_database_url" = *'/aicrm_test?sslmode=disable' ]]
database_url="${base_database_url%/aicrm_test?sslmode=disable}/aicrm_test_wechat_shop_material_00095?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
down_output="$(mktemp -t aicrm-wechat-shop-material-down.XXXXXX)"

cleanup() {
  rm -f "$down_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 \
    -c 'DROP DATABASE IF EXISTS aicrm_test_wechat_shop_material_00095 WITH (FORCE)' >/dev/null
}
trap cleanup EXIT
cleanup
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 \
  -c 'CREATE DATABASE aicrm_test_wechat_shop_material_00095' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 95 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id IN (94,95) AND is_applied')" = '2' ]]

GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s \
  -run '^TestWeChatShopMaterialPostgreSQLConstraintsAndPIIBoundary$' ./acceptance/order \
  -args -wechat-shop-material-database-url "$database_url"

set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 94 >"$down_output" 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back populated WeChat Shop order material' "$down_output"
grep -Fq 'SQLSTATE 55000' "$down_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 95 AND is_applied')" = '1' ]]
printf 'P4 WeChat Shop order material PG16.14 acceptance: PASS (94->95, typed/PII-free store, idempotent replay, Provider-over-legacy, populated rollback guard; no Provider call)\n'
