#!/usr/bin/env bash
set -euo pipefail

: "${P4PUSHCENTER_TEST_DATABASE_URL:?P4PUSHCENTER_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4PUSHCENTER_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

has_migration_table="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")"
if [[ "$has_migration_table" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 43
else
  waterline="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")"
  if (( waterline > 43 )); then
    "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 43
  elif (( waterline < 43 )); then
    "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 43
  fi
fi

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 44
read -r waterline state entries text_index tenant_columns foreign_keys ready fixture allow_fixture <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.push_center_read_model_state') IS NOT NULL)::int,
  (to_regclass('public.push_center_read_model_entries') IS NOT NULL)::int,
  (to_regclass('public.push_center_read_model_entries_text_trgm_idx') IS NOT NULL)::int,
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'push_center_read_model_%' AND column_name ILIKE ('%' || 'ten' || 'ant%')),
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('push_center_read_model_state'::regclass, 'push_center_read_model_entries'::regclass) AND contype='f'),
  (SELECT production_data_ready::int FROM push_center_read_model_state WHERE singleton=true),
  (SELECT fixture_mode::int FROM push_center_read_model_state WHERE singleton=true),
  (SELECT allow_fixture_repo_in_prod::int FROM push_center_read_model_state WHERE singleton=true)")"
[[ "$waterline" = "44" && "$state" = "1" && "$entries" = "1" && "$text_index" = "1" && "$tenant_columns" = "0" && "$foreign_keys" = "0" && "$ready" = "0" && "$fixture" = "0" && "$allow_fixture" = "0" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s ./acceptance/pushcenter -args -database-url "$database_url"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 43
read -r waterline state entries <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.push_center_read_model_state') IS NOT NULL)::int,
  (to_regclass('public.push_center_read_model_entries') IS NOT NULL)::int")"
[[ "$waterline" = "43" && "$state" = "0" && "$entries" = "0" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 44
read -r waterline ready fixture allow_fixture <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (SELECT production_data_ready::int FROM push_center_read_model_state WHERE singleton=true),
  (SELECT fixture_mode::int FROM push_center_read_model_state WHERE singleton=true),
  (SELECT allow_fixture_repo_in_prod::int FROM push_center_read_model_state WHERE singleton=true)")"
[[ "$waterline" = "44" && "$ready" = "0" && "$fixture" = "0" && "$allow_fixture" = "0" ]]

printf 'P4 Push Center 0421/0422 migration compatibility: PASS (43/44/43/44; read-only projection, no tenant, FK, worker, provider, or outbound execution)\n'
