#!/usr/bin/env bash
set -euo pipefail

: "${P4ADMINOPS_TEST_DATABASE_URL:?P4ADMINOPS_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4ADMINOPS_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

has_migration_table="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")"
if [[ "$has_migration_table" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 42
else
  waterline="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")"
  if [[ "$waterline" = "43" ]]; then
    "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 42
  elif [[ "$waterline" != "42" ]]; then
    "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 42
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
    (SELECT count(*) FROM automation_agent_configurations),
    (SELECT md5(COALESCE(string_agg(row_to_json(a)::text,E'\\n' ORDER BY id),'')) FROM automation_agent_configurations a)"
}

baseline="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 43
read -r waterline credentials categories releases jobs receipts notifications tenant_columns cross_domain_foreign_keys secret_boundary_columns <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.admin_ops_credentials') IS NOT NULL)::int,
  (to_regclass('public.admin_ops_config_categories') IS NOT NULL)::int,
  (to_regclass('public.admin_ops_config_releases') IS NOT NULL)::int,
  (to_regclass('public.admin_ops_jobs') IS NOT NULL)::int,
  (to_regclass('public.admin_ops_action_receipts') IS NOT NULL)::int,
  (to_regclass('public.admin_ops_notification_settings') IS NOT NULL)::int,
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'admin_ops_%' AND column_name ILIKE '%tenant%'),
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('admin_ops_credentials'::regclass,'admin_ops_config_categories'::regclass,'admin_ops_config_releases'::regclass,'admin_ops_jobs'::regclass,'admin_ops_action_receipts'::regclass,'admin_ops_notification_settings'::regclass) AND contype='f' AND confrelid NOT IN ('admin_ops_config_releases'::regclass)),
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('admin_ops_credentials','admin_ops_notification_settings') AND column_name IN ('secret_ref','secret_mask'))")"
[[ "$waterline" = "43" && "$credentials" = "1" && "$categories" = "1" && "$releases" = "1" && "$jobs" = "1" && "$receipts" = "1" && "$notifications" = "1" && "$tenant_columns" = "0" && "$cross_domain_foreign_keys" = "0" && "$secret_boundary_columns" = "4" ]]
[[ "$(history_snapshot)" = "$baseline" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  ADMINOPS_TEST_DATABASE_URL="$database_url" "$go_command" test -race -count=1 -timeout=180s ./acceptance/adminops

post_acceptance="$(history_snapshot)"
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 42
read -r waterline credentials jobs receipts <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.admin_ops_credentials') IS NOT NULL)::int,
  (to_regclass('public.admin_ops_jobs') IS NOT NULL)::int,
  (to_regclass('public.admin_ops_action_receipts') IS NOT NULL)::int")"
[[ "$waterline" = "42" && "$credentials" = "0" && "$jobs" = "0" && "$receipts" = "0" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 43
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")" = "43" ]]
[[ "$(history_snapshot)" = "$post_acceptance" ]]

printf 'P4 Admin Config and Jobs A+B migration compatibility: PASS (42/43/42/43; Auth/session/Event/Automation history preserved; secret references only, no tenant, cross-domain FK, worker, River, provider, or outbound effect)\n'
