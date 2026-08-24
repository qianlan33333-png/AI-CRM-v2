#!/usr/bin/env bash
set -euo pipefail

: "${P4MEMBERGRID_TEST_DATABASE_URL:?P4MEMBERGRID_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4MEMBERGRID_TEST_DATABASE_URL"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=300s -run '^TestLaneD2MemberGridRepositoryPG16_14$' ./acceptance/product \
  -args -database-url "$base_database_url"
