#!/usr/bin/env bash
set -euo pipefail

: "${MIGRATION_TEST_DATABASE_URL:?MIGRATION_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"

# The acceptance database URL is deliberately fixed and locally validated
# before this destructive test-fixture reset. It cannot address a deployment
# database.
MIGRATION_TEST_DATABASE_URL="$MIGRATION_TEST_DATABASE_URL" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

psql "$MIGRATION_TEST_DATABASE_URL" -X -q -v ON_ERROR_STOP=1 -c \
  'DROP SCHEMA public CASCADE; CREATE SCHEMA public' >/dev/null
