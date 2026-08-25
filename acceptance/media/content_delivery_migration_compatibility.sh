#!/usr/bin/env bash
set -euo pipefail

: "${P4MEDIADELIVERY_TEST_DATABASE_URL:?P4MEDIADELIVERY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4MEDIADELIVERY_TEST_DATABASE_URL"
database_url="${base_database_url/aicrm_test/aicrm_test_media_delivery_83}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_media_delivery_83 WITH (FORCE)' >/dev/null; }
trap cleanup EXIT
cleanup
psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_media_delivery_83' >/dev/null
MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME=aicrm_test_media_delivery_83 GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 83
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=83 AND is_applied')" = 1 ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('media_content_packages','media_attachment_uploads','media_campaign_delivery_bindings','outbound_media_acceptances') AND column_name ~* 'tenant|workspace|organization|public_url|object_storage'")" = 0 ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 82
[[ "$(psql "$database_url" -X -q -At -c "SELECT to_regclass('public.media_content_packages') IS NULL")" = t ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 83
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=83 AND is_applied')" = 1 ]]
printf 'P4 Media Content Delivery migration compatibility: PASS (83/82/83; self-applied only)\\n'
