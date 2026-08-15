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
    "$(jq -n --arg base "$base_sha" '{tree:{sha:"same-tree"},parents:[{sha:$base}]}')"
  write_fixture "$root" "/repos/$repository/commits/$merge_sha/pulls?per_page=100" '[{"number":7}]'
  write_fixture "$root" "/repos/$repository/pulls/7" \
    "$(jq -n --arg merge "$merge_sha" --arg base "$base_sha" --arg head "$head_sha" '{state:"closed",merged_at:"2026-08-14T01:03:00Z",merge_commit_sha:$merge,base:{ref:"main",sha:$base},head:{sha:$head},changed_files:1}')"
  write_fixture "$root" "/repos/$repository/git/commits/$head_sha" \
    '{"tree":{"sha":"same-tree"},"committer":{"date":"2026-08-14T01:00:00Z"}}'
  write_fixture "$root" "/repos/$repository/commits/$head_sha/check-runs?per_page=100" \
    "$(jq -n --arg head "$head_sha" '{check_runs:["application / go","application / web","policy / repo-contract","security / secret-scan"] | map({name:.,status:"completed",conclusion:"success",head_sha:$head,completed_at:"2026-08-14T01:02:00Z",app:{slug:"github-actions"}})}')"
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

for promoted in migration:migrations/00032_example.sql openapi:api/openapi.yaml matrix:docs/feature-matrix.csv generated:internal/api/generated/server.gen.go; do
  name="${promoted%%:*}"
  path="${promoted#*:}"
  root="$test_root/promoted-$name"
  make_valid_case "$root"
  files_file="$root/$(fixture_name "/repos/$repository/pulls/7/files?per_page=100").json"
  jq --arg path "$path" '.[0].filename = $path' "$files_file" >"$files_file.tmp"
  mv "$files_file.tmp" "$files_file"
  run_case "promoted-$name" "$root" promotion same-tree-squash-provenance-verified
done

for risk in workflow:.github/workflows/application-go.yml checker:scripts/check_repo_contract.sh root-dep:go.mod migration-framework:migrations/README.md shared-global:internal/platform/runtime/run.go; do
  name="${risk%%:*}"
  path="${risk#*:}"
  root="$test_root/risk-$name"
  make_valid_case "$root"
  files_file="$root/$(fixture_name "/repos/$repository/pulls/7/files?per_page=100").json"
  jq --arg path "$path" '.[0].filename = $path' "$files_file" >"$files_file.tmp"
  mv "$files_file.tmp" "$files_file"
  run_case "risk-$name" "$root" full framework-risk-change
done

echo "ci-promotion-tests: PASS"
