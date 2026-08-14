#!/usr/bin/env bash
set -euo pipefail

: "${P4W0L01_STATS_TEST_DATABASE_URL:?P4W0L01_STATS_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
database_url="$P4W0L01_STATS_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

has_migration_table="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
  "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int")"
if [[ "$has_migration_table" = "0" ]]; then
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 25
else
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 25
fi

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=90s -run '^TestL01MigrationHistoryFixture$' \
  ./acceptance/stats -args -database-url "$database_url"

read -r history_event_id history_stats_job <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT e.id,d.river_job_id
       FROM event_log e
       JOIN event_deliveries d ON d.event_id=e.id AND d.consumer='stats.tag-applied.v1'
      WHERE e.payload->>'actor'='admin:l01-migration'
      ORDER BY e.id DESC LIMIT 1"
)"
[[ "$history_event_id" =~ ^[1-9][0-9]*$ && "$history_stats_job" =~ ^[1-9][0-9]*$ ]]
pre_upgrade_receipts=0
if [[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
  "SELECT (to_regclass('public.stats_event_receipts') IS NOT NULL)::int")" = "1" ]]; then
  pre_upgrade_receipts="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c \
    "SELECT count(*) FROM stats_event_receipts WHERE event_id=${history_event_id}")"
fi
[[ "$pre_upgrade_receipts" = "0" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r upgrade_waterline daily_table receipt_table daily_index <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
            (to_regclass('public.stats_daily') IS NOT NULL)::int,
            (to_regclass('public.stats_event_receipts') IS NOT NULL)::int,
            (to_regclass('public.stats_daily_metric_date_idx') IS NOT NULL)::int
       FROM goose_db_version WHERE is_applied"
)"
[[ "$upgrade_waterline" = "27" && "$daily_table" = "1" && "$receipt_table" = "1" && "$daily_index" = "1" ]]

/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=90s -run '^TestL01ConsumeMigrationHistory$' \
  ./acceptance/stats -args -database-url "$database_url"

read -r receipt_count projection_value delivery_status automation_receipt job_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT
       (SELECT count(*) FROM stats_event_receipts WHERE event_id=${history_event_id}),
       (SELECT value::bigint FROM stats_daily s JOIN event_log e
          ON e.id=${history_event_id}
         AND s.stat_date=(e.occurred_at AT TIME ZONE 'UTC')::date
         AND s.metric_key='customer.tag_applied'
         AND s.dims=jsonb_build_object('tag_id',(e.payload->>'tag_id')::bigint)),
       (SELECT status FROM event_deliveries WHERE event_id=${history_event_id} AND consumer='stats.tag-applied.v1'),
       (SELECT count(*) FROM automation_trigger_receipts WHERE event_id=${history_event_id} AND state='triggered'),
       (SELECT count(*) FROM river_job WHERE id=${history_stats_job})"
)"
[[ "$receipt_count" = "1" && "$projection_value" = "1" && "$delivery_status" = "completed" ]]
[[ "$automation_receipt" = "1" && "$job_count" = "1" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 25
read -r rollback_waterline receipt_count projection_value delivery_status automation_receipt job_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
       (SELECT count(*) FROM stats_event_receipts WHERE event_id=${history_event_id}),
       (SELECT value::bigint FROM stats_daily s JOIN event_log e
          ON e.id=${history_event_id}
         AND s.stat_date=(e.occurred_at AT TIME ZONE 'UTC')::date
         AND s.metric_key='customer.tag_applied'
         AND s.dims=jsonb_build_object('tag_id',(e.payload->>'tag_id')::bigint)),
       (SELECT status FROM event_deliveries WHERE event_id=${history_event_id} AND consumer='stats.tag-applied.v1'),
       (SELECT count(*) FROM automation_trigger_receipts WHERE event_id=${history_event_id} AND state='triggered'),
       (SELECT count(*) FROM river_job WHERE id=${history_stats_job})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$rollback_waterline" = "25" && "$receipt_count" = "1" && "$projection_value" = "1" ]]
[[ "$delivery_status" = "completed" && "$automation_receipt" = "1" && "$job_count" = "1" ]]

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up
read -r final_waterline receipt_count projection_value delivery_status automation_receipt job_count <<<"$(
  psql "$database_url" -X -v ON_ERROR_STOP=1 -At -F ' ' -c \
    "SELECT max(version_id),
       (SELECT count(*) FROM stats_event_receipts WHERE event_id=${history_event_id}),
       (SELECT value::bigint FROM stats_daily s JOIN event_log e
          ON e.id=${history_event_id}
         AND s.stat_date=(e.occurred_at AT TIME ZONE 'UTC')::date
         AND s.metric_key='customer.tag_applied'
         AND s.dims=jsonb_build_object('tag_id',(e.payload->>'tag_id')::bigint)),
       (SELECT status FROM event_deliveries WHERE event_id=${history_event_id} AND consumer='stats.tag-applied.v1'),
       (SELECT count(*) FROM automation_trigger_receipts WHERE event_id=${history_event_id} AND state='triggered'),
       (SELECT count(*) FROM river_job WHERE id=${history_stats_job})
       FROM goose_db_version WHERE is_applied"
)"
[[ "$final_waterline" = "27" && "$receipt_count" = "1" && "$projection_value" = "1" ]]
[[ "$delivery_status" = "completed" && "$automation_receipt" = "1" && "$job_count" = "1" ]]

printf 'P4-W0-L01 migration compatibility: PASS (25/27/25/27, history preserved through current waterline)\n'
