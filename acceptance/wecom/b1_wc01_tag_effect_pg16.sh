#!/usr/bin/env bash
set -euo pipefail

: "${P4B1WC01_WECOM_TAG_EFFECT_TEST_DATABASE_URL:?P4B1WC01_WECOM_TAG_EFFECT_TEST_DATABASE_URL is required}"
base_database_url="$P4B1WC01_WECOM_TAG_EFFECT_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
temporary_database='aicrm_test_wecom_tag_effect_00088'
database_url="${base_database_url%"$database_suffix"}/$temporary_database?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-wecom-tag-effect-down.XXXXXX")"
legacy_key_digest="$(printf %s 'wc01-preexisting-legacy' | openssl dgst -sha256 -r | awk '{print $1}')"

MIGRATION_TEST_DATABASE_URL="$base_database_url" MIGRATION_TEST_DATABASE_NAME='aicrm_test' \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  rm -f "$guard_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

goose=("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url")
"${goose[@]}" up-to 87 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -v legacy_key_digest="$legacy_key_digest" <<'SQL'
INSERT INTO public.legacy_tag_live_mutation_receipts
  (actor_id,idempotency_key,key_digest,operation,payload,trace_id,state,event_id,river_job_id,accepted_at)
VALUES
  (88,'wc01-preexisting-legacy',decode(:'legacy_key_digest','hex'),'mark','{}','wc01-preexisting','queued',8800,8800,now());
SQL

"${goose[@]}" up-to 88 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '88' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('wecom_tag_effects','wecom_tag_catalog_snapshots','wecom_tag_catalog_groups','wecom_tag_catalog_tags')")" = '4' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='wecom_tag_effects' AND column_name IN ('effect_id','accept_receipt_id','queue_receipt_id','attempt_receipt_id','reconcile_receipt_id') AND data_type='bigint'")" = '5' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT (SELECT count(*) FROM public.legacy_tag_live_mutation_receipts WHERE trace_id='wc01-preexisting') || '|' || (SELECT count(*) FROM public.wecom_tag_effects) || '|' || (SELECT count(*) FROM public.external_effects WHERE owner='wecom' AND kind='wecom_tag_sync')")" = '1|0|0' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT (to_regclass('public.wecom_tag_effect_attempts') IS NULL)::int || '|' || (to_regclass('public.wecom_tag_effect_reconciliations') IS NULL)::int")" = '1|1' ]]

"${goose[@]}" down-to 87 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c "SELECT (to_regclass('public.wecom_tag_effects') IS NULL)::int")" = '1' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM public.legacy_tag_live_mutation_receipts WHERE trace_id='wc01-preexisting'")" = '1' ]]
"${goose[@]}" up-to 88 >/dev/null

P4B1WC01_WECOM_TAG_EFFECT_TEST_DATABASE_URL="$database_url" \
  /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestB1WC01WeComTagEffectPG16$' ./acceptance/wecom

set +e
"${goose[@]}" down-to 87 >"$guard_output" 2>&1
guard_status=$?
set -e
[[ "$guard_status" -ne 0 ]]
grep -Fq 'cannot roll back populated WeCom tag effect runtime' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = '88' ]]

printf 'P4 B1-WC01 WeCom tag effect PG16.14 acceptance: PASS (88 up/down/up, atomic legacy rollback, no legacy replay, EER/River disabled+unknown/reconcile, immutable catalog, populated 55000 guard; zero network)\n'
