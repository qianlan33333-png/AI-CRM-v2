#!/usr/bin/env bash
set -euo pipefail

: "${P4AIAUDIENCE_TEST_DATABASE_URL:?P4AIAUDIENCE_TEST_DATABASE_URL is required}"

base_database_url="$P4AIAUDIENCE_TEST_DATABASE_URL"
database_suffix='/aicrm_test?sslmode=disable'
[[ "$base_database_url" = *"$database_suffix" ]]
database_name='aicrm_test_ai_audience_00084'
database_url="${base_database_url%"$database_suffix"}/$database_name?sslmode=disable"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
guard_output="$(mktemp "${TMPDIR:-/tmp}/aicrm-ai-audience-00084-down.XXXXXX")"

MIGRATION_TEST_DATABASE_URL="$base_database_url" MIGRATION_TEST_DATABASE_NAME='aicrm_test' \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  rm -f "$guard_output"
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $database_name WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $database_name" >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME="$database_name" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

goose=("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url")
"${goose[@]}" up-to 84 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = '160014' ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 84 AND is_applied')" = '1' ]]

CI_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=240s -run '^TestLocalConfigurationSQLRepositoryPG16' ./acceptance/segment

"${goose[@]}" down-to 83 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 84 AND is_applied')" = '0' ]]
"${goose[@]}" up-to 84 >/dev/null

psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "
  WITH actor AS (
    INSERT INTO public.admin_users
      (auth_provider, wecom_corp_id, provider_subject_id, display_name, role)
    VALUES ('wecom', 'audience84-guard', 'audience84-guard', 'Audience 84 Guard', 'admin')
    RETURNING id
  ), segment AS (
    INSERT INTO public.segments (name, definition, refresh_mode, member_count, refresh_status)
    VALUES ('audience84-binding-version-guard', '{\"field\":\"is_deleted\",\"op\":\"eq\",\"value\":false}'::jsonb, 'manual', 0, 'idle')
    RETURNING id
  ), metadata AS (
    INSERT INTO public.ai_audience_package_metadata
      (segment_id, lifecycle, version, created_by, updated_by)
    SELECT segment.id, 'active', 1, actor.id, actor.id FROM segment, actor
    RETURNING segment_id, created_by
  ), agent AS (
    INSERT INTO public.automation_agent_configurations
      (agent_name, agent_code, automation_type, status, created_by, updated_by, created_at, updated_at)
    SELECT 'Audience 84 Guard', 'audience84_binding_guard', 'agent', 'active', actor.id, actor.id, now(), now() FROM actor
    RETURNING id
  )
  INSERT INTO public.ai_audience_package_automation_bindings
    (package_id, automation_agent_id, created_by, updated_by, version)
  SELECT metadata.segment_id, agent.id, metadata.created_by, metadata.created_by, 2 FROM metadata, agent"

if "${goose[@]}" down-to 83 >"$guard_output" 2>&1; then
  printf 'expected non-default automation binding version guard to fail\n' >&2
  exit 1
fi
grep -Fq 'cannot roll back populated AI Audience local configuration closure facts' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "
  DELETE FROM public.segments WHERE name = 'audience84-binding-version-guard';
  DELETE FROM public.automation_agent_configurations WHERE agent_code = 'audience84_binding_guard';
  DELETE FROM public.admin_users WHERE wecom_corp_id = 'audience84-guard' AND provider_subject_id = 'audience84-guard'"
"${goose[@]}" down-to 83 >/dev/null
"${goose[@]}" up-to 84 >/dev/null

psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "
  INSERT INTO public.ai_audience_local_configuration_receipts
    (operation, actor_id, key_digest, payload_digest, state, result_json, created_at, completed_at)
  VALUES
    ('configuration_version_put', 1, decode(repeat('aa', 32), 'hex'), decode(repeat('bb', 32), 'hex'),
     'completed', '{}'::jsonb, now(), now())"

if "${goose[@]}" down-to 83 >"$guard_output" 2>&1; then
  printf 'expected populated AI Audience 00084 down guard to fail\n' >&2
  exit 1
fi
grep -Fq 'cannot roll back populated AI Audience local configuration closure facts' "$guard_output"
grep -Fq 'SQLSTATE 55000' "$guard_output"
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id = 84 AND is_applied')" = '1' ]]

printf 'P4 AI Audience local configuration PG16.14: PASS (exact 84, HTTP-service UoW/ports/receipt replay-conflict-rollback, empty 84/83/84, binding-version and populated-receipt rollback guards; no provider)\n'
