#!/usr/bin/env bash
set -euo pipefail

: "${P4A01_AUTH_TEST_DATABASE_URL:?P4A01_AUTH_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4A01_AUTH_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

has_migration_table="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
  "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")"
if [[ "$has_migration_table" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 26
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 26
fi

read -r server_version baseline_waterline oauth_table <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT current_setting('server_version_num'), max(version_id),
            (to_regclass('public.admin_oauth_states') IS NOT NULL)::int
       FROM goose_db_version WHERE is_applied"
)"
[[ "$server_version" = "160014" && "$baseline_waterline" = "26" && "$oauth_table" = "0" ]]

admin_user_id="$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
    "INSERT INTO admin_users (
       auth_provider, provider_tenant_id, provider_subject_id, display_name, role,
       is_active, login_enabled, session_version
     ) VALUES ('wecom', 'corp-a01-migration', 'member-a01-migration', 'A01 migration fixture', 'admin', TRUE, TRUE, 1)
     ON CONFLICT (auth_provider, provider_tenant_id, provider_subject_id) DO UPDATE SET
       display_name=EXCLUDED.display_name, role=EXCLUDED.role, staff_id=NULL,
       is_active=TRUE, login_enabled=TRUE, session_version=admin_users.session_version + 1,
       updated_at=now()
     RETURNING id"
)"
[[ "$admin_user_id" =~ ^[1-9][0-9]*$ ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "DELETE FROM admin_sessions WHERE admin_user_id=${admin_user_id};
   INSERT INTO admin_sessions (
     session_token_hash, csrf_token_hash, admin_user_id, session_version, auth_time, expires_at
   ) SELECT
     decode(md5('a01-session') || md5('a01-session-2'), 'hex'),
     decode(md5('a01-csrf') || md5('a01-csrf-2'), 'hex'),
     id, session_version, now(), now() + interval '8 hours'
   FROM admin_users WHERE id=${admin_user_id};"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r upgrade_waterline oauth_table oauth_index session_count <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (to_regclass('public.admin_oauth_states') IS NOT NULL)::int,
            (to_regclass('public.idx_admin_oauth_states_expiry') IS NOT NULL)::int,
            (SELECT count(*) FROM admin_sessions WHERE admin_user_id=${admin_user_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$upgrade_waterline" = "27" && "$oauth_table" = "1" && "$oauth_index" = "1" && "$session_count" = "1" ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "INSERT INTO admin_oauth_states (state_hash, auth_provider, next_path, created_at, expires_at)
   VALUES (
     decode(md5('a01-state') || md5('a01-state-2'), 'hex'), 'wecom', '/admin', now(), now() + interval '5 minutes'
   );"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down
read -r rollback_waterline oauth_table session_count <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (to_regclass('public.admin_oauth_states') IS NOT NULL)::int,
            (SELECT count(*) FROM admin_sessions WHERE admin_user_id=${admin_user_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$rollback_waterline" = "26" && "$oauth_table" = "0" && "$session_count" = "1" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r final_waterline oauth_table state_count session_count <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (to_regclass('public.admin_oauth_states') IS NOT NULL)::int,
            (SELECT count(*) FROM admin_oauth_states),
            (SELECT count(*) FROM admin_sessions WHERE admin_user_id=${admin_user_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$final_waterline" = "27" && "$oauth_table" = "1" && "$state_count" = "0" && "$session_count" = "1" ]]

psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "DELETE FROM admin_sessions WHERE admin_user_id=${admin_user_id};"
printf 'P4-A01 migration compatibility: PASS (26/27/26/27, existing session history preserved; transient state reset)\n'
