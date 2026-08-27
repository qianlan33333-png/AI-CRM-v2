#!/usr/bin/env bash
set -euo pipefail

: "${P4_V1_ARCHIVE_TEST_DATABASE_URL:?P4_V1_ARCHIVE_TEST_DATABASE_URL is required}"
base_database_url="$P4_V1_ARCHIVE_TEST_DATABASE_URL"
reader_password="$(python3 -c 'import secrets; print(secrets.token_urlsafe(24))')"
IFS=$'\t' read -r database_url source_database_url reader_database_url <<<"$(python3 - "$base_database_url" "$reader_password" <<'PY'
import sys
from urllib.parse import quote, urlsplit, urlunsplit

parsed = urlsplit(sys.argv[1])
if parsed.path != '/aicrm_test':
    raise SystemExit('unexpected V1 archive migration base database')
target = urlunsplit((parsed.scheme, parsed.netloc, '/aicrm_test_v1_archive_00107', parsed.query, parsed.fragment))
source = urlunsplit((parsed.scheme, parsed.netloc, '/aicrm_test_v1_archive_source_00107', parsed.query, parsed.fragment))
host = parsed.hostname or ''
if ':' in host:
    host = f'[{host}]'
reader_netloc = f'{quote("aicrm_v1_archive_fixture_reader", safe="")}:{quote(sys.argv[2], safe="")}@{host}'
if parsed.port:
    reader_netloc += f':{parsed.port}'
reader = urlunsplit((parsed.scheme, reader_netloc, '/aicrm_test_v1_archive_source_00107', parsed.query, parsed.fragment))
print('\t'.join((target, source, reader)))
PY
)"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
failure_log="$(mktemp -t aicrm-v1-archive.XXXXXX)"
full_result="$(mktemp -t aicrm-v1-archive-full.XXXXXX)"
reconcile_result="$(mktemp -t aicrm-v1-archive-reconcile.XXXXXX)"
source_dump="$(mktemp -t aicrm-v1-archive-source.XXXXXX)"

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 \
    -c 'DROP DATABASE IF EXISTS aicrm_test_v1_archive_00107 WITH (FORCE)' \
    -c 'DROP DATABASE IF EXISTS aicrm_test_v1_archive_source_00107 WITH (FORCE)' \
    -c 'DROP ROLE IF EXISTS aicrm_v1_archive_fixture_reader' >/dev/null
}
finish() {
  cleanup
  rm -f "$failure_log" "$full_result" "$reconcile_result" "$source_dump"
}
trap finish EXIT

MIGRATION_TEST_DATABASE_URL="$base_database_url" \
  MIGRATION_TEST_DATABASE_NAME=aicrm_test \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 \
  -c 'CREATE DATABASE aicrm_test_v1_archive_00107' \
  -c 'CREATE DATABASE aicrm_test_v1_archive_source_00107' >/dev/null
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 107 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = "160014" ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = "107" ]]

psql "$source_database_url" -X -q -v ON_ERROR_STOP=1 -v reader_password="$reader_password" <<'SQL' >/dev/null
CREATE TABLE fixture_contacts (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  api_token TEXT NOT NULL
);
INSERT INTO fixture_contacts VALUES (1,'A','sensitive-one'),(2,'B','sensitive-two');
CREATE TABLE fixture_keyless(value TEXT NOT NULL);
INSERT INTO fixture_keyless VALUES ('duplicate'),('duplicate');
CREATE TABLE campaigns (
  id BIGINT PRIMARY KEY, campaign_code TEXT NOT NULL, display_name TEXT NOT NULL,
  review_status TEXT NOT NULL, run_status TEXT NOT NULL, owner_userid TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL
);
INSERT INTO campaigns VALUES
  (11,'safe-v1','Safe V1','pending_review','draft','owner-a',now(),now()),
  (12,'active-v1','Active V1','approved','active','owner-a',now(),now());
CREATE TABLE campaign_steps (
  id BIGINT PRIMARY KEY, campaign_id BIGINT NOT NULL, campaign_segment_id BIGINT NOT NULL,
  step_index INTEGER NOT NULL, day_offset INTEGER NOT NULL, send_time TEXT NOT NULL,
  timezone TEXT NOT NULL, content_text TEXT NOT NULL
);
INSERT INTO campaign_steps VALUES
  (31,11,21,0,0,'09:30','Asia/Shanghai','safe local definition'),
  (32,12,22,0,0,'10:00','Asia/Shanghai','must remain archived');
CREATE TABLE questionnaires (id BIGINT PRIMARY KEY);
CREATE TABLE questionnaire_questions (id BIGINT PRIMARY KEY);
CREATE TABLE questionnaire_options (id BIGINT PRIMARY KEY);
CREATE TABLE questionnaire_submissions (id BIGINT PRIMARY KEY);
CREATE TABLE questionnaire_submission_answers (id BIGINT PRIMARY KEY);
CREATE TABLE miniprogram_library (id BIGINT PRIMARY KEY);
CREATE TABLE radar_links (id BIGINT PRIMARY KEY);
CREATE TABLE wechat_shop_orders (id BIGINT PRIMARY KEY);
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
CREATE ROLE aicrm_v1_archive_fixture_reader LOGIN PASSWORD :'reader_password';
GRANT CONNECT ON DATABASE aicrm_test_v1_archive_source_00107 TO aicrm_v1_archive_fixture_reader;
GRANT USAGE ON SCHEMA public TO aicrm_v1_archive_fixture_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO aicrm_v1_archive_fixture_reader;
ALTER ROLE aicrm_v1_archive_fixture_reader SET default_transaction_read_only=on;
SQL

export AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL="$reader_database_url"
export AICRM_V1_ARCHIVE_TARGET_DATABASE_URL="$database_url"
export AICRM_V1_ARCHIVE_SOURCE_HMAC_KEY="$(python3 -c 'import secrets; print(secrets.token_urlsafe(24))')"
export AICRM_V1_ARCHIVE_ENCRYPTION_KEY="$(python3 -c 'import secrets; print(secrets.token_urlsafe(24))')"
printf 'immutable V1 archive acceptance snapshot\n' >"$source_dump"
common_args=(
  --run-id v1-archive-acceptance
  --source-dump-path "$source_dump"
  --repository-sha aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  --batch-size 2
)
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./cmd/aicrm-v1-import --mode preflight >"$failure_log"
grep -Fq '"table_count":12' "$failure_log"
grep -Fq '"row_count":8' "$failure_log"
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./cmd/aicrm-v1-import --mode full "${common_args[@]}" >"$full_result"
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./cmd/aicrm-v1-import --mode reconcile "${common_args[@]}" >"$reconcile_result"
grep -Fq '"mode":"full"' "$full_result"
grep -Fq '"mode":"reconcile"' "$reconcile_result"

[[ "$(psql "$database_url" -X -q -At -c "SELECT phase FROM data_migration_runs WHERE run_id='v1-archive-acceptance'")" = "reconciled" ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*)||'|'||count(DISTINCT source_key_digest) FROM v1_archive_records WHERE run_id='v1-archive-acceptance'")" = "8|8" ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM v1_archive_records WHERE run_id='v1-archive-acceptance' AND table_id='public/fixture_keyless'")" = "2" ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT bool_and(position('sensitive-' in encode(ciphertext,'escape'))=0) FROM v1_archive_records WHERE run_id='v1-archive-acceptance'")" = "t" ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT redaction_metadata::text FROM v1_archive_records WHERE run_id='v1-archive-acceptance' AND table_id='public/fixture_contacts' ORDER BY source_ordinal LIMIT 1")" = '["api_token"]' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT (SELECT count(*) FROM external_effects)||'|'||(SELECT count(*) FROM outbound_tasks)||'|'||(SELECT count(*) FROM event_log)")" = "0|0|0" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 108 >/dev/null
[[ "$(psql "$database_url" -X -q -At -c 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')" = "108" ]]
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./cmd/aicrm-v1-domain-import \
  --domain campaign --archive-run-id v1-archive-acceptance --campaign-actors owner-a=7 >"$failure_log"
grep -Fq '"ImportedCampaigns":1' "$failure_log"
grep -Fq '"ImportedSteps":1' "$failure_log"
grep -Fq '"ArchivedRows":2' "$failure_log"
[[ "$(psql "$database_url" -X -q -At -c "SELECT approval_status||'|'||runtime_status FROM cloud_campaigns WHERE campaign_code='safe-v1'")" = "rejected|paused" ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT (SELECT count(*) FROM cloud_campaigns)||'|'||(SELECT count(*) FROM cloud_campaign_steps)||'|'||(SELECT count(*) FROM cloud_campaign_local_commands)')" = "1|1|0" ]]
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./cmd/aicrm-v1-domain-import \
  --mode reconcile --domain all --archive-run-id v1-archive-acceptance >"$failure_log"
grep -Fq '"selected_source_count":4' "$failure_log"
grep -Fq '"receipt_count":4' "$failure_log"
grep -Fq '"imported_count":2' "$failure_log"
grep -Fq '"archived_count":2' "$failure_log"
[[ "$(psql "$database_url" -X -q -At -c "SELECT receipt_count||'|'||verified_count FROM v1_domain_import_reconciliation_receipts WHERE import_version='v1-domain-a1'")" = "4|4" ]]
if psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO v1_domain_import_receipts (import_version,archive_run_id,adapter_id,table_id,source_key_digest,payload_digest,disposition,reason,verified) SELECT 'v1-domain-a1',run_id,adapter_id,table_id,source_key_digest,payload_digest,'archive','late_write',true FROM v1_archive_records WHERE run_id='v1-archive-acceptance' AND table_id='public/fixture_contacts' ORDER BY source_ordinal LIMIT 1" >"$failure_log" 2>&1; then
  echo 'reconciled V1 domain import unexpectedly allowed a late receipt' >&2
  exit 1
fi
grep -Fq 'V1 domain import is already reconciled' "$failure_log"

if psql "$database_url" -X -q -v ON_ERROR_STOP=1 \
  -c "UPDATE v1_domain_import_receipts SET reason='changed'" >"$failure_log" 2>&1; then
  echo 'V1 domain import receipt unexpectedly allowed mutation' >&2
  exit 1
fi
grep -Fq 'data migration facts are immutable' "$failure_log"
if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down >"$failure_log" 2>&1; then
  echo 'populated V1 domain import journal unexpectedly allowed migration rollback' >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' "$failure_log"
printf 'V1 full archive migration PG16 acceptance: PASS\n'
