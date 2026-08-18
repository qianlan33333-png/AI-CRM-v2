#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

selection_mode="${1:-}"
selected_groups="${2:-}"
database_url="${CI_TEST_DATABASE_URL:-}"

fail() {
  printf 'ci-database: %s\n' "$1" >&2
  exit 2
}

[[ "$selection_mode" = "selected" || "$selection_mode" = "full" ]] ||
  fail "mode must be selected or full"
[[ -n "$database_url" ]] || fail "CI_TEST_DATABASE_URL is required"
command -v go >/dev/null 2>&1 || fail "go is required"

make --no-print-directory generate-check

if [[ "$selection_mode" = "full" ]]; then
  make --no-print-directory migration-validate
  ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 \
    ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 \
    MIGRATION_TEST_DATABASE_URL="$database_url" \
    make --no-print-directory migration-integration
  ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 \
    ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 \
    MIGRATION_TEST_DATABASE_URL="$database_url" \
    CI_ACCEPTANCE_DATABASE_URL="$database_url" \
    scripts/run_ci_acceptance_manifest.sh
  printf 'ci-database: PASS mode=full\n'
  exit 0
fi

[[ -n "$selected_groups" ]] || fail "selected mode requires at least one group"
remaining_groups="$selected_groups"
while [[ -n "$remaining_groups" ]]; do
  if [[ "$remaining_groups" = *,* ]]; then
    group_name="${remaining_groups%%,*}"
    remaining_groups="${remaining_groups#*,}"
  else
    group_name="$remaining_groups"
    remaining_groups=""
  fi
  case "$group_name" in
    media)
      P4H01A1_MEDIA_TEST_DATABASE_URL="$database_url" make --no-print-directory p4-h01a1-media-acceptance
      P4H03_MEDIA_TEST_DATABASE_URL="$database_url" make --no-print-directory p4-h03-media-acceptance
      P4MINIPROGRAMLIBRARY_TEST_DATABASE_URL="$database_url" make --no-print-directory p4-miniprogram-library-ab-acceptance
      P4IMAGEFACETS_TEST_DATABASE_URL="$database_url" \
        GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
        go test -race -count=1 -timeout=240s -run '^TestImageFacets0358' ./acceptance/media
      ;;
    *)
      fail "unsupported selected database group: $group_name"
      ;;
  esac
done
printf 'ci-database: PASS mode=selected groups=%s\n' "$selected_groups"
