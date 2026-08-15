#!/usr/bin/env bash
set -euo pipefail

: "${P3O6B1_CANCEL_TEST_DATABASE_URL:?P3O6B1_CANCEL_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P3O6B1_CANCEL_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 23

/usr/bin/env -u BASH_ENV -u ENV \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s \
  -run '^TestCancelPendingTaskCommitsJobReceiptEventAndStableReplay$' ./acceptance/outbound \
  -args -database-url "$database_url"

read -r receipt_id task_id event_id generation river_job_id <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT receipt.id, receipt.task_id, receipt.event_id,
            receipt.job_generation, receipt.river_job_id
       FROM outbound_control_receipts AS receipt
      WHERE receipt.idempotency_key='outbound-cancel-0000001'
        AND receipt.state='completed'"
)"
[[ "$receipt_id" =~ ^[1-9][0-9]*$ && "$task_id" =~ ^[1-9][0-9]*$ && "$event_id" =~ ^[1-9][0-9]*$ ]]
[[ "$generation" = "1" && "$river_job_id" =~ ^[1-9][0-9]*$ ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down

read -r rollback_waterline receipts links events jobs task_status <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM outbound_control_receipts WHERE id=${receipt_id} AND state='completed'),
            (SELECT count(*) FROM outbound_task_job_links WHERE task_id=${task_id} AND generation=${generation} AND cancelled_at IS NOT NULL),
            (SELECT count(*) FROM event_log WHERE id=${event_id} AND event_type='outbound.cancelled'),
            (SELECT count(*) FROM river_job WHERE id=${river_job_id}),
            (SELECT status FROM outbound_tasks WHERE id=${task_id})
       FROM goose_db_version
      WHERE is_applied"
)"
[[ "$rollback_waterline" = "22" && "$receipts" = "1" && "$links" = "1" ]]
[[ "$events" = "1" && "$jobs" = "0" && "$task_status" = "cancelled" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up

read -r upgrade_waterline receipts links events jobs task_status <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM outbound_control_receipts WHERE id=${receipt_id} AND state='completed'),
            (SELECT count(*) FROM outbound_task_job_links WHERE task_id=${task_id} AND generation=${generation} AND cancelled_at IS NOT NULL),
            (SELECT count(*) FROM event_log WHERE id=${event_id} AND event_type='outbound.cancelled'),
            (SELECT count(*) FROM river_job WHERE id=${river_job_id}),
            (SELECT status FROM outbound_tasks WHERE id=${task_id})
       FROM goose_db_version
      WHERE is_applied"
)"
[[ "$upgrade_waterline" = "39" && "$receipts" = "1" && "$links" = "1" ]]
[[ "$events" = "1" && "$jobs" = "0" && "$task_status" = "cancelled" ]]

read -r outbound_links river_foreign_keys <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT
       (SELECT count(*) FROM outbound_task_job_links WHERE task_id=${task_id} AND river_job_id=${river_job_id}),
       (SELECT count(*)
          FROM pg_constraint AS constraint_row
          JOIN pg_class AS relation ON relation.oid=constraint_row.conrelid
         WHERE constraint_row.contype='f'
           AND relation.relname IN ('outbound_task_job_links','outbound_control_receipts')
           AND pg_get_constraintdef(constraint_row.oid) LIKE '%river_job%')"
)"
[[ "$outbound_links" = "1" && "$river_foreign_keys" = "0" ]]
