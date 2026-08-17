#!/usr/bin/env bash
set -euo pipefail

: "${P3O6A_RETRY_TEST_DATABASE_URL:?P3O6A_RETRY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P3O6A_RETRY_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 22

/usr/bin/env -u BASH_ENV -u ENV \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=45s \
  -run '^TestOutboundRetryableFailureIsRetriedByRealRiver$' ./acceptance/outbound \
  -args -database-url "$database_url" -o6a-real-river

read -r marker_id history_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT marker.id, count(history.id)
       FROM outbound_send_attempts AS marker
       JOIN outbound_send_attempt_history AS history ON history.send_attempt_id=marker.id
      GROUP BY marker.id"
)"
[[ "$marker_id" =~ ^[1-9][0-9]*$ && "$history_count" = "2" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down

read -r rollback_waterline rollback_history <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id), (SELECT count(*) FROM outbound_send_attempt_history)
       FROM goose_db_version
      WHERE is_applied"
)"
[[ "$rollback_waterline" = "21" && "$rollback_history" = "2" ]]

old_marker_id="$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
    "INSERT INTO outbound_send_attempts (river_job_id, task_id, job_kind)
     SELECT river_job_id, task_id, job_kind
       FROM outbound_send_attempts
      WHERE id=${marker_id}
     ON CONFLICT (river_job_id) DO UPDATE
       SET river_job_id=EXCLUDED.river_job_id
     RETURNING outbound_send_attempts.id"
)"
[[ "$old_marker_id" = "$marker_id" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up

read -r upgrade_waterline upgrade_history marker_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM outbound_send_attempt_history),
            (SELECT count(*) FROM outbound_send_attempts WHERE river_job_id=(
              SELECT river_job_id FROM outbound_send_attempts WHERE id=${marker_id}
            ))
       FROM goose_db_version
      WHERE is_applied"
)"
[[ "$upgrade_waterline" = "45" && "$upgrade_history" = "2" && "$marker_count" = "1" ]]
