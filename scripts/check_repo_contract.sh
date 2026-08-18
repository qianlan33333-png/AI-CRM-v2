#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
  printf 'repo-contract: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 0 ]] || fail "unexpected argument"
git diff --quiet -- . ||
  fail "working tree differs from the staged index; contract would be ambiguous"

for required_path in \
  .github/ci-map.yml \
  .github/workflows/ci.yml \
  .github/workflows/nightly.yml \
  scripts/ci/classify_changes.py \
  scripts/ci/test_selector.py \
  scripts/ci/run_full_regression.sh; do
  [[ -f "$required_path" && ! -L "$required_path" ]] ||
    fail "required CI path is not a regular file: $required_path"
done

expected_workflows=$'.github/workflows/ci.yml\n.github/workflows/nightly.yml'
actual_workflows="$(git -c core.quotePath=false ls-files -- '.github/workflows/*.yml' '.github/workflows/*.yaml' | LC_ALL=C sort)"
[[ "$actual_workflows" = "$expected_workflows" ]] ||
  fail "only ci.yml and nightly.yml may remain active workflows"

legacy_script_paths="$(git -c core.quotePath=false ls-files -- 'scripts/*promotion*' 'scripts/*candidate*merge*guard*')"
[[ -z "$legacy_script_paths" ]] ||
  fail "retired promotion or candidate-guard scripts remain tracked"

ruby -e 'require "yaml"; ARGV.each { |path| YAML.parse_file(path) }' \
  .github/workflows/ci.yml .github/workflows/nightly.yml >/dev/null ||
  fail "active workflow YAML is invalid"

for workflow in .github/workflows/ci.yml .github/workflows/nightly.yml; do
  mode="$(git ls-files -s -- "$workflow" | awk '{print $1}')"
  [[ "$mode" = "100644" ]] || fail "workflow must be tracked mode 100644: $workflow"

  source="$(git show ":$workflow")"
  [[ "$(grep -Fxc 'permissions:' <<<"$source" || true)" = "1" ]] ||
    fail "workflow permissions block must be unique: $workflow"
  [[ "$(grep -Fxc '  contents: read' <<<"$source" || true)" = "1" ]] ||
    fail "workflow must use contents: read: $workflow"
  ! grep -Eq '^[[:space:]]+[A-Za-z0-9_-]+:[[:space:]]+(write|write-all)[[:space:]]*$' <<<"$source" ||
    fail "workflow requests write permission: $workflow"
  ! grep -Fq 'pull_request_target' <<<"$source" ||
    fail "pull_request_target is forbidden: $workflow"
  ! grep -Eq '^[[:space:]]+paths(-ignore)?:' <<<"$source" ||
    fail "workflow-level path filters are forbidden: $workflow"

  while IFS= read -r action_ref; do
    [[ "$action_ref" =~ ^[^@[:space:]]+@[0-9a-f]{40}$ ]] ||
      fail "action reference is not pinned to a full commit: $workflow: $action_ref"
  done < <(sed -nE 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]+([^[:space:]#]+).*$/\2/p' <<<"$source")
done

ci_source="$(git show ':.github/workflows/ci.yml')"
nightly_source="$(git show ':.github/workflows/nightly.yml')"
full_regression="$(git show ':scripts/ci/run_full_regression.sh')"

for anchor in \
  '    name: ci / classify' \
  '    name: ci / secret-diff' \
  '    name: ci / self-test' \
  '    name: ci / merge-gate' \
  'python3 scripts/ci/test_selector.py' \
  'scripts/check_repo_contract.sh' \
  'scripts/test_repo_contract.sh'; do
  grep -Fq "$anchor" <<<"$ci_source" ||
    fail "PR CI lost required merge-gate or self-test wiring: $anchor"
done

merge_gate_block="$(awk '''
  /^  merge_gate:$/ { capture = 1; first = 1 }
  capture && !first && /^  [a-z][a-z0-9_]*:$/ { exit }
  capture { print; first = 0 }
''' <<<"$ci_source")"
grep -Fqx '    name: ci / merge-gate' <<<"$merge_gate_block" ||
  fail "merge gate name is missing"
grep -Fqx '    if: always()' <<<"$merge_gate_block" ||
  fail "merge gate must use if: always()"
for needed_job in classify secret_diff go_selected web api_codegen database shared_regression ci_self; do
  grep -Fqx "      - $needed_job" <<<"$merge_gate_block" ||
    fail "merge gate lost required dependency: $needed_job"
done

! grep -Fq 'pull_request:' <<<"$nightly_source" ||
  fail "nightly must not run as a pull-request gate"
for anchor in \
  '  schedule:' \
  '  workflow_dispatch:' \
  'scripts/ci/run_full_regression.sh'; do
  grep -Fq "$anchor" <<<"$nightly_source" ||
    fail "nightly lost a required trigger or full-regression entrypoint: $anchor"
done
! grep -Fq 'ci / merge-gate' <<<"$nightly_source" ||
  fail "nightly must not publish the required merge-gate context"

for anchor in \
  'python3 scripts/ci/test_selector.py' \
  'scripts/check_repo_contract.sh' \
  'scripts/test_repo_contract.sh' \
  'make --no-print-directory ci-go' \
  'make --no-print-directory migration-integration' \
  'scripts/run_ci_acceptance_manifest.sh' \
  'npm run ci' \
  'gitleaks git .' \
  'scripts/test_gitleaks_config.sh' \
  'scripts/scan_sensitive_paths.sh' \
  'scripts/test_build_slice_bundle.sh'; do
  grep -Fq "$anchor" <<<"$full_regression" ||
    fail "nightly full regression lost required coverage: $anchor"
done

promotion_token="ci"'_promotion'
promotion_smoke_token="check_ci_"'promotion_smoke'
candidate_token="candidate_"'merge_guard'
candidate_log_token="candidate-"'merge-guard'
legacy_workflow_tokens=(
  "application-"'go.yml'
  "repo-"'contract.yml'
  "secret-"'scan.yml'
)
executable_paths="$(git -c core.quotePath=false ls-files -- '.github/workflows/*' 'scripts/*' 'Makefile')"
for token in "$promotion_token" "$promotion_smoke_token" "$candidate_token" "$candidate_log_token" "${legacy_workflow_tokens[@]}"; do
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    if grep -Fq "$token" "$path"; then
      fail "retired CI chain is still referenced by executable path: $path"
    fi
  done <<<"$executable_paths"
done

metadata_tokens=(
  "github.event.pull_request."'title'
  "github.event.pull_request."'body'
  "evidence"' status'
  "no_schema"'_or_external_effect'
)
policy_paths="$(git -c core.quotePath=false ls-files -- '.github/ci-map.yml' '.github/workflows/ci.yml' 'scripts/ci/classify_changes.py' 'scripts/ci/*.sh')"
for token in "${metadata_tokens[@]}"; do
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    if grep -Fiq "$token" "$path"; then
      fail "PR metadata or governance text controls executable CI: $path"
    fi
  done <<<"$policy_paths"
done

printf 'repo-contract: PASS (relevance CI only)\n'
