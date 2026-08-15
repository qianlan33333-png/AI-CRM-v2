#!/usr/bin/env bash
set -euo pipefail

: "${P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL:?P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s \
  -run '^TestManualRetryFinalFailedCommitsOneNextGenerationAndStableReplay$' ./acceptance/outbound \
  -args -database-url "$database_url"

read -r receipt_id task_id event_id generation river_job_id <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT id,task_id,event_id,job_generation,river_job_id
       FROM outbound_control_receipts
      WHERE idempotency_key='outbound-manual-retry-0001' AND operation='manual_retry' AND state='completed'"
)"
[[ "$receipt_id" =~ ^[1-9][0-9]*$ && "$task_id" =~ ^[1-9][0-9]*$ && "$event_id" =~ ^[1-9][0-9]*$ ]]
[[ "$generation" = "2" && "$river_job_id" =~ ^[1-9][0-9]*$ ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 23
read -r rollback_waterline receipts links events jobs task_status <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM outbound_control_receipts WHERE id=${receipt_id} AND operation='manual_retry' AND state='completed'),
            (SELECT count(*) FROM outbound_task_job_links WHERE task_id=${task_id}),
            (SELECT count(*) FROM event_log WHERE id=${event_id} AND event_type='outbound.manual_retry_requested'),
            (SELECT count(*) FROM river_job WHERE id=${river_job_id}),
            (SELECT status FROM outbound_tasks WHERE id=${task_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$rollback_waterline" = "23" && "$receipts" = "1" && "$links" = "2" ]]
[[ "$events" = "1" && "$jobs" = "1" && "$task_status" = "pending" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r upgrade_waterline receipts links events jobs task_status <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM outbound_control_receipts WHERE id=${receipt_id} AND operation='manual_retry' AND state='completed'),
            (SELECT count(*) FROM outbound_task_job_links WHERE task_id=${task_id}),
            (SELECT count(*) FROM event_log WHERE id=${event_id} AND event_type='outbound.manual_retry_requested'),
            (SELECT count(*) FROM river_job WHERE id=${river_job_id}),
            (SELECT status FROM outbound_tasks WHERE id=${task_id})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$upgrade_waterline" = "37" && "$receipts" = "1" && "$links" = "2" ]]
[[ "$events" = "1" && "$jobs" = "1" && "$task_status" = "pending" ]]
