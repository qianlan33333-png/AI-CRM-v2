#!/usr/bin/env bash
set -euo pipefail

: "${P4SI00B_AUTH_TEST_DATABASE_URL:?P4SI00B_AUTH_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4SI00B_AUTH_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

has_migration_table="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
  "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")"
if [[ "$has_migration_table" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 27
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 27
fi

read -r server_version baseline_waterline old_column new_column <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT current_setting('server_version_num'), max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='provider_tenant_id'),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='wecom_corp_id')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$server_version" = "160014" && "$baseline_waterline" = "27" && "$old_column" = "1" && "$new_column" = "0" ]]

first_user_id="$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
    "INSERT INTO admin_users (
       auth_provider, provider_tenant_id, provider_subject_id, display_name, role,
       is_active, login_enabled, session_version
     ) VALUES ('wecom', 'ww-si00b-corp-a', 'member-shared', 'SI00B corp A', 'admin', TRUE, TRUE, 1)
     ON CONFLICT (auth_provider, provider_tenant_id, provider_subject_id) DO UPDATE SET
       display_name=EXCLUDED.display_name, is_active=TRUE, login_enabled=TRUE,
       session_version=admin_users.session_version + 1, updated_at=now()
     RETURNING id"
)"
second_user_id="$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
    "INSERT INTO admin_users (
       auth_provider, provider_tenant_id, provider_subject_id, display_name, role,
       is_active, login_enabled, session_version
     ) VALUES ('wecom', 'ww-si00b-corp-b', 'member-shared', 'SI00B corp B', 'ops', TRUE, TRUE, 1)
     ON CONFLICT (auth_provider, provider_tenant_id, provider_subject_id) DO UPDATE SET
       display_name=EXCLUDED.display_name, is_active=TRUE, login_enabled=TRUE,
       session_version=admin_users.session_version + 1, updated_at=now()
     RETURNING id"
)"
[[ "$first_user_id" =~ ^[1-9][0-9]*$ && "$second_user_id" =~ ^[1-9][0-9]*$ && "$first_user_id" != "$second_user_id" ]]

psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "DELETE FROM admin_sessions WHERE admin_user_id IN (${first_user_id}, ${second_user_id});
   INSERT INTO admin_sessions (
     session_token_hash, csrf_token_hash, admin_user_id, session_version, auth_time, expires_at
   ) SELECT
     decode(md5('si00b-session') || md5('si00b-session-2'), 'hex'),
     decode(md5('si00b-csrf') || md5('si00b-csrf-2'), 'hex'),
     id, session_version, now(), now() + interval '8 hours'
   FROM admin_users WHERE id=${first_user_id};"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r upgrade_waterline old_column new_column check_constraint unique_constraint unique_index accounts sessions located <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='provider_tenant_id'),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='wecom_corp_id'),
            (SELECT count(*) FROM pg_constraint WHERE conrelid='public.admin_users'::regclass AND conname='ck_admin_users_wecom_corp_id'),
            (SELECT count(*) FROM pg_constraint WHERE conrelid='public.admin_users'::regclass AND conname='uq_admin_users_wecom_identity'),
            (to_regclass('public.uq_admin_users_wecom_identity') IS NOT NULL)::int,
            (SELECT count(*) FROM admin_users WHERE id IN (${first_user_id}, ${second_user_id})),
            (SELECT count(*) FROM admin_sessions WHERE admin_user_id=${first_user_id}),
            (SELECT count(*) FROM admin_users WHERE auth_provider='wecom' AND wecom_corp_id='ww-si00b-corp-a' AND provider_subject_id='member-shared' AND id=${first_user_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$upgrade_waterline" = "28" && "$old_column" = "0" && "$new_column" = "1" &&
   "$check_constraint" = "1" && "$unique_constraint" = "1" && "$unique_index" = "1" &&
   "$accounts" = "2" && "$sessions" = "1" && "$located" = "1" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down
read -r rollback_waterline old_column new_column old_check old_unique old_index accounts sessions <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='provider_tenant_id'),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='wecom_corp_id'),
            (SELECT count(*) FROM pg_constraint WHERE conrelid='public.admin_users'::regclass AND conname='ck_admin_users_provider_tenant'),
            (SELECT count(*) FROM pg_constraint WHERE conrelid='public.admin_users'::regclass AND conname='uq_admin_users_provider_identity'),
            (to_regclass('public.uq_admin_users_provider_identity') IS NOT NULL)::int,
            (SELECT count(*) FROM admin_users WHERE id IN (${first_user_id}, ${second_user_id})),
            (SELECT count(*) FROM admin_sessions WHERE admin_user_id=${first_user_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$rollback_waterline" = "27" && "$old_column" = "1" && "$new_column" = "0" &&
   "$old_check" = "1" && "$old_unique" = "1" && "$old_index" = "1" &&
   "$accounts" = "2" && "$sessions" = "1" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r final_waterline old_column new_column accounts sessions distinct_corps <<<"$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='provider_tenant_id'),
            (SELECT count(*) FROM information_schema.columns
              WHERE table_schema='public' AND table_name='admin_users' AND column_name='wecom_corp_id'),
            (SELECT count(*) FROM admin_users WHERE id IN (${first_user_id}, ${second_user_id})),
            (SELECT count(*) FROM admin_sessions WHERE admin_user_id=${first_user_id}),
            (SELECT count(DISTINCT wecom_corp_id) FROM admin_users WHERE id IN (${first_user_id}, ${second_user_id}))
       FROM goose_db_version WHERE is_applied"
)"
[[ "$final_waterline" = "28" && "$old_column" = "0" && "$new_column" = "1" &&
   "$accounts" = "2" && "$sessions" = "1" && "$distinct_corps" = "2" ]]

psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "DELETE FROM admin_sessions WHERE admin_user_id IN (${first_user_id}, ${second_user_id});
   DELETE FROM admin_users WHERE id IN (${first_user_id}, ${second_user_id});"
printf 'SI00B Auth migration compatibility: PASS (27/28/27/28, CorpID accounts and existing session preserved)\n'
