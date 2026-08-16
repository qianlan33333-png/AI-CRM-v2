#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

event_name="${CI_PROMOTION_EVENT_NAME:-${GITHUB_EVENT_NAME:-}}"
ref_name="${CI_PROMOTION_REF:-${GITHUB_REF:-}}"
commit_sha="${CI_PROMOTION_SHA:-${GITHUB_SHA:-}}"
repository="${GITHUB_REPOSITORY:-}"
api_url="${GITHUB_API_URL:-https://api.github.com}"
output_file="${GITHUB_OUTPUT:-}"

mode="full"
reason="not-an-eligible-main-push"
affected_manifest=""

emit() {
  printf 'ci-promotion: mode=%s reason=%s\n' "$mode" "$reason"
  if [[ -n "$output_file" ]]; then
    printf 'mode=%s\nreason=%s\n' "$mode" "$reason" >>"$output_file"
    if [[ -n "$affected_manifest" ]]; then
      printf 'affected_manifest=%s\n' "$affected_manifest" >>"$output_file"
    fi
  fi
}

fallback() {
  reason="$1"
  emit
  exit 0
}

[[ "$event_name" = "push" && "$ref_name" = "refs/heads/main" ]] || fallback "$reason"
[[ "$commit_sha" =~ ^[0-9a-f]{40}$ ]] || fallback "invalid-main-sha"
[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fallback "invalid-repository"

api_get() {
  local endpoint="$1"
  if [[ "${CI_PROMOTION_TEST_MODE:-}" = "1" ]]; then
    [[ "${GITHUB_ACTIONS:-}" != "true" ]] || return 1
    local fixture_name
    fixture_name="$(printf '%s' "$endpoint" | tr '/?&=' '____' | tr -cd 'A-Za-z0-9_.-')"
    [[ -f "${CI_PROMOTION_FIXTURE_DIR:?}/${fixture_name}.json" ]] || return 1
    command cat "${CI_PROMOTION_FIXTURE_DIR}/${fixture_name}.json"
    return
  fi
  [[ -n "${GITHUB_TOKEN:-}" ]] || return 1
  curl --fail --silent --show-error --location \
    --proto '=https' --proto-redir '=https' --tlsv1.2 \
    -H 'Accept: application/vnd.github+json' \
    -H "Authorization: Bearer ${GITHUB_TOKEN}" \
    -H 'X-GitHub-Api-Version: 2022-11-28' \
    "${api_url}${endpoint}"
}

valid_json() { jq -e . >/dev/null 2>&1; }

is_framework_risk_path() {
  local candidate_path="$1"
  if [[ "$candidate_path" = migrations/* && ! "$candidate_path" =~ ^migrations/[0-9]{5}_[A-Za-z0-9_]+\.sql$ ]]; then
    return 0
  fi
  [[ "$candidate_path" =~ ^(\.github/|scripts/|tools/|acceptance/fixtures/|internal/platform/|docs/(adr|architecture|governance|plans|spec|execution/slice-ledger\.yml|execution/implementation-plan\.md|evidence/phases/)|Makefile$|go\.(mod|sum)$|package(-lock)?\.json$|sqlc\.ya?ml$|\.gitleaks\.toml$|cmd/aicrm/main\.go$|cmd/aicrm/components\.go$) ]]
}

is_domain_artifact_path() {
  local candidate_path="$1"
  [[ "$candidate_path" =~ ^migrations/[0-9]{5}_[A-Za-z0-9_]+\.sql$ ]] ||
    [[ "$candidate_path" =~ ^api/[A-Za-z0-9._/-]+\.ya?ml$ ]] ||
    [[ "$candidate_path" =~ ^(internal/api/(candidate/)?generated|web/src/api/generated)/[A-Za-z0-9._/-]+$ ]] ||
    [[ "$candidate_path" =~ ^internal/(auth|automation|contact|coupon|identity|media|order|outbound|product|pushcenter|segment|stats|survey|wecom)/[A-Za-z0-9._/-]+$ ]] ||
    [[ "$candidate_path" =~ ^acceptance/(auth|automation|contact|coupon|identity|media|order|outbound|product|pushcenter|segment|stats|survey|wecom)/[A-Za-z0-9._/-]+$ ]] ||
    [[ "$candidate_path" =~ ^cmd/aicrm/legacy_[A-Za-z0-9._-]+\.go$ ]] ||
    [[ "$candidate_path" =~ ^docs/(feature-matrix\.csv|migration-mapping\.jsonl|api-mapping\.jsonl|execution/slices/[A-Za-z0-9._-]+\.md|evidence/slices/[A-Za-z0-9._-]+\.md|ci/go-acceptance-manifest\.tsv)$ ]] ||
    [[ "$candidate_path" = "scripts/generated-sources.sha256" ]]
}

commit_endpoint="/repos/${repository}/git/commits/${commit_sha}"
pulls_endpoint="/repos/${repository}/commits/${commit_sha}/pulls?per_page=100"
current_commit="$(api_get "$commit_endpoint")" || fallback "api-failure-current-commit"
pulls="$(api_get "$pulls_endpoint")" || fallback "api-failure-associated-pulls"
valid_json <<<"$current_commit" || fallback "api-invalid-current-commit"
valid_json <<<"$pulls" || fallback "api-invalid-associated-pulls"

[[ "$(jq 'length' <<<"$pulls")" = "1" ]] || fallback "associated-pr-count-not-one"
pr_number="$(jq -r '.[0].number // empty' <<<"$pulls")"
[[ "$pr_number" =~ ^[1-9][0-9]*$ ]] || fallback "invalid-associated-pr"
pr_endpoint="/repos/${repository}/pulls/${pr_number}"
pr="$(api_get "$pr_endpoint")" || fallback "api-failure-pull-request"
valid_json <<<"$pr" || fallback "api-invalid-pull-request"

[[ "$(jq -r '.state' <<<"$pr")" = "closed" && "$(jq -r '.merged_at // empty' <<<"$pr")" != "" ]] || fallback "associated-pr-not-merged"
[[ "$(jq -r '.base.ref' <<<"$pr")" = "main" ]] || fallback "pr-base-not-main"
[[ "$(jq -r '.merge_commit_sha' <<<"$pr")" = "$commit_sha" ]] || fallback "merge-commit-mismatch"

parent_count="$(jq '.parents | length' <<<"$current_commit")"
[[ "$parent_count" = "1" ]] || fallback "main-commit-parent-count-not-one"
parent_sha="$(jq -r '.parents[0].sha' <<<"$current_commit")"
base_sha="$(jq -r '.base.sha' <<<"$pr")"
head_sha="$(jq -r '.head.sha' <<<"$pr")"
[[ "$parent_sha" = "$base_sha" ]] || fallback "parent-base-mismatch"
[[ "$head_sha" =~ ^[0-9a-f]{40}$ ]] || fallback "invalid-pr-head-sha"

head_endpoint="/repos/${repository}/git/commits/${head_sha}"
head_commit="$(api_get "$head_endpoint")" || fallback "api-failure-pr-head"
valid_json <<<"$head_commit" || fallback "api-invalid-pr-head"
[[ "$(jq -r '.tree.sha' <<<"$current_commit")" = "$(jq -r '.tree.sha' <<<"$head_commit")" ]] || fallback "tree-mismatch"

checks_endpoint="/repos/${repository}/commits/${head_sha}/check-runs?per_page=100"
checks="$(api_get "$checks_endpoint")" || fallback "api-failure-check-runs"
valid_json <<<"$checks" || fallback "api-invalid-check-runs"
head_time="$(jq -r '.committer.date // empty' <<<"$head_commit")"
[[ -n "$head_time" ]] || fallback "missing-head-commit-time"

required_checks=(
  "application / go"
  "application / web"
  "policy / repo-contract"
  "security / secret-scan"
)
for context in "${required_checks[@]}"; do
  matching="$(jq --arg name "$context" '[.check_runs[] | select(.name == $name)] | sort_by(.completed_at // "") | reverse' <<<"$checks")"
  [[ "$(jq 'length' <<<"$matching")" -gt 0 ]] || fallback "missing-required-check"
  [[ "$(jq -r '.[0].status' <<<"$matching")" = "completed" ]] || fallback "incomplete-required-check"
  [[ "$(jq -r '.[0].conclusion' <<<"$matching")" = "success" ]] || fallback "failed-required-check"
  [[ "$(jq -r '.[0].head_sha' <<<"$matching")" = "$head_sha" ]] || fallback "check-head-mismatch"
  [[ "$(jq -r '.[0].app.slug // empty' <<<"$matching")" = "github-actions" ]] || fallback "untrusted-check-app"
  completed_at="$(jq -r '.[0].completed_at // empty' <<<"$matching")"
  [[ -n "$completed_at" && "$completed_at" > "$head_time" ]] || fallback "stale-required-check"
done

changed_files="$(jq -r '.changed_files' <<<"$pr")"
[[ "$changed_files" =~ ^[0-9]+$ && "$changed_files" -le 100 ]] || fallback "changed-files-unbounded"
files_endpoint="/repos/${repository}/pulls/${pr_number}/files?per_page=100"
files="$(api_get "$files_endpoint")" || fallback "api-failure-pull-files"
valid_json <<<"$files" || fallback "api-invalid-pull-files"
[[ "$(jq 'length' <<<"$files")" = "$changed_files" ]] || fallback "pull-files-count-mismatch"

affected_files="$(jq -ce '[.[] | {filename, status, sha}]' <<<"$files")" || fallback "api-invalid-pull-files"
jq -e '
  type == "array" and length > 0 and
  all(.[];
    (.filename | type == "string" and test("^[A-Za-z0-9._/-]+$") and (contains("..") | not)) and
    (.status == "added" or .status == "modified") and
    (.sha | type == "string" and test("^[0-9a-f]{40}$"))
  )
' <<<"$affected_files" >/dev/null || fallback "unsupported-domain-change-status"

while IFS= read -r file_path; do
  if is_framework_risk_path "$file_path"; then
    fallback "framework-risk-change"
  fi
  is_domain_artifact_path "$file_path" || fallback "unclassified-change"
done < <(jq -r '.[].filename' <<<"$affected_files")

if [[ -n "$output_file" ]]; then
  manifest_dir="${RUNNER_TEMP:-}"
  [[ -n "$manifest_dir" && -d "$manifest_dir" && ! -L "$manifest_dir" ]] || fallback "invalid-runner-temp"
  umask 077
  affected_manifest="$(mktemp "$manifest_dir/aicrm-v2-promotion-affected.XXXXXX")" || fallback "affected-manifest-create-failure"
  printf '%s\n' "$affected_files" >"$affected_manifest" || fallback "affected-manifest-write-failure"
  chmod 600 "$affected_manifest" || fallback "affected-manifest-mode-failure"
fi

mode="promotion"
reason="same-tree-squash-provenance-verified"
emit
