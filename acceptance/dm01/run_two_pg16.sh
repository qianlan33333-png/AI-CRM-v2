#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

source_url="${DM01_SOURCE_TEST_DATABASE_URL:-}"
target_url="${DM01_TARGET_TEST_DATABASE_URL:-}"
[[ -n "$source_url" ]] || { echo "DM01_SOURCE_TEST_DATABASE_URL is required" >&2; exit 2; }
[[ -n "$target_url" ]] || { echo "DM01_TARGET_TEST_DATABASE_URL is required" >&2; exit 2; }
[[ "$source_url" != "$target_url" ]] || { echo "DM01 source and target URLs must differ" >&2; exit 2; }

MIGRATION_TEST_DATABASE_URL="$source_url" MIGRATION_TEST_DATABASE_NAME=aicrm_test_dm01_source \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go run ./acceptance/fixtures/cmd/validate-database-url
MIGRATION_TEST_DATABASE_URL="$target_url" MIGRATION_TEST_DATABASE_NAME=aicrm_test_dm01_target \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go run ./acceptance/fixtures/cmd/validate-database-url

GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go tool -modfile=tools/go.mod goose -dir migrations postgres "$target_url" up-to 72
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go tool -modfile=tools/go.mod goose -dir migrations postgres "$target_url" down
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go tool -modfile=tools/go.mod goose -dir migrations postgres "$target_url" up-to 72
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go test -race -count=1 -timeout=300s ./acceptance/dm01 -args \
    -source-database-url "$source_url" -target-database-url "$target_url"

down_log="$(mktemp -t aicrm-dm01-target-down.XXXXXX)"
trap 'rm -f "$down_log"' EXIT
if GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go tool -modfile=tools/go.mod goose -dir migrations postgres "$target_url" down >"$down_log" 2>&1; then
  echo "DM01 materialized target unexpectedly allowed migration rollback" >&2
  exit 1
fi
grep -Fq 'SQLSTATE 55000' "$down_log"
