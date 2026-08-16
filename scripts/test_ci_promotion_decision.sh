#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

test_root="$(mktemp -d -t aicrm-v2-ci-promotion.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

merge_sha="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
base_sha="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
head_sha="cccccccccccccccccccccccccccccccccccccccc"
repository="example/aicrm"

fixture_name() {
  printf '%s' "$1" | tr '/?&=' '____' | tr -cd 'A-Za-z0-9_.-'
}

write_fixture() {
  local root="$1" endpoint="$2" payload="$3"
  printf '%s\n' "$payload" >"$root/$(fixture_name "$endpoint").json"
}

make_valid_case() {
  local root="$1"
  mkdir -p "$root"
  write_fixture "$root" "/repos/$repository/git/commits/$merge_sha" \
    "$(jq -n --arg sha "$merge_sha" --arg base "$base_sha" '{sha:$sha,tree:{sha:"same-tree"},parents:[{sha:$base}]}')"
  write_fixture "$root" "/repos/$repository/commits/$merge_sha/pulls?per_page=100" '[{"number":7}]'
  write_fixture "$root" "/repos/$repository/pulls/7" \
    "$(jq -n --arg merge "$merge_sha" --arg base "$base_sha" --arg head "$head_sha" '{state:"closed",merged_at:"2026-08-14T01:03:00Z",merge_commit_sha:$merge,base:{ref:"main",sha:$base},head:{sha:$head},changed_files:1}')"
  write_fixture "$root" "/repos/$repository/git/commits/$head_sha" \
    "$(jq -n --arg sha "$head_sha" '{sha:$sha,tree:{sha:"same-tree"},committer:{date:"2026-08-14T01:00:00Z"}}')"
  write_fixture "$root" "/repos/$repository/commits/$head_sha/check-runs?per_page=100" \
    "$(jq -n --arg head "$head_sha" '{check_runs:["application / go","application / web","policy / repo-contract","security / secret-scan"] | map({name:.,status:"completed",conclusion:"success",head_sha:$head,started_at:"2026-08-14T01:01:00Z",completed_at:"2026-08-14T01:02:00Z",app:{slug:"github-actions"}})}')"
  write_fixture "$root" "/repos/$repository/pulls/7/files?per_page=100" \
    "$(jq -n --arg sha "$head_sha" '[{filename:"internal/contact/app/service.go",status:"modified",sha:$sha}]')"
}

run_case() {
  local name="$1" root="$2" expected_mode="$3" expected_reason="$4"
  local output
  output="$(CI_PROMOTION_TEST_MODE=1 CI_PROMOTION_FIXTURE_DIR="$root" \
    CI_PROMOTION_EVENT_NAME=push CI_PROMOTION_REF=refs/heads/main \
    CI_PROMOTION_SHA="$merge_sha" GITHUB_REPOSITORY="$repository" GITHUB_ACTIONS=false \
    scripts/ci_promotion_decision.sh)"
  grep -Fq "mode=$expected_mode reason=$expected_reason" <<<"$output" || {
    printf 'ci-promotion-tests: %s failed: %s\n' "$name" "$output" >&2
    exit 1
  }
}

valid="$test_root/valid"
make_valid_case "$valid"
run_case valid "$valid" promotion same-tree-squash-provenance-verified

runtime_output="$test_root/promotion-output"
CI_PROMOTION_TEST_MODE=1 CI_PROMOTION_FIXTURE_DIR="$valid" \
  CI_PROMOTION_EVENT_NAME=push CI_PROMOTION_REF=refs/heads/main \
  CI_PROMOTION_SHA="$merge_sha" GITHUB_REPOSITORY="$repository" GITHUB_ACTIONS=false \
  GITHUB_OUTPUT="$runtime_output" RUNNER_TEMP="$valid" \
  scripts/ci_promotion_decision.sh >/dev/null
runtime_manifest="$(awk -F= '$1 == "affected_manifest" { print $2 }' "$runtime_output")"
[[ -f "$runtime_manifest" && ! -L "$runtime_manifest" ]] || {
  echo "ci-promotion-tests: affected manifest was not emitted" >&2
  exit 1
}
jq -e --arg sha "$head_sha" 'length == 1 and .[0].filename == "internal/contact/app/service.go" and .[0].sha == $sha' "$runtime_manifest" >/dev/null || {
  echo "ci-promotion-tests: affected manifest contents are invalid" >&2
  exit 1
}

direct_push="$test_root/direct-push"
make_valid_case "$direct_push"
write_fixture "$direct_push" "/repos/$repository/commits/$merge_sha/pulls?per_page=100" '[]'
run_case direct-push "$direct_push" full associated-pr-count-not-one

multiple_prs="$test_root/multiple-prs"
make_valid_case "$multiple_prs"
write_fixture "$multiple_prs" "/repos/$repository/commits/$merge_sha/pulls?per_page=100" '[{"number":7},{"number":8}]'
run_case multiple-prs "$multiple_prs" full associated-pr-count-not-one

missing_pr="$test_root/missing-pr"
make_valid_case "$missing_pr"
rm -f "$missing_pr/$(fixture_name "/repos/$repository/commits/$merge_sha/pulls?per_page=100").json"
run_case missing-pr "$missing_pr" full api-failure-associated-pulls

failed_check="$test_root/failed-check"
make_valid_case "$failed_check"
checks_file="$failed_check/$(fixture_name "/repos/$repository/commits/$head_sha/check-runs?per_page=100").json"
jq '(.check_runs[] | select(.name == "application / go")).conclusion = "failure"' "$checks_file" >"$checks_file.tmp"
mv "$checks_file.tmp" "$checks_file"
run_case failed-check "$failed_check" full failed-required-check

missing_check="$test_root/missing-check"
make_valid_case "$missing_check"
checks_file="$missing_check/$(fixture_name "/repos/$repository/commits/$head_sha/check-runs?per_page=100").json"
jq 'del(.check_runs[] | select(.name == "application / web"))' "$checks_file" >"$checks_file.tmp"
mv "$checks_file.tmp" "$checks_file"
run_case missing-check "$missing_check" full missing-required-check

tree_mismatch="$test_root/tree-mismatch"
make_valid_case "$tree_mismatch"
head_file="$tree_mismatch/$(fixture_name "/repos/$repository/git/commits/$head_sha").json"
jq '.tree.sha = "different-tree"' "$head_file" >"$head_file.tmp"
mv "$head_file.tmp" "$head_file"
run_case tree-mismatch "$tree_mismatch" full tree-mismatch

parent_mismatch="$test_root/parent-mismatch"
make_valid_case "$parent_mismatch"
commit_file="$parent_mismatch/$(fixture_name "/repos/$repository/git/commits/$merge_sha").json"
jq '.parents[0].sha = "dddddddddddddddddddddddddddddddddddddddd"' "$commit_file" >"$commit_file.tmp"
mv "$commit_file.tmp" "$commit_file"
run_case parent-base-mismatch "$parent_mismatch" full parent-base-mismatch

multiple_parents="$test_root/multiple-parents"
make_valid_case "$multiple_parents"
commit_file="$multiple_parents/$(fixture_name "/repos/$repository/git/commits/$merge_sha").json"
jq --arg base "$base_sha" '.parents = [{sha:$base},{sha:"dddddddddddddddddddddddddddddddddddddddd"}]' "$commit_file" >"$commit_file.tmp"
mv "$commit_file.tmp" "$commit_file"
run_case multiple-parents "$multiple_parents" full main-commit-parent-count-not-one

stale_check="$test_root/stale-check"
make_valid_case "$stale_check"
checks_file="$stale_check/$(fixture_name "/repos/$repository/commits/$head_sha/check-runs?per_page=100").json"
jq '(.check_runs[] | select(.name == "application / go")).completed_at = "2026-08-14T00:59:00Z"' "$checks_file" >"$checks_file.tmp"
mv "$checks_file.tmp" "$checks_file"
run_case stale-check "$stale_check" full stale-required-check

api_failure="$test_root/api-failure"
mkdir -p "$api_failure"
run_case api-failure "$api_failure" full api-failure-current-commit

for promoted in \
  migration:migrations/00032_example.sql \
  openapi:api/openapi.yaml \
  matrix:docs/feature-matrix.csv \
  api-mapping:docs/api-mapping.jsonl \
  migration-mapping:docs/migration-mapping.jsonl \
  acceptance-manifest:docs/ci/go-acceptance-manifest.tsv \
  repo-fingerprint:docs/ci/repo-contract-fingerprints.tsv \
  generated:internal/api/generated/server.gen.go \
  generated-fingerprint:scripts/generated-sources.sha256; do
  name="${promoted%%:*}"
  path="${promoted#*:}"
  root="$test_root/promoted-$name"
  make_valid_case "$root"
  files_file="$root/$(fixture_name "/repos/$repository/pulls/7/files?per_page=100").json"
  jq --arg path "$path" '.[0].filename = $path' "$files_file" >"$files_file.tmp"
  mv "$files_file.tmp" "$files_file"
  run_case "promoted-$name" "$root" promotion same-tree-squash-provenance-verified
done

for risk in \
  workflow:.github/workflows/application-go.yml \
  checker:scripts/check_repo_contract.sh \
  fingerprint-checker:scripts/check_repo_fingerprints.sh \
  matrix-checker:scripts/check_feature_matrix_contract.sh \
  merge-guard:scripts/check_candidate_merge_guard.sh \
  openapi-checker:tools/openapi-contract/main.go \
  acceptance-runner:scripts/run_ci_acceptance_manifest.sh \
  root-dep:go.mod \
  security-config:.gitleaks.toml \
  generator-config:api/oapi-codegen.yaml \
  auth-security-runtime:internal/auth/app/service.go \
  config-security-runtime:internal/config/registry.go \
  adminops-security-runtime:internal/adminops/app/service.go \
  gateway-security-runtime:internal/gateway/auth.go \
  auth-composition:cmd/aicrm/legacy_auth.go \
  config-composition:cmd/aicrm/legacy_config_settings.go \
  admin-composition:cmd/aicrm/legacy_admin_ops.go \
  migration-framework:migrations/README.md \
  shared-global:internal/platform/runtime/run.go; do
  name="${risk%%:*}"
  path="${risk#*:}"
  root="$test_root/risk-$name"
  make_valid_case "$root"
  files_file="$root/$(fixture_name "/repos/$repository/pulls/7/files?per_page=100").json"
  jq --arg path "$path" '.[0].filename = $path' "$files_file" >"$files_file.tmp"
  mv "$files_file.tmp" "$files_file"
  run_case "risk-$name" "$root" full framework-risk-change
done

for unsupported_status in removed renamed copied; do
  root="$test_root/status-$unsupported_status"
  make_valid_case "$root"
  files_file="$root/$(fixture_name "/repos/$repository/pulls/7/files?per_page=100").json"
  jq --arg status "$unsupported_status" '.[0].status = $status' "$files_file" >"$files_file.tmp"
  mv "$files_file.tmp" "$files_file"
  run_case "status-$unsupported_status" "$root" full unsupported-domain-change-status
done

unclassified="$test_root/unclassified"
make_valid_case "$unclassified"
files_file="$unclassified/$(fixture_name "/repos/$repository/pulls/7/files?per_page=100").json"
jq '.[0].filename = "unknown/business.txt"' "$files_file" >"$files_file.tmp"
mv "$files_file.tmp" "$files_file"
run_case unclassified "$unclassified" full unclassified-change

duplicate_path="$test_root/duplicate-path"
make_valid_case "$duplicate_path"
pr_file="$duplicate_path/$(fixture_name "/repos/$repository/pulls/7").json"
jq '.changed_files = 2' "$pr_file" >"$pr_file.tmp"
mv "$pr_file.tmp" "$pr_file"
files_file="$duplicate_path/$(fixture_name "/repos/$repository/pulls/7/files?per_page=100").json"
jq '.[1] = .[0]' "$files_file" >"$files_file.tmp"
mv "$files_file.tmp" "$files_file"
run_case duplicate-path "$duplicate_path" full unsupported-domain-change-status

untrusted_check="$test_root/untrusted-check"
make_valid_case "$untrusted_check"
checks_file="$untrusted_check/$(fixture_name "/repos/$repository/commits/$head_sha/check-runs?per_page=100").json"
jq '(.check_runs[] | select(.name == "application / go")).app.slug = "foreign-app"' "$checks_file" >"$checks_file.tmp"
mv "$checks_file.tmp" "$checks_file"
run_case untrusted-check "$untrusted_check" full untrusted-check-app

incomplete_check="$test_root/incomplete-check"
make_valid_case "$incomplete_check"
checks_file="$incomplete_check/$(fixture_name "/repos/$repository/commits/$head_sha/check-runs?per_page=100").json"
jq '(.check_runs[] | select(.name == "application / go")).status = "in_progress"' "$checks_file" >"$checks_file.tmp"
mv "$checks_file.tmp" "$checks_file"
run_case incomplete-check "$incomplete_check" full incomplete-required-check

current_sha_mismatch="$test_root/current-sha-mismatch"
make_valid_case "$current_sha_mismatch"
commit_file="$current_sha_mismatch/$(fixture_name "/repos/$repository/git/commits/$merge_sha").json"
jq '.sha = "dddddddddddddddddddddddddddddddddddddddd"' "$commit_file" >"$commit_file.tmp"
mv "$commit_file.tmp" "$commit_file"
run_case current-sha-mismatch "$current_sha_mismatch" full current-commit-sha-mismatch

head_sha_mismatch="$test_root/head-sha-mismatch"
make_valid_case "$head_sha_mismatch"
head_file="$head_sha_mismatch/$(fixture_name "/repos/$repository/git/commits/$head_sha").json"
jq '.sha = "dddddddddddddddddddddddddddddddddddddddd"' "$head_file" >"$head_file.tmp"
mv "$head_file.tmp" "$head_file"
run_case head-sha-mismatch "$head_sha_mismatch" full pr-head-sha-mismatch

echo "ci-promotion-tests: PASS"
