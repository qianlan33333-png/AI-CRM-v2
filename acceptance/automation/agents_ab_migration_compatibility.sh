#!/usr/bin/env bash
set -euo pipefail

: "${P4AUTOMATIONAGENTSAB_TEST_DATABASE_URL:?P4AUTOMATIONAGENTSAB_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4AUTOMATIONAGENTSAB_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

has_migration_table="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")"
if [[ "$has_migration_table" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 41
else
  waterline="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")"
  if [[ "$waterline" = "42" || "$waterline" = "43" ]]; then
    "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 41
  elif [[ "$waterline" != "41" ]]; then
    "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 41
  fi
fi

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(row_to_json(e)::text,E'\\n' ORDER BY id),'')) FROM event_log e),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(row_to_json(u)::text,E'\\n' ORDER BY id),'')) FROM admin_users u),
    (SELECT count(*) FROM admin_sessions),
    (SELECT md5(COALESCE(string_agg(row_to_json(s)::text,E'\\n' ORDER BY id),'')) FROM admin_sessions s),
    (SELECT count(*) FROM questionnaires),
    (SELECT md5(COALESCE(string_agg(row_to_json(q)::text,E'\\n' ORDER BY id),'')) FROM questionnaires q)"
}

baseline="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 42
read -r waterline configurations receipts tenant_columns foreign_keys indexes <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.automation_agent_configurations') IS NOT NULL)::int,
  (to_regclass('public.automation_agent_operation_receipts') IS NOT NULL)::int,
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('automation_agent_configurations','automation_agent_operation_receipts') AND column_name ILIKE '%tenant%'),
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('automation_agent_configurations'::regclass,'automation_agent_operation_receipts'::regclass) AND contype='f'),
  (SELECT count(*) FROM pg_index WHERE indrelid IN ('automation_agent_configurations'::regclass,'automation_agent_operation_receipts'::regclass) AND indisvalid AND indisready AND indislive)")"
[[ "$waterline" = "42" && "$configurations" = "1" && "$receipts" = "1" && "$tenant_columns" = "0" && "$foreign_keys" = "0" && "$indexes" = "6" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s -run '^TestP4AutomationAgentsAB' \
  ./acceptance/automation -args -database-url "$database_url" -expected-waterline 42

post_acceptance="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 41
read -r waterline configurations receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.automation_agent_configurations') IS NOT NULL)::int,
  (to_regclass('public.automation_agent_operation_receipts') IS NOT NULL)::int")"
[[ "$waterline" = "41" && "$configurations" = "0" && "$receipts" = "0" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 42
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "42" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]

printf 'P4 Automation Agents A+B migration compatibility: PASS (41/42/41/42; Auth/session/Event/Survey history preserved; no tenant, foreign key, worker, AI, or outbound effect)\n'
