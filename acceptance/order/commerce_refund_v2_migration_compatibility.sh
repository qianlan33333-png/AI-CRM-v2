#!/usr/bin/env bash
set -euo pipefail

: "${P4COMMERCE_REFUND_V2_TEST_DATABASE_URL:?P4COMMERCE_REFUND_V2_TEST_DATABASE_URL is required}"
base_database_url="$P4COMMERCE_REFUND_V2_TEST_DATABASE_URL"
[[ "$base_database_url" = *'/aicrm_test?sslmode=disable' ]]
database_url="${base_database_url%/aicrm_test?sslmode=disable}/aicrm_test_commerce_refund_v2_00086?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
down_output="$(mktemp -t aicrm-commerce-refund-v2-down.XXXXXX)"

cleanup() {
  rm -f "$down_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_commerce_refund_v2_00086 WITH (FORCE)' >/dev/null
}
trap cleanup EXIT
cleanup
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_commerce_refund_v2_00086' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 86 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 86 AND is_applied')" = '1' ]]

# Refund 86 is serialized directly after Group Ops 85 in this batch.
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 85 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 86 AND is_applied')" = '0' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 85 AND is_applied')" = '1' ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 86 >/dev/null

GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -timeout=300s -run '^TestCommerceRefundV2' ./acceptance/order -args -database-url "$database_url"

for table in order_wechat_shop_refunds order_wechat_shop_refund_attempts order_wechat_shop_refund_callbacks order_wechat_shop_refund_queries; do
  [[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM public.$table")" -ge 1 ]]
done

set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 85 >"$down_output" 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back materialized WeChat Shop refund facts' "$down_output"
grep -Fq 'SQLSTATE 55000' "$down_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 86 AND is_applied')" = '1' ]]
printf 'P4 Commerce Refund V2 PG16.14 acceptance: PASS (85->86->85->86, disabled production HTTP path, four-table populated 55000 guard; no Provider)\n'
