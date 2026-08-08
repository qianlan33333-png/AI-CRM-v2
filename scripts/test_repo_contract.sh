#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "repo-contract-tests: $*" >&2
  exit 1
}

for forbidden_git_env in \
  GIT_INDEX_FILE GIT_DIR GIT_WORK_TREE GIT_OBJECT_DIRECTORY \
  GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_QUARANTINE_PATH; do
  [[ -z "${!forbidden_git_env:-}" ]] ||
    fail "repository redirection environment is forbidden: $forbidden_git_env"
done

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
test_root="$(mktemp -d -t aicrm-v2-repo-contract-test.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

make_fixture() {
  local name="$1"
  local fixture="$test_root/$name"
  mkdir -p "$fixture"
  git -C "$repo_root" archive --format=tar "$(git -C "$repo_root" write-tree)" |
    tar -xf - -C "$fixture"
  git -C "$fixture" init -q
  git -C "$fixture" add -A
  printf '%s\n' "$fixture"
}

baseline_fixture="$(make_fixture baseline)"
if ! (cd "$baseline_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "valid staged baseline was rejected"
fi

missing_p0s03_contract_fixture="$(make_fixture missing-p0s03-contract)"
rm -f "$missing_p0s03_contract_fixture/acceptance/p0s03/static_contract.sh"
git -C "$missing_p0s03_contract_fixture" add -u acceptance/p0s03/static_contract.sh
if (cd "$missing_p0s03_contract_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing required P0-S03 contract file was accepted"
fi

broken_p0s03_acceptance_fixture="$(make_fixture broken-p0s03-acceptance)"
sed -i.bak -E \
  's/^p0-s03-acceptance: p0-s03-contract$/p0-s03-acceptance:/' \
  "$broken_p0s03_acceptance_fixture/Makefile"
rm -f "$broken_p0s03_acceptance_fixture/Makefile.bak"
git -C "$broken_p0s03_acceptance_fixture" add Makefile
if (cd "$broken_p0s03_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S03 acceptance target without contract dependency was accepted"
fi

broken_p0s03_ci_fixture="$(make_fixture broken-p0s03-ci-dependency)"
sed -i.bak -E \
  's/[[:space:]]p0-s03-acceptance$//' \
  "$broken_p0s03_ci_fixture/Makefile"
rm -f "$broken_p0s03_ci_fixture/Makefile.bak"
git -C "$broken_p0s03_ci_fixture" add Makefile
if (cd "$broken_p0s03_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without the P0-S03 acceptance dependency was accepted"
fi

hollow_p0s03_acceptance_fixture="$(make_fixture hollow-p0s03-acceptance)"
sed -i.bak \
  's#acceptance/p0s03/static_contract.sh#true#' \
  "$hollow_p0s03_acceptance_fixture/Makefile"
rm -f "$hollow_p0s03_acceptance_fixture/Makefile.bak"
git -C "$hollow_p0s03_acceptance_fixture" add Makefile
if (cd "$hollow_p0s03_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P0-S03 acceptance recipe was accepted"
fi

duplicate_p0s03_acceptance_fixture="$(make_fixture duplicate-p0s03-acceptance)"
printf '%s\n' '' 'p0-s03-acceptance: p0-s03-contract' $'\t@true' \
  >>"$duplicate_p0s03_acceptance_fixture/Makefile"
git -C "$duplicate_p0s03_acceptance_fixture" add Makefile
if (cd "$duplicate_p0s03_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "duplicate P0-S03 acceptance target was accepted"
fi

explicit_key_workflow_fixture="$(make_fixture unexpected-explicit-key-workflow)"
printf '%s\n' \
  'name: forbidden explicit key fixture' \
  'on: [push]' \
  'permissions:' \
  '  contents: read' \
  'jobs:' \
  '  probe:' \
  '    runs-on: ubuntu-latest' \
  '    steps:' \
  '      - ? |-' \
  '          uses' \
  '        : actions/checkout@v4' \
  >"$explicit_key_workflow_fixture/.github/workflows/explicit-key.yml"
git -C "$explicit_key_workflow_fixture" add .github/workflows/explicit-key.yml
if (cd "$explicit_key_workflow_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "unexpected workflow with an explicit block uses key was accepted"
fi

tagged_key_workflow_fixture="$(make_fixture unexpected-tagged-key-workflow)"
printf '%s\n' \
  'name: forbidden tagged key fixture' \
  'on: [push]' \
  'permissions:' \
  '  contents: read' \
  'jobs:' \
  '  probe:' \
  '    runs-on: ubuntu-latest' \
  '    steps:' \
  '      - ? !<tag:yaml.org,2002:str> |-' \
  '          uses' \
  '        : actions/checkout@v4' \
  >"$tagged_key_workflow_fixture/.github/workflows/tagged-key.yml"
git -C "$tagged_key_workflow_fixture" add .github/workflows/tagged-key.yml
if (cd "$tagged_key_workflow_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "unexpected workflow with a tagged block uses key was accepted"
fi

escaped_yaml_fixture="$(make_fixture escaped-yaml-policy)"
escaped_yaml_tmp="$escaped_yaml_fixture/.github/workflows/application-go.yml.tmp"
awk '
  { print }
  $0 == "    steps:" {
    slash = sprintf("%c", 92)
    print "      - { \"" slash "x75ses\": \"actions/checkout@" slash "x764\" }"
  }
' "$escaped_yaml_fixture/.github/workflows/application-go.yml" >"$escaped_yaml_tmp"
mv "$escaped_yaml_tmp" "$escaped_yaml_fixture/.github/workflows/application-go.yml"
git -C "$escaped_yaml_fixture" add .github/workflows/application-go.yml
grep -Fq '\x75ses' "$escaped_yaml_fixture/.github/workflows/application-go.yml" ||
  fail "failed to construct YAML escape fixture"
if (cd "$escaped_yaml_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "YAML escape that decodes to an unpinned uses key was accepted"
fi

overwritten_workflows_fixture="$(make_fixture overwritten-central-workflows)"
cp "$overwritten_workflows_fixture/.github/workflows/application-go.yml" \
  "$overwritten_workflows_fixture/.github/workflows/repo-contract.yml"
cp "$overwritten_workflows_fixture/.github/workflows/application-go.yml" \
  "$overwritten_workflows_fixture/.github/workflows/secret-scan.yml"
git -C "$overwritten_workflows_fixture" add .github/workflows
if (cd "$overwritten_workflows_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "overwritten central policy workflows were accepted"
fi

workflow_symlink_fixture="$(make_fixture central-workflow-symlink)"
rm -f "$workflow_symlink_fixture/.github/workflows/repo-contract.yml"
ln -s application-go.yml \
  "$workflow_symlink_fixture/.github/workflows/repo-contract.yml"
git -C "$workflow_symlink_fixture" add .github/workflows/repo-contract.yml
if (cd "$workflow_symlink_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "central policy workflow symlink was accepted"
fi

unpinned_fixture="$(make_fixture unpinned-action)"
sed -i.bak -E \
  's#actions/checkout@[0-9a-f]{40}#actions/checkout@v4#' \
  "$unpinned_fixture/.github/workflows/repo-contract.yml"
rm -f "$unpinned_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$unpinned_fixture" add .github/workflows/repo-contract.yml
grep -q 'actions/checkout@v4' \
  "$unpinned_fixture/.github/workflows/repo-contract.yml" ||
  fail "failed to construct unpinned Action fixture"
if (cd "$unpinned_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "unpinned GitHub Action was accepted"
fi

nonhex_fixture="$(make_fixture nonhex-action-ref)"
sed -i.bak -E \
  's#actions/checkout@[0-9a-f]{40}#actions/checkout@zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz#' \
  "$nonhex_fixture/.github/workflows/repo-contract.yml"
rm -f "$nonhex_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$nonhex_fixture" add .github/workflows/repo-contract.yml
if (cd "$nonhex_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "40-character non-hex Action reference was accepted"
fi

quoted_uses_fixture="$(make_fixture quoted-uses-key)"
sed -i.bak -E \
  's/^([[:space:]]*)uses: actions\/checkout@[0-9a-f]{40}/\1"uses": actions\/checkout@v4/' \
  "$quoted_uses_fixture/.github/workflows/repo-contract.yml"
rm -f "$quoted_uses_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$quoted_uses_fixture" add .github/workflows/repo-contract.yml
grep -q '"uses": actions/checkout@v4' \
  "$quoted_uses_fixture/.github/workflows/repo-contract.yml" ||
  fail "failed to construct quoted uses fixture"
if (cd "$quoted_uses_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "quoted uses key with an unpinned Action was accepted"
fi

flow_uses_fixture="$(make_fixture flow-uses-key)"
sed -i.bak \
  '/^    steps:$/a\
      - { name: Unpinned flow, uses: actions/checkout@v4 }
' \
  "$flow_uses_fixture/.github/workflows/repo-contract.yml"
rm -f "$flow_uses_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$flow_uses_fixture" add .github/workflows/repo-contract.yml
grep -q -- 'uses: actions/checkout@v4' \
  "$flow_uses_fixture/.github/workflows/repo-contract.yml" ||
  fail "failed to construct flow-style uses fixture"
if (cd "$flow_uses_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "flow-style uses key with an unpinned Action was accepted"
fi

write_permission_fixture="$(make_fixture write-permission)"
sed -i.bak \
  's/^  contents: read$/  issues: write/' \
  "$write_permission_fixture/.github/workflows/application-go.yml"
rm -f "$write_permission_fixture/.github/workflows/application-go.yml.bak"
git -C "$write_permission_fixture" add .github/workflows/application-go.yml
if (cd "$write_permission_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow issues: write permission was accepted"
fi

quoted_write_fixture="$(make_fixture quoted-write-permission)"
sed -i.bak \
  '/^  contents: read$/a\
  issues: "write"
' \
  "$quoted_write_fixture/.github/workflows/application-go.yml"
rm -f "$quoted_write_fixture/.github/workflows/application-go.yml.bak"
git -C "$quoted_write_fixture" add .github/workflows/application-go.yml
if (cd "$quoted_write_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "quoted workflow issues: write permission was accepted"
fi

write_all_fixture="$(make_fixture write-all-permission)"
sed -i.bak \
  's/^permissions:$/permissions: write-all/' \
  "$write_all_fixture/.github/workflows/application-go.yml"
rm -f "$write_all_fixture/.github/workflows/application-go.yml.bak"
git -C "$write_all_fixture" add .github/workflows/application-go.yml
if (cd "$write_all_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow permissions: write-all was accepted"
fi

secrets_inherit_fixture="$(make_fixture secrets-inherit)"
sed -i.bak \
  's/^    steps:$/    "secrets": inherit/' \
  "$secrets_inherit_fixture/.github/workflows/application-go.yml"
rm -f "$secrets_inherit_fixture/.github/workflows/application-go.yml.bak"
git -C "$secrets_inherit_fixture" add .github/workflows/application-go.yml
if (cd "$secrets_inherit_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow secrets: inherit was accepted"
fi

secrets_context_fixture="$(make_fixture secrets-context)"
sed -i.bak \
  's/^      GOTOOLCHAIN: local$/      GOTOOLCHAIN: ${{ secrets }}/' \
  "$secrets_context_fixture/.github/workflows/application-go.yml"
rm -f "$secrets_context_fixture/.github/workflows/application-go.yml.bak"
git -C "$secrets_context_fixture" add .github/workflows/application-go.yml
if (cd "$secrets_context_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow secrets context was accepted"
fi

envrc_fixture="$(make_fixture envrc-path)"
touch "$envrc_fixture/.envrc"
git -C "$envrc_fixture" add -f .envrc
if (cd "$envrc_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail ".envrc path was accepted"
fi

runtime_state_fixture="$(make_fixture runtime-state-path)"
mkdir -p "$runtime_state_fixture/runtime"
touch "$runtime_state_fixture/runtime/state.json"
git -C "$runtime_state_fixture" add -f runtime/state.json
if (cd "$runtime_state_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "root runtime state path was accepted"
fi

secret_fixture="$(make_fixture staged-secret)"
safe_readme="$test_root/safe-readme.md"
cp "$secret_fixture/README.md" "$safe_readme"
fake_secret="AKI""A0000000000000000"
sed -i.bak "1s/$/ ${fake_secret}/" "$secret_fixture/README.md"
rm -f "$secret_fixture/README.md.bak"
git -C "$secret_fixture" add README.md
cp "$safe_readme" "$secret_fixture/README.md"
if (cd "$secret_fixture" && scripts/scan_sensitive_paths.sh >/dev/null 2>&1); then
  fail "secret staged in the index but absent from the worktree was accepted"
fi

echo "repo-contract-tests: PASS"
