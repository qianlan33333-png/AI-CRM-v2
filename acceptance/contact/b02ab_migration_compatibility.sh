#!/usr/bin/env bash
set -euo pipefail

: "${P4B02AB_TAG_TEST_DATABASE_URL:?P4B02AB_TAG_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4B02AB_TAG_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
if [[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 37
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 37
fi

marker="b02ab-history-$(date +%s)-$$"
group_id="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "INSERT INTO tag_groups(name) VALUES('$marker') RETURNING id")"
tag_id="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "INSERT INTO tags(group_id,name) VALUES($group_id,'$marker-tag') RETURNING id")"
history_shape(){ psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F '|' -c "SELECT g.id,g.name,t.id,t.name FROM tag_groups g JOIN tags t ON t.group_id=g.id WHERE g.id=$group_id AND t.id=$tag_id"; }
before="$(history_shape)"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 38
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "38" ]]
[[ "$(history_shape)" = "$before" ]]
read -r sync_table live_table gate_table executed external <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT to_regclass('public.legacy_tag_sync_receipts')::text,to_regclass('public.legacy_tag_live_mutation_receipts')::text,to_regclass('public.legacy_tag_execution_status')::text,payload->>'executed',payload->>'real_external_call_executed' FROM legacy_tag_execution_status WHERE singleton")"
[[ "$sync_table" = "legacy_tag_sync_receipts" && "$live_table" = "legacy_tag_live_mutation_receipts" && "$gate_table" = "legacy_tag_execution_status" && "$executed" = "false" && "$external" = "false" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 37
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "37" ]]
[[ "$(history_shape)" = "$before" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.legacy_tag_sync_receipts') IS NULL)::int")" = "1" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 38
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "38" ]]
[[ "$(history_shape)" = "$before" ]]
printf 'P4-B02AB migration compatibility: PASS (37/38/37/38, local tag history preserved)\n'
