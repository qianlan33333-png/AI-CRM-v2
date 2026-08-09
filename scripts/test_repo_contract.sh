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

for path in scripts/ownership/main.go scripts/test_ownership.sh docs/architecture/table-ownership.yml; do
  ownership_receipt_fixture="$(make_fixture "ownership-receipt-${path##*/}")"
  printf '%s\n' '# ownership receipt drift' >>"$ownership_receipt_fixture/$path"
  git -C "$ownership_receipt_fixture" add "$path"
  if (cd "$ownership_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "ownership lint receipt drift was accepted: $path"
  fi
done

ownership_runner_mode_fixture="$(make_fixture ownership-runner-mode)"
chmod 644 "$ownership_runner_mode_fixture/scripts/test_ownership.sh"
git -C "$ownership_runner_mode_fixture" add scripts/test_ownership.sh
if (cd "$ownership_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ownership lint runner mode drift was accepted"
fi

broken_ownership_ci_fixture="$(make_fixture broken-ownership-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]ownership-lint-test([[:space:]]|$)/\1/' "$broken_ownership_ci_fixture/Makefile"
rm -f "$broken_ownership_ci_fixture/Makefile.bak"
git -C "$broken_ownership_ci_fixture" add Makefile
if (cd "$broken_ownership_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without ownership lint tests was accepted"
fi

for path in scripts/check_arch_imports.go scripts/test_arch_imports.sh; do
  arch_receipt_fixture="$(make_fixture "arch-receipt-${path##*/}")"
  printf '%s\n' '// architecture receipt drift' >>"$arch_receipt_fixture/$path"
  git -C "$arch_receipt_fixture" add "$path"
  if (cd "$arch_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "architecture lint receipt drift was accepted: $path"
  fi
done

arch_runner_mode_fixture="$(make_fixture arch-runner-mode)"
chmod 644 "$arch_runner_mode_fixture/scripts/test_arch_imports.sh"
git -C "$arch_runner_mode_fixture" add scripts/test_arch_imports.sh
if (cd "$arch_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "architecture lint runner mode drift was accepted"
fi

broken_arch_ci_fixture="$(make_fixture broken-arch-ci)"
sed -i.bak -E \
  '/^ci-go:/ s/[[:space:]]arch-import-lint-test([[:space:]]|$)/\1/' \
  "$broken_arch_ci_fixture/Makefile"
rm -f "$broken_arch_ci_fixture/Makefile.bak"
git -C "$broken_arch_ci_fixture" add Makefile
if (cd "$broken_arch_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without the architecture lint tests was accepted"
fi

missing_p0s02_runner_fixture="$(make_fixture missing-p0s02-runner)"
rm -f "$missing_p0s02_runner_fixture/acceptance/p0s02/test_static_contract.sh"
git -C "$missing_p0s02_runner_fixture" add -u acceptance/p0s02/test_static_contract.sh
if (cd "$missing_p0s02_runner_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P0-S02 static-contract runner was accepted"
fi

p0s02_runner_mode_fixture="$(make_fixture p0s02-runner-mode)"
chmod 644 "$p0s02_runner_mode_fixture/acceptance/p0s02/test_static_contract.sh"
git -C "$p0s02_runner_mode_fixture" add acceptance/p0s02/test_static_contract.sh
if (cd "$p0s02_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S02 static-contract runner mode drift was accepted"
fi

broken_p0s02_acceptance_fixture="$(make_fixture broken-p0s02-acceptance)"
sed -i.bak 's/^p0-s02-acceptance: p0-s02-contract$/p0-s02-acceptance:/' \
  "$broken_p0s02_acceptance_fixture/Makefile"
rm -f "$broken_p0s02_acceptance_fixture/Makefile.bak"
git -C "$broken_p0s02_acceptance_fixture" add Makefile
if (cd "$broken_p0s02_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S02 acceptance target without contract dependency was accepted"
fi

missing_p0s03_contract_fixture="$(make_fixture missing-p0s03-contract)"
rm -f "$missing_p0s03_contract_fixture/acceptance/p0s03/static_contract.sh"
git -C "$missing_p0s03_contract_fixture" add -u acceptance/p0s03/static_contract.sh
if (cd "$missing_p0s03_contract_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing required P0-S03 contract file was accepted"
fi

for path in \
  acceptance/p0s03/query_contract_test.go \
  acceptance/p0s03/source_contract.go \
  acceptance/p0s03/test_contract.sh; do
  p0s03_receipt_fixture="$(make_fixture "p0s03-receipt-${path##*/}")"
  case "$path" in
    *.go) printf '%s\n' '// P0-S03 receipt drift' >>"$p0s03_receipt_fixture/$path" ;;
    *.sh) printf '%s\n' '# P0-S03 receipt drift' >>"$p0s03_receipt_fixture/$path" ;;
  esac
  git -C "$p0s03_receipt_fixture" add "$path"
  if (cd "$p0s03_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P0-S03 receipt drift was accepted: $path"
  fi
done

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
  '/^ci-go:/ s/[[:space:]]p0-s03-acceptance([[:space:]])/\1/' \
  "$broken_p0s03_ci_fixture/Makefile"
rm -f "$broken_p0s03_ci_fixture/Makefile.bak"
if grep -Eq '^ci-go:.*p0-s03-acceptance([[:space:]]|$)' "$broken_p0s03_ci_fixture/Makefile"; then
  fail "failed to remove the P0-S03 acceptance dependency"
fi
grep -Eq '^ci-go:.*p0-s04-acceptance([[:space:]]|$)' "$broken_p0s03_ci_fixture/Makefile" ||
  fail "P0-S03 fixture unexpectedly removed the P0-S04 acceptance dependency"
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

missing_p0s04_contract_fixture="$(make_fixture missing-p0s04-contract)"
rm -f "$missing_p0s04_contract_fixture/acceptance/p0s04/static_contract.sh"
git -C "$missing_p0s04_contract_fixture" add -u acceptance/p0s04/static_contract.sh
if (cd "$missing_p0s04_contract_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing required P0-S04 contract file was accepted"
fi

replaced_p0s04_runner_fixture="$(make_fixture replaced-p0s04-runner)"
sed -i.bak \
  's#acceptance/p0s04/test_source_contract.sh#true#' \
  "$replaced_p0s04_runner_fixture/Makefile"
rm -f "$replaced_p0s04_runner_fixture/Makefile.bak"
git -C "$replaced_p0s04_runner_fixture" add Makefile
if (cd "$replaced_p0s04_runner_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "replaced P0-S04 contract runner was accepted"
fi

missing_p0s04_caller_hygiene_fixture="$(make_fixture missing-p0s04-caller-hygiene)"
sed -i.bak '/^unexport BASH_ENV ENV$/d' \
  "$missing_p0s04_caller_hygiene_fixture/Makefile"
rm -f "$missing_p0s04_caller_hygiene_fixture/Makefile.bak"
git -C "$missing_p0s04_caller_hygiene_fixture" add Makefile
if (cd "$missing_p0s04_caller_hygiene_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P0-S04 BASH_ENV and ENV caller hygiene was accepted"
fi

missing_p0s04_recipe_hygiene_fixture="$(make_fixture missing-p0s04-recipe-hygiene)"
sed -i.bak \
  's#@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh#@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh#' \
  "$missing_p0s04_recipe_hygiene_fixture/Makefile"
rm -f "$missing_p0s04_recipe_hygiene_fixture/Makefile.bak"
git -C "$missing_p0s04_recipe_hygiene_fixture" add Makefile
if (cd "$missing_p0s04_recipe_hygiene_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P0-S04 contract recipe BASH_ENV and ENV hygiene was accepted"
fi

weakened_p0s04_coverage_fixture="$(make_fixture weakened-p0s04-coverage-proof)"
sed -i.bak 's/matches == 1 && !invalid/1/' \
  "$weakened_p0s04_coverage_fixture/Makefile"
rm -f "$weakened_p0s04_coverage_fixture/Makefile.bak"
git -C "$weakened_p0s04_coverage_fixture" add Makefile
if (cd "$weakened_p0s04_coverage_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "weakened P0-S04 positive coverage proof was accepted"
fi

broken_p0s04_ci_fixture="$(make_fixture broken-p0s04-ci-dependency)"
sed -i.bak -E \
  '/^ci-go:/ s/[[:space:]]p0-s04-acceptance([[:space:]])/\1/' \
  "$broken_p0s04_ci_fixture/Makefile"
rm -f "$broken_p0s04_ci_fixture/Makefile.bak"
git -C "$broken_p0s04_ci_fixture" add Makefile
if (cd "$broken_p0s04_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without the P0-S04 acceptance dependency was accepted"
fi

missing_p0s04_workflow_fixture="$(make_fixture missing-p0s04-workflow-integration)"
sed -i.bak '/^          make p0-s04-integration$/d' \
  "$missing_p0s04_workflow_fixture/.github/workflows/application-go.yml"
rm -f "$missing_p0s04_workflow_fixture/.github/workflows/application-go.yml.bak"
git -C "$missing_p0s04_workflow_fixture" add .github/workflows/application-go.yml
if (cd "$missing_p0s04_workflow_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "application workflow without P0-S04 integration was accepted"
fi

missing_orval_gate_fixture="$(make_fixture missing-orval-gate)"
sed -i.bak 's/npm run orval:check && //' \
  "$missing_orval_gate_fixture/package.json"
rm -f "$missing_orval_gate_fixture/package.json.bak"
git -C "$missing_orval_gate_fixture" add package.json
if (cd "$missing_orval_gate_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "package scripts without the Orval consistency gate were accepted"
fi

orval_runner_mode_fixture="$(make_fixture orval-runner-mode)"
chmod 644 "$orval_runner_mode_fixture/scripts/test_orval_generated_check.sh"
git -C "$orval_runner_mode_fixture" add scripts/test_orval_generated_check.sh
if (cd "$orval_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "Orval generated-client runner mode drift was accepted"
fi

duplicate_p0s04_contract_fixture="$(make_fixture duplicate-p0s04-contract)"
printf '%s\n' '' 'p0-s04-contract:' $'\t@true' \
  >>"$duplicate_p0s04_contract_fixture/Makefile"
git -C "$duplicate_p0s04_contract_fixture" add Makefile
if (cd "$duplicate_p0s04_contract_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "duplicate or overriding P0-S04 contract target was accepted"
fi

multi_target_p0s04_override_fixture="$(make_fixture multi-target-p0s04-override)"
printf '%s\n' '' 'p0-s04-contract p0-s04-sidecar:' $'\t@true' \
  >>"$multi_target_p0s04_override_fixture/Makefile"
git -C "$multi_target_p0s04_override_fixture" add Makefile
if (cd "$multi_target_p0s04_override_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "multi-target P0-S04 contract override was accepted"
fi

p0s04_runner_mode_fixture="$(make_fixture p0s04-runner-mode)"
chmod 644 "$p0s04_runner_mode_fixture/acceptance/p0s04/test_static_contract.sh"
git -C "$p0s04_runner_mode_fixture" add acceptance/p0s04/test_static_contract.sh
if (cd "$p0s04_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S04 static runner mode drift was accepted"
fi

p0s04_go_mode_fixture="$(make_fixture p0s04-go-mode)"
chmod 755 "$p0s04_go_mode_fixture/internal/platform/river/contract.go"
git -C "$p0s04_go_mode_fixture" add internal/platform/river/contract.go
if (cd "$p0s04_go_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S04 Go contract mode drift was accepted"
fi

for path in go.mod go.sum; do
  p0s04_module_pin_fixture="$(make_fixture "p0s04-module-pin-$path")"
  printf '%s\n' '# P0-S04 module pin drift' >>"$p0s04_module_pin_fixture/$path"
  git -C "$p0s04_module_pin_fixture" add "$path"
  if (cd "$p0s04_module_pin_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P0-S04 $path content drift was accepted"
  fi
done
for path in go.mod go.sum; do
  p0s04_module_mode_fixture="$(make_fixture "p0s04-module-mode-$path")"
  chmod 755 "$p0s04_module_mode_fixture/$path"
  git -C "$p0s04_module_mode_fixture" add "$path"
  if (cd "$p0s04_module_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P0-S04 $path mode drift was accepted"
  fi
done

make_bin="$(type -P make || true)"
[[ "$make_bin" == /* && -x "$make_bin" && -x /usr/bin/perl ]] || fail "trusted make/watchdog unavailable"
reset_p0s04_fixture() {
  local fixture="$1"
  rm -f "$fixture/internal/platform/river/runtime.go" \
    "$fixture/internal/platform/river/migrate.go" \
    "$fixture/internal/platform/river/runtime_test.go"
  rm -rf "$fixture/.git"
}
make_p0s04_fixture() {
  local fixture
  fixture="$(make_fixture "$1")"
  reset_p0s04_fixture "$fixture"
  printf '%s\n' "$fixture"
}
assert_p0s04_pending() {
  local fixture="$1" target="$2" gate output
  gate="${target#p0-s04-}"
  output="$(cd "$fixture" && "$make_bin" -o p0-s04-contract "$target" 2>&1)" ||
    fail "canonical empty P0-S04 $gate gate was rejected without Git"
  grep -Fqx "P0-S04 $gate gate: PENDING (implementation not present)" <<<"$output" ||
    fail "canonical empty P0-S04 $gate gate did not report PENDING"
}

p0s04_empty_fixture="$(make_fixture p0s04-canonical-empty)"
for file in runtime.go migrate.go runtime_test.go; do
  candidate="$p0s04_empty_fixture/internal/platform/river/$file"
  [[ -e "$candidate" || -L "$candidate" ]] || printf '%s\n' 'package platformriver' >"$candidate"
  [[ -f "$candidate" && ! -L "$candidate" ]] || fail "P0-S04 implementation-present fixture is incomplete: $file"
done
reset_p0s04_fixture "$p0s04_empty_fixture"
[[ ! -e "$p0s04_empty_fixture/.git" && ! -L "$p0s04_empty_fixture/.git" ]] ||
  fail "P0-S04 no-Git fixture retained .git"
for file in runtime.go migrate.go runtime_test.go; do
  [[ ! -e "$p0s04_empty_fixture/internal/platform/river/$file" && ! -L "$p0s04_empty_fixture/internal/platform/river/$file" ]] ||
    fail "P0-S04 canonical-empty fixture retained implementation: $file"
done
for target in p0-s04-acceptance p0-s04-integration; do
  assert_p0s04_pending "$p0s04_empty_fixture" "$target"
done
p0s04_coverage_fixture="$(make_p0s04_fixture p0s04-coverage-parser)"
for file in runtime.go migrate.go runtime_test.go; do : >"$p0s04_coverage_fixture/internal/platform/river/$file"; done
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$p0s04_coverage_fixture/acceptance/p0s04/static_contract.sh"
printf '%s\n' '#!/usr/bin/env bash' "printf '%s\\n' 'coverage: 12.5% of statements' 'ok  github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river  0.004s  coverage: [no statements]'" >"$p0s04_coverage_fixture/fake-go"
chmod 755 "$p0s04_coverage_fixture/acceptance/p0s04/static_contract.sh" "$p0s04_coverage_fixture/fake-go"
if (cd "$p0s04_coverage_fixture" && GO="$p0s04_coverage_fixture/fake-go" "$make_bin" -o p0-s04-contract p0-s04-acceptance >/dev/null 2>&1); then
  fail "P0-S04 acceptance accepted fake positive coverage with no statements"
fi
printf '%s\n' 'exit 97' >"$p0s04_empty_fixture/hostile-bash-env"
hostile_output="$(cd "$p0s04_empty_fixture" && BASH_ENV="$p0s04_empty_fixture/hostile-bash-env" ENV="$p0s04_empty_fixture/hostile-bash-env" "$make_bin" -o p0-s04-contract p0-s04-acceptance 2>&1)" ||
  fail "hostile BASH_ENV rejected canonical empty P0-S04 gate"
grep -Fqx 'P0-S04 acceptance gate: PENDING (implementation not present)' <<<"$hostile_output" ||
  fail "hostile BASH_ENV bypassed P0-S04 caller hygiene"
set +e
empty_path_output="$(cd "$p0s04_empty_fixture" && PATH=/nonexistent "$make_bin" -o p0-s04-contract p0-s04-acceptance 2>&1)"
empty_path_status=$?
set -e
[[ "$empty_path_status" -ne 0 ]] || fail "empty PATH P0-S04 gate did not fail closed"
for kind in hidden fifo subdir ancestor_symlink contract_symlink runtime_partial runtime_fifo; do
  fixture="$(make_p0s04_fixture "p0s04-invalid-$kind")"
  case "$kind" in
    hidden) : >"$fixture/internal/platform/river/.unexpected" ;;
    fifo) mkfifo "$fixture/internal/platform/river/unexpected-fifo" ;;
    subdir) mkdir "$fixture/internal/platform/river/unexpected-dir" ;;
    ancestor_symlink) mv "$fixture/internal/platform/river" "$fixture/internal/platform/river.real"; ln -s river.real "$fixture/internal/platform/river" ;;
    contract_symlink) mv "$fixture/internal/platform/river/contract.go" "$fixture/internal/platform/river/contract.go.real"; ln -s contract.go.real "$fixture/internal/platform/river/contract.go" ;;
    runtime_partial) : >"$fixture/internal/platform/river/runtime.go" ;;
    runtime_fifo) mkfifo "$fixture/internal/platform/river/runtime.go" ;;
  esac
  for target in p0-s04-acceptance p0-s04-integration; do
    set +e
    (cd "$fixture" && /usr/bin/perl -e 'alarm 5; exec @ARGV' "$make_bin" -o p0-s04-contract "$target" >/dev/null 2>&1); invalid_status=$?
    set -e
    [[ "$invalid_status" -ne 0 && "$invalid_status" -ne 142 ]] || fail "invalid P0-S04 canonical-empty shape was accepted or timed out: $kind/$target"
  done
done

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
