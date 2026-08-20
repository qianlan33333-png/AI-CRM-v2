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

run_make_acceptance() {
  local environment_name="$1" target_name="$2"
  env "$environment_name=$database_url" make --no-print-directory "$target_name"
}

run_migration_checks() {
  make --no-print-directory migration-validate
  ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 \
    ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 \
    MIGRATION_TEST_DATABASE_URL="$database_url" \
    make --no-print-directory migration-integration
}

if [[ "$selection_mode" = "full" ]]; then
  run_migration_checks
  ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 \
    ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 \
    MIGRATION_TEST_DATABASE_URL="$database_url" \
    CI_ACCEPTANCE_DATABASE_URL="$database_url" \
    scripts/run_ci_acceptance_manifest.sh
  printf 'ci-database: PASS mode=full\n'
  exit 0
fi

run_migration_checks

# A migration-only pull request has no domain group. Its PR gate ends after the
# current schema has completed a fresh up/down/up cycle; the complete historical
# acceptance manifest remains in nightly.
if [[ -z "$selected_groups" ]]; then
  printf 'ci-database: PASS mode=selected groups=-\n'
  exit 0
fi

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
    adminops)
      run_make_acceptance P4ADMINOPS_TEST_DATABASE_URL p4-adminops-jobs-ab-acceptance
      ;;
    auth)
      run_make_acceptance P4A01_AUTH_TEST_DATABASE_URL p4-a01-auth-acceptance
      run_make_acceptance P4SI00B_AUTH_TEST_DATABASE_URL p4-si00b-auth-acceptance
      ;;
    automation)
      run_make_acceptance P4W0D01_AUTOMATION_TEST_DATABASE_URL p4-w0-d01-automation-acceptance
      run_make_acceptance P4AUTOMATIONAGENTSAB_TEST_DATABASE_URL p4-automation-agents-ab-acceptance
      ;;
    contact)
      run_make_acceptance P4C01_CHANNEL_TEST_DATABASE_URL p4-c01-channel-acceptance
      run_make_acceptance P4B02AB_TAG_TEST_DATABASE_URL p4-b02ab-tag-acceptance
      ;;
    coupon)
      run_make_acceptance P4J01_COUPON_TEST_DATABASE_URL p4-j01-coupon-acceptance
      run_make_acceptance P4COUPONAB_TEST_DATABASE_URL p4-coupon-ab-acceptance
      ;;
    events)
      run_make_acceptance P4INTERNAL_EVENTS_TEST_DATABASE_URL p4-internal-events-0367-0368-acceptance
      ;;
    identity)
      run_make_acceptance P3R4B_TEST_DATABASE_URL p3-r4b-identity-storage-acceptance
      ;;
    media)
      run_make_acceptance P4IMAGEDELETE_TEST_DATABASE_URL p4-image-delete-0362-acceptance
      P4H01A1_MEDIA_TEST_DATABASE_URL="$database_url" make --no-print-directory p4-h01a1-media-acceptance
      P4H03_MEDIA_TEST_DATABASE_URL="$database_url" make --no-print-directory p4-h03-media-acceptance
      P4MINIPROGRAMLIBRARY_TEST_DATABASE_URL="$database_url" make --no-print-directory p4-miniprogram-library-ab-acceptance
      P4IMAGEFACETS_TEST_DATABASE_URL="$database_url" \
        GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
        go test -race -count=1 -timeout=240s -run '^TestImageFacets0358' ./acceptance/media
      ;;
    operationcycle)
      run_make_acceptance P4OPERATIONCYCLE_TEST_DATABASE_URL p4-operation-cycle-ab-acceptance
      ;;
    order)
      run_make_acceptance P4I03_ORDER_TEST_DATABASE_URL p4-i03-order-acceptance
      run_make_acceptance P4ORDERAB_TEST_DATABASE_URL p4-order-ab-acceptance
      ;;
    outbound)
      run_make_acceptance P3O2_ENQUEUE_ONE_TEST_DATABASE_URL p3-o2-enqueue-one-acceptance
      run_make_acceptance P3O3_ENQUEUE_BATCH_TEST_DATABASE_URL p3-o3-enqueue-batch-acceptance
      run_make_acceptance P3O4_SENDER_TEST_DATABASE_URL p3-o4-sender-acceptance
      run_make_acceptance P3O5_STATUS_TEST_DATABASE_URL p3-o5-status-acceptance
      run_make_acceptance P3O6A_RETRY_TEST_DATABASE_URL p3-o6a-retry-acceptance
      run_make_acceptance P3O6B1_CANCEL_TEST_DATABASE_URL p3-o6b1-cancel-acceptance
      run_make_acceptance P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL p3-o6b2-manual-retry-acceptance
      run_make_acceptance P3O7_LEGACY_API_TEST_DATABASE_URL p3-o7-legacy-api-acceptance
      run_make_acceptance P4DELIVERYLINEAGE_TEST_DATABASE_URL p4-delivery-lineage-0308-acceptance
      ;;
    product)
      run_make_acceptance P4I01A_PRODUCT_TEST_DATABASE_URL p4-i01a-product-acceptance
      run_make_acceptance P4I01B_PRODUCT_TEST_DATABASE_URL p4-i01b-product-entitlement-acceptance
      ;;
    pushcenter)
      run_make_acceptance P4PUSHCENTER_TEST_DATABASE_URL p4-push-center-0421-0422-acceptance
      ;;
    segment)
      run_make_acceptance SEGMENT_CRUD_TEST_DATABASE_URL p3-s05b-acceptance
      ;;
    stats)
      run_make_acceptance P4W0L01_STATS_TEST_DATABASE_URL p4-w0-l01-stats-acceptance
      ;;
    survey)
      run_make_acceptance P4F01A_SURVEY_TEST_DATABASE_URL p4-f01a-survey-acceptance
      run_make_acceptance P4F01AB_SURVEY_TEST_DATABASE_URL p4-f01ab-survey-acceptance
      ;;
    wecom)
      run_make_acceptance WECOM_SYNC_TEST_DATABASE_URL p3-w4-acceptance
      run_make_acceptance P4MESSAGEARCHIVE_TEST_DATABASE_URL p4-message-archive-ab-acceptance
      ;;
    *)
      fail "unsupported selected database group: $group_name"
      ;;
  esac
done
printf 'ci-database: PASS mode=selected groups=%s\n' "$selected_groups"
