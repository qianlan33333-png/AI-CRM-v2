#!/usr/bin/env bash
set -euo pipefail

# Domain-owned destructive acceptance, wired through p4-f01ab-survey-acceptance.
# It explicitly targets migration 52, proves 52->51->52 even when newer
# migrations are present, and restores the latest waterline after cleaning the
# anonymous fixture rows.
: "${P4SURVEY_PUBLIC_TEST_DATABASE_URL:?P4SURVEY_PUBLIC_TEST_DATABASE_URL is required}"
base_database_url="$P4SURVEY_PUBLIC_TEST_DATABASE_URL"
temporary_database="aicrm_test_f01public"
db_url="${base_database_url/aicrm_test/$temporary_database}"
root="$(cd "$(dirname "$0")/../.." && pwd)"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go run "$root/acceptance/fixtures/cmd/validate-database-url"
cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
MIGRATION_TEST_DATABASE_URL="$db_url" MIGRATION_TEST_DATABASE_NAME="$temporary_database" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go run "$root/acceptance/fixtures/cmd/validate-database-url"
goose=(go tool -modfile="$root/tools/go.mod" goose -dir "$root/migrations" postgres "$db_url")
"${goose[@]}" up
psql "$db_url" -v ON_ERROR_STOP=1 -Atqc "SELECT 1 FROM goose_db_version WHERE version_id=52 AND is_applied" >/dev/null
"${goose[@]}" down-to 51
test "$(psql "$db_url" -v ON_ERROR_STOP=1 -Atqc "SELECT CASE WHEN to_regclass('public.questionnaire_public_definitions') IS NULL AND to_regclass('public.questionnaires') IS NOT NULL AND to_regclass('public.event_log') IS NOT NULL THEN 1 ELSE 0 END")" = 1
"${goose[@]}" up-to 52
test "$(psql "$db_url" -v ON_ERROR_STOP=1 -Atqc "SELECT CASE WHEN to_regclass('public.questionnaire_public_definitions') IS NOT NULL AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='questionnaire_public_definitions_immutable') THEN 1 ELSE 0 END")" = 1
test "$(psql "$db_url" -v ON_ERROR_STOP=1 -Atqc "SELECT CASE WHEN NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'questionnaire_public_%' AND column_name IN ('mobile','phone','openid','unionid','external_user_id','customer_name','redirect_url','result_token')) THEN 1 ELSE 0 END")" = 1
slug="survey-lock-$(date +%s)"
qid="$(psql "$db_url" -v ON_ERROR_STOP=1 -Atqc "INSERT INTO questionnaires(slug,name,title,description,answer_display_mode,assessment_enabled,assessment_config,is_disabled,created_by,version,submission_count,created_at,updated_at) VALUES ('$slug','lock','lock','', 'all_in_one',false,'{}',false,1,1,0,now(),now()) RETURNING id")"
did="$(psql "$db_url" -v ON_ERROR_STOP=1 -Atqc "INSERT INTO questionnaire_public_definitions(questionnaire_id,definition_version,slug,state,answer_display_mode,title,description,created_at,published_at) VALUES ($qid,1,'$slug','public','all_in_one','lock','',now(),now()) RETURNING id")"
(psql "$db_url" -v ON_ERROR_STOP=1 -c "BEGIN; SELECT id FROM questionnaire_public_definitions WHERE id=$did AND state='public' FOR SHARE; SELECT pg_sleep(2); COMMIT;" >/dev/null) & locker=$!
sleep 0.2
if psql "$db_url" -v ON_ERROR_STOP=1 -c "SET statement_timeout='250ms'; UPDATE questionnaire_public_definitions SET state='disabled',disabled_at=now() WHERE id=$did;" >/dev/null 2>&1; then echo 'disable unexpectedly bypassed submit shared lock' >&2; exit 1; fi
wait "$locker"
psql "$db_url" -v ON_ERROR_STOP=1 -c "UPDATE questionnaire_public_definitions SET state='disabled',disabled_at=now() WHERE id=$did;" >/dev/null
test "$(psql "$db_url" -v ON_ERROR_STOP=1 -Atqc "SELECT CASE WHEN NOT EXISTS(SELECT 1 FROM questionnaire_public_definitions WHERE slug='$slug' AND state='public') AND NOT EXISTS(SELECT 1 FROM questionnaire_public_submissions WHERE definition_id=$did) THEN 1 ELSE 0 END")" = 1
"${goose[@]}" down-to 51
"${goose[@]}" up-to 52
"${goose[@]}" up
