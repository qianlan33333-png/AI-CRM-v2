#!/usr/bin/env bash
set -euo pipefail

: "${P4W0D01_AUTOMATION_TEST_DATABASE_URL:?P4W0D01_AUTOMATION_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4W0D01_AUTOMATION_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 24

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s -run '^TestD01MigrationHistoryFixture$' \
  ./acceptance/automation -args -database-url "$database_url"

history_event_id="$(
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
    "SELECT id FROM event_log WHERE idempotency_key LIKE 'd01-migration-history:%' ORDER BY id DESC LIMIT 1"
)"
[[ "$history_event_id" =~ ^[1-9][0-9]*$ ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r upgrade_waterline history_events delivery_table receipt_table delivery_index receipt_index <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM event_log WHERE id=${history_event_id}),
            (to_regclass('public.event_deliveries') IS NOT NULL)::int,
            (to_regclass('public.automation_trigger_receipts') IS NOT NULL)::int,
            (to_regclass('public.event_deliveries_consumer_state_idx') IS NOT NULL)::int,
            (to_regclass('public.automation_trigger_receipts_list_idx') IS NOT NULL)::int
       FROM goose_db_version WHERE is_applied"
)"
[[ "$upgrade_waterline" = "33" && "$history_events" = "1" ]]
[[ "$delivery_table" = "1" && "$receipt_table" = "1" && "$delivery_index" = "1" && "$receipt_index" = "1" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=60s \
  -run '^TestD01ContactProducerAndAutomationConsumerCloseOneObservableLoop$' \
  ./acceptance/automation -args -database-url "$database_url"

read -r receipt_id delivery_event_id river_job_id triggered_event_id <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT r.id,r.event_id,d.river_job_id,r.triggered_event_id
       FROM automation_trigger_receipts AS r
       JOIN event_deliveries AS d ON d.event_id=r.event_id AND d.consumer=r.consumer
      WHERE r.actor='admin:d01' AND r.state='triggered' AND d.status='completed'
      ORDER BY r.id DESC LIMIT 1"
)"
[[ "$receipt_id" =~ ^[1-9][0-9]*$ && "$delivery_event_id" =~ ^[1-9][0-9]*$ ]]
[[ "$river_job_id" =~ ^[1-9][0-9]*$ && "$triggered_event_id" =~ ^[1-9][0-9]*$ ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 24
read -r rollback_waterline history_events receipts deliveries jobs triggered_events <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM event_log WHERE id=${history_event_id}),
            (SELECT count(*) FROM automation_trigger_receipts WHERE id=${receipt_id} AND state='triggered'),
            (SELECT count(*) FROM event_deliveries WHERE event_id=${delivery_event_id} AND status='completed'),
            (SELECT count(*) FROM river_job WHERE id=${river_job_id}),
            (SELECT count(*) FROM event_log WHERE id=${triggered_event_id} AND event_type='automation.triggered')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$rollback_waterline" = "24" && "$history_events" = "1" ]]
[[ "$receipts" = "1" && "$deliveries" = "1" && "$jobs" = "1" && "$triggered_events" = "1" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r final_waterline history_events receipts deliveries jobs triggered_events <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (SELECT count(*) FROM event_log WHERE id=${history_event_id}),
            (SELECT count(*) FROM automation_trigger_receipts WHERE id=${receipt_id} AND state='triggered'),
            (SELECT count(*) FROM event_deliveries WHERE event_id=${delivery_event_id} AND status='completed'),
            (SELECT count(*) FROM river_job WHERE id=${river_job_id}),
            (SELECT count(*) FROM event_log WHERE id=${triggered_event_id} AND event_type='automation.triggered')
       FROM goose_db_version WHERE is_applied"
)"
[[ "$final_waterline" = "33" && "$history_events" = "1" ]]
[[ "$receipts" = "1" && "$deliveries" = "1" && "$jobs" = "1" && "$triggered_events" = "1" ]]

printf 'P4-W0-D01 migration compatibility: PASS (24/32/24/32, D01, L01, and current history preserved)\n'
