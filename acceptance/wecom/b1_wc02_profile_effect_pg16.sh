#!/usr/bin/env bash
set -euo pipefail

: "${P4B1WC02_WECOM_PROFILE_EFFECT_TEST_DATABASE_URL:?P4B1WC02_WECOM_PROFILE_EFFECT_TEST_DATABASE_URL is required}"
base_database_url="$P4B1WC02_WECOM_PROFILE_EFFECT_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
temporary_database='aicrm_test_wecom_contact_profile_effect_00102'
database_url="${base_database_url%"$database_suffix"}/$temporary_database?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-wecom-profile-effect-down.XXXXXX")"

cleanup() {
  rm -f "$guard_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
goose=("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url")
"${goose[@]}" up >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = '102' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='wecom_contact_profile_effects' AND column_name IN ('effect_id','accept_receipt_id','queue_receipt_id','attempt_receipt_id','reconcile_receipt_id')")" = '5' ]]

P4B1WC02_WECOM_PROFILE_EFFECT_TEST_DATABASE_URL="$database_url" \
  /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestB1WC02WeComContactProfileEffectPG16$' ./acceptance/wecom

set +e
"${goose[@]}" down-to 101 >"$guard_output" 2>&1
guard_status=$?
set -e
[[ "$guard_status" -ne 0 ]]
grep -Fq 'cannot roll back populated WeCom contact profile effect runtime' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"

printf 'P4 B1-WC02 WeCom contact profile effect PG16.14 acceptance: PASS (102 migration, typed EER unknown/reconcile, durable provider facts, immutable populated down guard; zero network)\n'
