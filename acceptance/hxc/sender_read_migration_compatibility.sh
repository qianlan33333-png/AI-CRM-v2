#!/usr/bin/env bash
set -euo pipefail

: "${P4HXC_SENDER_TEST_DATABASE_URL:?P4HXC_SENDER_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4HXC_SENDER_TEST_DATABASE_URL"

# The manifest self-test substitutes executors and must not start PostgreSQL.
if [[ "$database_url" = "postgres://fixture" && -n "${CI_ACCEPTANCE_TEST_LOG:-}" ]]; then
  exit 0
fi

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

if [[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 47
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 47
fi

history_snapshot() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
    (SELECT count(*) FROM event_log),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM event_log value),
    (SELECT count(*) FROM admin_users),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM admin_users value),
    (SELECT count(*) FROM media_images),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM media_images value),
    (SELECT count(*) FROM questionnaires),
    (SELECT md5(COALESCE(string_agg(to_jsonb(value)::text,E'\\n' ORDER BY id),'')) FROM questionnaires value)"
}

base_history="$(history_snapshot)"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 48
read -r waterline table_exists columns_ok forbidden_columns <<<"$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -F ' ' -c "SELECT
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (to_regclass('public.hxc_sender_configs') IS NOT NULL)::int,
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='hxc_sender_configs' AND column_name IN ('id','sender_userid','display_name','priority','is_active','created_at','updated_at')),
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='hxc_sender_configs' AND column_name ~ '(tenant|corp|provider|raw)')")"
[[ "$waterline" = "48" && "$table_exists" = "1" && "$columns_ok" = "7" && "$forbidden_columns" = "0" ]]

set +e
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO hxc_sender_configs (id,sender_userid,priority) VALUES ('hxc-invalid-blank',' ',0)" >/dev/null 2>&1
blank_status=$?
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO hxc_sender_configs (id,sender_userid,priority) VALUES ('hxc-invalid-priority','hxc-invalid-priority',100001)" >/dev/null 2>&1
priority_status=$?
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO hxc_sender_configs (id,sender_userid,priority) VALUES (' hxc-invalid-id','hxc-invalid-id',0)" >/dev/null 2>&1
padded_id_status=$?
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO hxc_sender_configs (id,sender_userid,priority) VALUES ('hxc-invalid-userid',' hxc-invalid-userid',0)" >/dev/null 2>&1
padded_userid_status=$?
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "BEGIN; INSERT INTO hxc_sender_configs (id,sender_userid,priority) VALUES ('hxc-canonical-one','hxc-canonical-userid',0); ROLLBACK" >/dev/null 2>&1
canonical_insert_status=$?
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "BEGIN; INSERT INTO hxc_sender_configs (id,sender_userid,priority) VALUES ('hxc-canonical-one','hxc-canonical-userid',0); INSERT INTO hxc_sender_configs (id,sender_userid,priority) VALUES ('hxc-canonical-two','hxc-canonical-userid',0)" >/dev/null 2>&1
duplicate_sender_status=$?
set -e
[[ "$blank_status" -ne 0 && "$priority_status" -ne 0 && "$padded_id_status" -ne 0 && "$padded_userid_status" -ne 0 && "$canonical_insert_status" = "0" && "$duplicate_sender_status" -ne 0 ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=120s -run '^TestHXCSenderReadPostgreSQLMergeAndUnavailable$' \
  ./acceptance/hxc -args -database-url "$database_url"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 47
rollback_waterline="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")"
[[ "$rollback_waterline" = "47" && "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.hxc_sender_configs') IS NOT NULL)::int")" = "0" && "$(history_snapshot)" = "$base_history" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 48
final_waterline="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied")"
[[ "$final_waterline" = "48" && "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT (to_regclass('public.hxc_sender_configs') IS NOT NULL)::int")" = "1" && "$(history_snapshot)" = "$base_history" ]]

printf 'P4 HXC sender migration compatibility: PASS (47/48/47/48, Event/Auth/Media/Survey history preserved, local read only)\n'
