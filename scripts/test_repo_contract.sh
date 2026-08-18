#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

test_root="$(mktemp -d -t aicrm-v2-relevance-contract.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fail() {
  printf 'repo-contract-tests: %s\n' "$*" >&2
  exit 1
}

refresh_manifest() {
  local fixture="$1"
  git -C "$fixture" add -A
}

run_contract() {
  local fixture="$1"
  (
    cd "$fixture"
    scripts/check_repo_contract.sh >/dev/null 2>&1
  )
}

expect_rejected() {
  local fixture="$1" label="$2"
  if run_contract "$fixture"; then
    fail "$label was accepted"
  fi
}

make_fixture() {
  local name="$1" fixture
  fixture="$test_root/$name"
  git clone --quiet "$test_root/baseline" "$fixture"
  printf '%s\n' "$fixture"
}

mkdir -p "$test_root/baseline/.github/workflows" "$test_root/baseline/scripts"
cp -a .github/ci-map.yml "$test_root/baseline/.github/"
cp -a .github/workflows/ci.yml .github/workflows/nightly.yml "$test_root/baseline/.github/workflows/"
mkdir -p "$test_root/baseline/scripts/ci"
cp -a scripts/ci/*.py scripts/ci/*.sh "$test_root/baseline/scripts/ci/"
cp -a \
  scripts/check_repo_contract.sh \
  scripts/test_repo_contract.sh \
  "$test_root/baseline/scripts/"
cp -a Makefile "$test_root/baseline/"
git -C "$test_root/baseline" init --quiet -b main
git -C "$test_root/baseline" config user.email ci@example.invalid
git -C "$test_root/baseline" config user.name ci-fixture
refresh_manifest "$test_root/baseline"
git -C "$test_root/baseline" commit --quiet -m baseline

baseline_fixture="$(make_fixture baseline-positive)"
run_contract "$baseline_fixture" || fail "valid relevance CI contract was rejected"
printf 'repo-contract-tests: baseline PASS\n'

docs_fixture="$(make_fixture docs-only)"
mkdir -p "$docs_fixture/docs/evidence/governance"
printf '%s\n' \
  'Evidence Status: Candidate; not-wired; no_schema_or_external_effect.' \
  'This is documentation only and must not control mergeability.' \
  >"$docs_fixture/docs/evidence/governance/rollout-proof.md"
printf '%s\n' '{"historical":"mapping text only"}' >"$docs_fixture/docs/api-mapping.jsonl"
refresh_manifest "$docs_fixture"
run_contract "$docs_fixture" || fail "ordinary docs/evidence/mapping change was rejected"
printf 'repo-contract-tests: docs-only PASS\n'

legacy_workflow_fixture="$(make_fixture legacy-workflow)"
legacy_workflow="$legacy_workflow_fixture/.github/workflows/application-"'go.yml'
printf '%s\n' 'name: legacy' 'on: [pull_request]' >"$legacy_workflow"
refresh_manifest "$legacy_workflow_fixture"
expect_rejected "$legacy_workflow_fixture" "retired workflow"
printf 'repo-contract-tests: retired-workflow PASS\n'

legacy_script_fixture="$(make_fixture legacy-script)"
legacy_script="$legacy_script_fixture/scripts/ci_"'promotion'"_decision.sh"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$legacy_script"
chmod +x "$legacy_script"
refresh_manifest "$legacy_script_fixture"
expect_rejected "$legacy_script_fixture" "retired promotion script"
printf 'repo-contract-tests: retired-script PASS\n'

legacy_reference_fixture="$(make_fixture legacy-reference)"
legacy_reference="scripts/check_"'candidate_''merge_guard'".sh"
printf '# legacy reference: %s\n' "$legacy_reference" >>"$legacy_reference_fixture/scripts/ci/run_web.sh"
refresh_manifest "$legacy_reference_fixture"
expect_rejected "$legacy_reference_fixture" "retired candidate-guard reference"
printf 'repo-contract-tests: retired-reference PASS\n'

missing_always_fixture="$(make_fixture merge-gate-not-always)"
python3 - "$missing_always_fixture/.github/workflows/ci.yml" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
source = path.read_text(encoding="utf-8")
old = "  merge_gate:\n    name: ci / merge-gate\n    if: always()"
new = "  merge_gate:\n    name: ci / merge-gate\n    if: success()"
if old not in source:
    raise SystemExit("merge-gate fixture anchor missing")
path.write_text(source.replace(old, new, 1), encoding="utf-8")
PY
refresh_manifest "$missing_always_fixture"
expect_rejected "$missing_always_fixture" "merge gate without always()"
printf 'repo-contract-tests: merge-gate PASS\n'

nightly_pr_fixture="$(make_fixture nightly-pull-request)"
python3 - "$nightly_pr_fixture/.github/workflows/nightly.yml" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
source = path.read_text(encoding="utf-8")
path.write_text(source.replace("on:\n", "on:\n  pull_request:\n", 1), encoding="utf-8")
PY
refresh_manifest "$nightly_pr_fixture"
expect_rejected "$nightly_pr_fixture" "nightly pull-request trigger"
printf 'repo-contract-tests: nightly-trigger PASS\n'

unpinned_fixture="$(make_fixture unpinned-action)"
python3 - "$unpinned_fixture/.github/workflows/ci.yml" <<'PY'
from pathlib import Path
import re
import sys
path = Path(sys.argv[1])
source = path.read_text(encoding="utf-8")
updated, count = re.subn(r"actions/checkout@[0-9a-f]{40}", "actions/checkout@v4", source, count=1)
if count != 1:
    raise SystemExit("checkout fixture anchor missing")
path.write_text(updated, encoding="utf-8")
PY
refresh_manifest "$unpinned_fixture"
expect_rejected "$unpinned_fixture" "unpinned action"
printf 'repo-contract-tests: action-pin PASS\n'

write_permission_fixture="$(make_fixture write-permission)"
sed -i.bak 's/^  contents: read$/  contents: write/' "$write_permission_fixture/.github/workflows/ci.yml"
rm -f "$write_permission_fixture/.github/workflows/ci.yml.bak"
refresh_manifest "$write_permission_fixture"
expect_rejected "$write_permission_fixture" "workflow write permission"
printf 'repo-contract-tests: permission PASS\n'

path_filter_fixture="$(make_fixture workflow-path-filter)"
python3 - "$path_filter_fixture/.github/workflows/ci.yml" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
source = path.read_text(encoding="utf-8")
old = "  pull_request:\n    branches: [main]\n"
new = "  pull_request:\n    branches: [main]\n    paths-ignore: [docs/**]\n"
if old not in source:
    raise SystemExit("pull_request fixture anchor missing")
path.write_text(source.replace(old, new, 1), encoding="utf-8")
PY
refresh_manifest "$path_filter_fixture"
expect_rejected "$path_filter_fixture" "workflow-level path filter"
printf 'repo-contract-tests: path-filter PASS\n'

metadata_fixture="$(make_fixture pr-metadata-policy)"
metadata_input="github.event.pull_request."'title'
printf '# forbidden policy input: %s\n' "$metadata_input" >>"$metadata_fixture/scripts/ci/classify_changes.py"
refresh_manifest "$metadata_fixture"
expect_rejected "$metadata_fixture" "PR metadata policy input"
printf 'repo-contract-tests: metadata-policy PASS\n'

printf 'repo-contract-tests: PASS cases=11\n'
