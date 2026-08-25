#!/usr/bin/env bash
set -euo pipefail

: "${P4EXTERNALPUSH_TEST_DATABASE_URL:?P4EXTERNALPUSH_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4EXTERNALPUSH_TEST_DATABASE_URL"
temporary_database="aicrm_test_commerce_external_push"
database_url="${base_database_url/aicrm_test/$temporary_database}"
down_output="$(mktemp)"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
  rm -f "$down_output"
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 85
psql "$database_url" -X -q -v ON_ERROR_STOP=1 <<'SQL'
WITH effect AS (
  INSERT INTO public.external_effects
    (owner,kind,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash,envelope_fingerprint,state)
  VALUES
    ('group_ops','group_ops_broadcast','sha256:'||repeat('1',64),'sha256:'||repeat('2',64),'sha256:'||repeat('3',64),'sha256:'||repeat('4',64),'sha256:'||repeat('5',64),'accepted')
  RETURNING id
)
INSERT INTO public.external_effect_receipts
  (operation,effect_id,receipt_key_digest,command_digest,state)
SELECT 'accept',id,'sha256:'||repeat('6',64),'sha256:'||repeat('7',64),'accepted' FROM effect;
SQL

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 87
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "87" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.product_external_push_configurations') IS NOT NULL)::int, (to_regclass('public.product_external_push_test_bindings') IS NOT NULL)::int")" = $'1|1' ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM public.external_effects WHERE owner='group_ops' AND kind='group_ops_broadcast' AND state='accepted'")" = "1" ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO public.external_effects (owner,kind,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash,envelope_fingerprint,state) VALUES ('group_ops','group_ops_broadcast','sha256:'||repeat('8',64),'sha256:'||repeat('9',64),'sha256:'||repeat('a',64),'sha256:'||repeat('b',64),'sha256:'||repeat('c',64),'accepted')" >/dev/null

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 86
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "86" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.product_external_push_configurations') IS NULL)::int, (to_regclass('public.product_external_push_test_bindings') IS NULL)::int")" = $'1|1' ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM public.external_effects WHERE owner='group_ops' AND kind='group_ops_broadcast' AND state='accepted'")" = "2" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 87
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "87" ]]
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestCommerceExternalPushCanonicalPG16$' \
  ./cmd/aicrm -args -p4-commerce87-database-url "$database_url"
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=120s -run '^TestCommerceExternalPushPG16RoundTrip$' \
  ./acceptance/product -args -database-url "$database_url"

set +e
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 86 >"$down_output" 2>&1
status=$?
set -e
[[ $status -ne 0 ]]
grep -Fq 'cannot roll back populated product external push facts' "$down_output"
grep -Fq 'SQLSTATE 55000' "$down_output"
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "87" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.product_external_push_configurations') IS NOT NULL)::int, (to_regclass('public.product_external_push_test_bindings') IS NOT NULL)::int")" = $'1|1' ]]

printf 'P4 Commerce External Push 00087 PG16.14: PASS (85 GroupOps EER -> 87, 87->86->87, canonical HTTP, single accepted proof, populated 55000 guard; no Provider)\n'
