#!/usr/bin/env bash
set -euo pipefail

: "${P4COMMERCE_REFUND_V2_TEST_DATABASE_URL:?P4COMMERCE_REFUND_V2_TEST_DATABASE_URL is required}"
base_database_url="$P4COMMERCE_REFUND_V2_TEST_DATABASE_URL"
[[ "$base_database_url" = *'/aicrm_test?sslmode=disable' ]]
database_url="${base_database_url%/aicrm_test?sslmode=disable}/aicrm_test_commerce_refund_v2_00096?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
down_output="$(mktemp -t aicrm-commerce-refund-v2-down.XXXXXX)"

cleanup() {
  rm -f "$down_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_commerce_refund_v2_00096 WITH (FORCE)' >/dev/null
}
trap cleanup EXIT
cleanup
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_commerce_refund_v2_00096' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 96 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id IN (95,96) AND is_applied')" = '2' ]]

# Refund Provider 96 is serialized directly after Shop Order Material 95.
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 95 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 96 AND is_applied')" = '0' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 95 AND is_applied')" = '1' ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 <<'SQL'
WITH inserted_order AS (
  INSERT INTO public.order_list_projections (
    provider, provider_label, merchant_order_no, platform_transaction_no,
    product_code, amount_minor, currency, status, status_label, detail_url,
    created_at, updated_at
  ) VALUES (
    'wechat_shop', '微信小店', 'migration-96-historical-final',
    'migration-96-historical-transaction', 'historical-product', 100, 'CNY',
    'paid', '已支付', '/migration-96-historical-final', now(), now()
  ) RETURNING id
), inserted_refund AS (
  INSERT INTO public.order_wechat_shop_refunds (
    order_id, actor_id, merchant_order_no, out_refund_no, amount_minor, currency,
    reason_digest, transaction_digest, command_key_digest, command_payload_digest,
    source_ref_digest, target_ref_digest, payload_digest, policy_version_digest,
    state, attempt_count, version, created_at, updated_at
  ) SELECT
    id, 9600, 'migration-96-historical-final',
    'wsr_96000000000000000000000000000000', 100, 'CNY',
    decode(repeat('11',32),'hex'), decode(repeat('12',32),'hex'),
    decode(repeat('13',32),'hex'), decode(repeat('14',32),'hex'),
    decode(repeat('15',32),'hex'), decode(repeat('16',32),'hex'),
    decode(repeat('17',32),'hex'), decode(repeat('18',32),'hex'),
    'final_failed', 1, 2, now(), now()
  FROM inserted_order RETURNING id
)
INSERT INTO public.order_wechat_shop_refund_attempts (
  refund_id, attempt_no, river_job_id, river_attempt, args_digest,
  request_digest, outcome, evidence_digest, started_at, completed_at
)
SELECT id, 1, 960001, 1, decode(repeat('21',32),'hex'),
  decode(repeat('22',32),'hex'), 'final_failed', NULL, now(), now()
FROM inserted_refund;
SQL
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 96 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM order_wechat_shop_refund_attempts a JOIN order_wechat_shop_refunds r ON r.id=a.refund_id WHERE r.contract_version='local/v1' AND r.out_refund_no='wsr_96000000000000000000000000000000' AND a.outcome='final_failed' AND a.evidence_digest IS NULL")" = '1' ]]

GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" test -race -count=1 -timeout=300s -run '^TestCommerceRefundV2' ./acceptance/order -args -database-url "$database_url"

for table in order_wechat_shop_refunds order_wechat_shop_refund_attempts order_wechat_shop_refund_callbacks order_wechat_shop_refund_queries order_wechat_shop_material_sync_requests; do
  [[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM public.$table")" -ge 1 ]]
done

set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 95 >"$down_output" 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back materialized WeChat Shop Provider facts' "$down_output"
grep -Fq 'SQLSTATE 55000' "$down_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 96 AND is_applied')" = '1' ]]
printf 'P4 WeChat Shop Refund Provider PG16.14 acceptance: PASS (95->96->95->96, material sync retry, exact aftersale callback/query settlement, populated 55000 guard; fake Provider only)\n'
