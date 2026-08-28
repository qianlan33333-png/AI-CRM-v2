#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

database_url="${CI_TEST_DATABASE_URL:-}"
dm01_source_url="${DM01_SOURCE_TEST_DATABASE_URL:-}"
dm01_target_url="${DM01_TARGET_TEST_DATABASE_URL:-}"
postgres_container="${POSTGRES_CONTAINER_ID:-}"
query_base="${QUERY_PLAN_BASE_SHA:-}"
query_head="${QUERY_PLAN_HEAD_SHA:-}"

fail() {
  printf 'ci-nightly: %s\n' "$1" >&2
  exit 2
}

[[ $# -eq 0 ]] || fail "unexpected argument"
[[ -n "$database_url" ]] || fail "CI_TEST_DATABASE_URL is required"
[[ -n "$dm01_source_url" ]] || fail "DM01_SOURCE_TEST_DATABASE_URL is required"
[[ -n "$dm01_target_url" ]] || fail "DM01_TARGET_TEST_DATABASE_URL is required"
[[ "$query_base" =~ ^[0-9a-f]{40}$ ]] || fail "QUERY_PLAN_BASE_SHA is invalid"
[[ "$query_head" =~ ^[0-9a-f]{40}$ ]] || fail "QUERY_PLAN_HEAD_SHA is invalid"
command -v go >/dev/null 2>&1 || fail "go is required"
command -v node >/dev/null 2>&1 || fail "node is required"
command -v npm >/dev/null 2>&1 || fail "npm is required"
command -v gitleaks >/dev/null 2>&1 || fail "gitleaks is required"
[[ "$(gitleaks version)" = "8.30.1" ]] || fail "gitleaks 8.30.1 is required"
[[ "$(node --version)" = "v24.18.0" ]] || fail "Node.js 24.18.0 is required"
[[ "$(npm --version)" = "11.12.1" ]] || fail "npm 11.12.1 is required"
if [[ -n "$postgres_container" ]]; then
  [[ "$(docker exec "$postgres_container" psql -U postgres -d aicrm_test -Atqc 'SHOW server_version_num')" = "160014" ]] ||
    fail "PostgreSQL 16.14 is required"
fi

python3 scripts/ci/test_selector.py
scripts/check_repo_contract.sh
scripts/test_repo_contract.sh

ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 \
ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 \
MIGRATION_TEST_DATABASE_URL="$database_url" \
make --no-print-directory migration-integration

go test -count=1 ./internal/survey/store -run '^TestSurveyUnresolvedHistoryPostgresRoundTripRollback$' -survey-unresolved-history-postgres-dsn="$database_url"

P0S03_PG_INTEGRATION=1 \
P0S03_TEST_DATABASE_URL="$database_url" \
ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$database_url" \
QUERY_PLAN_TEST_DATABASE_URL="$database_url" \
QUERY_PLAN_BASE_SHA="$query_base" \
QUERY_PLAN_HEAD_SHA="$query_head" \
make --no-print-directory ci-go

ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 \
ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 \
MIGRATION_TEST_DATABASE_URL="$database_url" \
CI_ACCEPTANCE_DATABASE_URL="$database_url" \
scripts/run_ci_acceptance_manifest.sh

P4_DM01_TEST_DATABASE_URL="$dm01_target_url" make --no-print-directory p4-dm01-migration-acceptance
make --no-print-directory p4-dm01-two-pg-acceptance

npm ci --ignore-scripts --no-audit --no-fund
npm run ci

gitleaks git . --config .gitleaks.toml --redact --no-banner --exit-code 1
scripts/test_gitleaks_config.sh
scripts/scan_sensitive_paths.sh
scripts/test_build_slice_bundle.sh
printf 'ci-nightly: PASS\n'
