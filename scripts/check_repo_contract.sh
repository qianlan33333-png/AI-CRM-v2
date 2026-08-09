#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
  echo "repo-contract: $*" >&2
  exit 1
}

git diff --quiet -- . ||
  fail "working tree differs from the staged index; contract would be ambiguous"

required=(
  README.md
  AGENTS.md
  CLAUDE.md
  CONTRIBUTING.md
  SECURITY.md
  NOTICE
  .tool-versions
  Makefile
  go.mod
  go.sum
  tools/go.mod
  tools/go.sum
  .github/CODEOWNERS
  .github/pull_request_template.md
  .github/workflows/application-go.yml
  .github/workflows/repo-contract.yml
  .github/workflows/secret-scan.yml
  api/openapi.yaml
  api/oapi-codegen.yaml
  sqlc.yaml
  migrations/00001_bootstrap.sql
  internal/platform/http/contract.go
  internal/platform/runtime/contract.go
  internal/platform/store/contract.go
  internal/platform/river/contract.go
  acceptance/p0s01/runtime_contract_test.go
  acceptance/p0s01/process_blackbox.sh
  acceptance/p0s01/static_contract.sh
  acceptance/p0s02/health_contract_test.go
  acceptance/p0s02/static_contract.sh
  acceptance/p0s02/test_static_contract.sh
  acceptance/p0s03/query_contract_test.go
  acceptance/p0s03/source_contract.go
  acceptance/p0s03/static_contract.sh
  acceptance/p0s03/test_contract.sh
  acceptance/p0s04/contract_test.go
  acceptance/p0s04/source_contract.go
  acceptance/p0s04/test_source_contract.sh
  acceptance/p0s04/static_contract.sh
  acceptance/p0s04/test_static_contract.sh
  scripts/build_slice_bundle.sh
  scripts/check_generated_sources.sh
  scripts/check_repo_contract.sh
  scripts/generated-sources.sha256
  scripts/scan_sensitive_paths.sh
  scripts/test_build_slice_bundle.sh
  scripts/test_gitless_generated_check.sh
  scripts/test_repo_contract.sh
  docs/architecture/canonical.md
  docs/architecture/port-contracts.md
  docs/architecture/table-ownership.yml
  docs/governance/limitations.md
  docs/execution/slice-card-template.md
  docs/execution/slice-ledger.yml
  docs/execution/slices/P0-S02.md
  docs/execution/slices/P0-S03.md
  docs/execution/slices/P0-S04.md
  docs/spec/AI-CRM-v2-执行方案.md
  docs/spec/AI-CRM-v2-重构详细设计.md
  docs/spec/SHA256SUMS
)

for path in "${required[@]}"; do
  [[ -f "$path" ]] || fail "missing required file: $path"
  [[ "$(git ls-files -s -- "$path" | wc -l | tr -d ' ')" = "1" ]] ||
    fail "required file is missing or has an ambiguous index entry: $path"
  index_mode="$(git ls-files -s -- "$path" | awk '{print $1}')"
  case "$index_mode" in
    100644|100755) ;;
    *) fail "required path must be a regular tracked file: $path (mode $index_mode)" ;;
  esac
done

verify_index_mode() {
  local path="$1"
  local expected="$2"
  local actual
  actual="$(git ls-files -s -- "$path" | awk 'NR == 1 { print $1 }')"
  [[ "$actual" = "$expected" ]] ||
    fail "pinned repository mode drifted: $path ($actual)"
}

while IFS=' ' read -r expected path; do
  verify_index_mode "$path" "$expected"
done <<'EOF'
100644 Makefile
100644 go.mod
100644 go.sum
100644 .github/workflows/application-go.yml
100755 scripts/check_repo_contract.sh
100755 scripts/test_repo_contract.sh
100755 acceptance/p0s02/static_contract.sh
100755 acceptance/p0s02/test_static_contract.sh
100644 internal/platform/river/contract.go
100644 acceptance/p0s04/contract_test.go
100644 acceptance/p0s04/source_contract.go
100755 acceptance/p0s04/test_source_contract.sh
100755 acceptance/p0s04/static_contract.sh
100755 acceptance/p0s04/test_static_contract.sh
100644 docs/execution/slices/P0-S04.md
EOF

verify_index_sha256() {
  local path="$1"
  local expected="$2"
  local actual
  actual="$(git show ":$path" | sha256sum | awk '{print $1}')"
  [[ "$actual" = "$expected" ]] ||
    fail "pinned repository content drifted: $path ($actual)"
}

verify_index_sha256 Makefile \
  1ce83378743d4ac4230890db83c72475b22c6239f16b01dfbd6227ccac6489a1
verify_index_sha256 go.mod \
  3cbf623a51f7f6d3d67fd9130d54173ede1c8234b23f28dfa6878c8b4c34d287
verify_index_sha256 go.sum \
  6f4bd6e24238ffc3775f110f672fd8c9f5a1114913dda5e31000d3d6881eda21
verify_index_sha256 .github/workflows/application-go.yml \
  16c984a169ada4cf8b0daa6c219c7a18a2348fb82bf71ef71a2d46c13c555605
verify_index_sha256 .github/workflows/repo-contract.yml \
  32ae51c23bffdc930bbf2cbec4098089d4eb46c879fb79b141665523f93547e5
verify_index_sha256 .github/workflows/secret-scan.yml \
  157db46e8147cdca2c71d3044e46d20ddae82374a0368e0fe0b4958d8d3c2488
verify_index_sha256 scripts/check_generated_sources.sh \
  f5454daac1f26512bd09292a805fc722e51bcd2efbf77e0f202c13e80c63644d
verify_index_sha256 scripts/generated-sources.sha256 \
  babd2070d3b7c52ad0c2f6d04e6f288e68e733b5f6ccbd707e60a85384521ff8
verify_index_sha256 scripts/test_repo_contract.sh \
  a6e902dda4c5fec0b290b05a288347682d213f65c60fd14b870bec91208b93cc
verify_index_sha256 acceptance/p0s02/static_contract.sh \
  b54cb39a87c72969caa8d480ff85da1471b1e6e42556a7ee8db7bf701e1eef91
verify_index_sha256 acceptance/p0s02/test_static_contract.sh \
  92aef4422e152576884d1152c50538d4e517b907ceda5807420cfa6ebdf3d699
verify_index_sha256 internal/platform/store/contract.go \
  747683b0f430da2ee29f001abaebe5fe621561aa3dd99b5b9db6b7d871895165
verify_index_sha256 acceptance/p0s03/query_contract_test.go \
  8ec6608f0856f33d5d3e76f497a88a26d0c54ea65c23b4744a08ef7f9c1ff072
verify_index_sha256 acceptance/p0s03/source_contract.go \
  d26d11f580eb5b9adf0ea11abc1d0c5feda56c974320cce9589cbe76d87a3f9e
verify_index_sha256 acceptance/p0s03/static_contract.sh \
  666b174b0017e44e774eaea1b784d1e0ba93e308632f42f7ace768917d9a3c84
verify_index_sha256 acceptance/p0s03/test_contract.sh \
  2e0108e38c8cb8805e45dc46b60698f62015429a41fb2e880a185585de1896e7
verify_index_sha256 internal/platform/river/contract.go \
  f03a64b78f9fa0f809b869a7d473f42a9edecc41201805fec461f4ba0f1cb292
verify_index_sha256 acceptance/p0s04/contract_test.go \
  969e834043c841f533c3405da13f2048178cb31982ac0619aec04f77ed600340
verify_index_sha256 acceptance/p0s04/source_contract.go \
  8d02d5d5fdd76a31999ed76a9eb77d8343b09d4a0cd48c3531f893d29331f9d0
verify_index_sha256 acceptance/p0s04/test_source_contract.sh \
  5d92037d3a54cc450b7862d08d60015ac9b1a776d11230d1d373d0169a516697
verify_index_sha256 acceptance/p0s04/static_contract.sh \
  8cf9ee7189be9a859c660301a5a801cef50495cfcf394d1ed6854b4e73a39c5d
verify_index_sha256 acceptance/p0s04/test_static_contract.sh \
  8b53e98baeba0c106a489ad3ea9e3b65d618433a3e740b4109ec032e0dd6b97e
verify_index_sha256 docs/execution/slices/P0-S04.md \
  bb8cc8f7ff0ef4d6e76c8c124bafc661f562a230ec25826b8318a82311281608

makefile="$(git show ':Makefile')"
printf '%s\n' "$makefile" | grep -Eq '^p0-s03-contract:[[:space:]]*$' ||
  fail "Makefile is missing the P0-S03 contract target"
p0_s03_contract_recipe="$(
  awk '
    /^p0-s03-contract:[[:space:]]*$/ { capture = 1; next }
    capture && /^[^[:space:]]/ { exit }
    capture { print }
  ' <<<"$makefile"
)"
printf '%s\n' "$p0_s03_contract_recipe" |
  grep -Fqx $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s03/test_contract.sh' ||
  fail "P0-S03 contract target must run the gitless contract tests"
printf '%s\n' "$makefile" |
  grep -Eq '^p0-s03-acceptance:[[:space:]]+p0-s03-contract([[:space:]]|$)' ||
  fail "P0-S03 acceptance target must depend on the contract target"
ci_go_target="$(printf '%s\n' "$makefile" | grep -E '^ci-go:[[:space:]]' || true)"
[[ "$ci_go_target" =~ (^|[[:space:]])p0-s03-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S03 acceptance target"

require_make_line() {
  local line="$1"
  local label="$2"
  [[ "$(printf '%s\n' "$makefile" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "$label"
}

require_unique_make_target() {
  local target="$1"
  local count
  count="$(awk -v target="$target" '
    /^[^[:space:]#][^:]*:/ {
      header = $0
      sub(/:.*/, "", header)
      fields = split(header, names, /[[:space:]]+/)
      for (position = 1; position <= fields; position++) {
        if (names[position] == target) count++
      }
    }
    END { print count + 0 }
  ' <<<"$makefile")"
  [[ "$count" = "1" ]] || fail "Makefile target must be unique: $target"
}

make_target_recipe() {
  local header="$1"
  awk -v header="$header" '
    $0 == header { seen++; capture = 1; next }
    capture && /^[^[:space:]]/ { exit }
    capture { print }
    END { exit !(seen == 1) }
  ' <<<"$makefile"
}

require_make_line 'unexport BASH_ENV ENV' \
  "Makefile must unexport BASH_ENV and ENV before starting recipes"
require_make_line '.PHONY: fmt-check vet test build vuln p0-s01-acceptance p0-s02-contract p0-s02-acceptance p0-s03-contract p0-s03-acceptance ci-go' \
  "Makefile must declare the P0-S02 contract and acceptance targets"
require_unique_make_target p0-s02-contract
require_unique_make_target p0-s02-acceptance
require_make_line 'p0-s02-acceptance: p0-s02-contract' \
  "P0-S02 acceptance target must depend on the contract target"
p0_s02_contract_recipe="$(make_target_recipe 'p0-s02-contract:')" ||
  fail "P0-S02 contract target must be unique"
[[ "$p0_s02_contract_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s02/test_static_contract.sh' ]] ||
  fail "P0-S02 contract target must run only the frozen static-contract runner"
[[ "$ci_go_target" =~ (^|[[:space:]])p0-s02-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S02 acceptance target"
require_make_line '.PHONY: p0-s04-contract p0-s04-acceptance p0-s04-integration' \
  "Makefile must declare the P0-S04 targets exactly once"
require_unique_make_target p0-s04-contract
require_unique_make_target p0-s04-acceptance
require_unique_make_target p0-s04-integration
require_make_line 'p0-s04-contract:' \
  "Makefile is missing or overriding the P0-S04 contract target"
require_make_line 'p0-s04-acceptance: p0-s04-contract' \
  "P0-S04 acceptance target must depend only on the contract target"
require_make_line 'p0-s04-integration: p0-s04-contract' \
  "P0-S04 integration target must depend only on the contract target"

p0_s04_contract_recipe="$(make_target_recipe 'p0-s04-contract:')" ||
  fail "P0-S04 contract target must be unique"
[[ "$p0_s04_contract_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh\n\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_static_contract.sh' ]] ||
  fail "P0-S04 contract target must run only the frozen source and static runners"

p0_s04_empty_lines=(
  $'\t@shopt -s nullglob dotglob; \\'
  $'\t\triver_entries=(internal/platform/river/*); \\'
  $'\tif [[ -d internal && ! -L internal && \\'
  $'\t\t-d internal/platform && ! -L internal/platform && \\'
  $'\t\t-d internal/platform/river && ! -L internal/platform/river && \\'
  $'\t\t-f internal/platform/river/contract.go && ! -L internal/platform/river/contract.go && \\'
  $'\t\t"$${#river_entries[@]}" -eq 1 && "$${river_entries[0]}" = "internal/platform/river/contract.go" && \\'
  $'\t\t! -e internal/platform/river/runtime.go && ! -L internal/platform/river/runtime.go && \\'
  $'\t\t! -e internal/platform/river/migrate.go && ! -L internal/platform/river/migrate.go && \\'
  $'\t\t! -e internal/platform/river/runtime_test.go && ! -L internal/platform/river/runtime_test.go ]]; then \\'
)

p0_s04_acceptance_recipe="$(make_target_recipe 'p0-s04-acceptance: p0-s04-contract')" ||
  fail "P0-S04 acceptance target must be unique"
for line in "${p0_s04_empty_lines[@]}" \
  $'\t\techo "P0-S04 acceptance gate: PENDING (implementation not present)"; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \\' \
  $'\t\tcoverage_output="$$(env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -cover -timeout=15s ./internal/platform/river 2>&1)" || { status=$$?; printf \'%s\\n\' "$$coverage_output"; exit "$$status"; }; \\' \
  $'\t\tif ! printf \'%s\\n\' "$$coverage_output" | awk \'$$1 == "ok" && $$2 == "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river" { matches++; value = $$0; if (value !~ /coverage: [0-9]+([.][0-9]+)?% of statements$$/) invalid = 1; else { sub(/^.*coverage: /, "", value); sub(/% of statements$$/, "", value); if (value + 0 <= 0) invalid = 1 } } END { exit !(matches == 1 && !invalid) }\'; then echo "P0-S04 acceptance gate: internal/platform/river must report positive numeric coverage" >&2; exit 1; fi; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=15s -tags=p0s04_acceptance -run \'^(TestPinnedRiverPublicAPISurface|TestRuntimeLifecycleContract|TestRuntimeStartContextIsolated|TestRuntimeCancellationWinsSimultaneousStopped|TestInvalidMigrationDirection)$$\' ./acceptance/p0s04; \\'; do
  printf '%s\n' "$p0_s04_acceptance_recipe" | grep -Fqx "$line" ||
    fail "P0-S04 acceptance target lost a frozen static, coverage, or non-PG call"
done

p0_s04_integration_recipe="$(make_target_recipe 'p0-s04-integration: p0-s04-contract')" ||
  fail "P0-S04 integration target must be unique"
for line in "${p0_s04_empty_lines[@]}" \
  $'\t\techo "P0-S04 integration gate: PENDING (implementation not present)"; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \\' \
  $'\t\ttest "$${ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST:-}" = "1" || { echo "ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 is required" >&2; exit 2; }; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 $(GO) test -race -timeout=45s -tags=p0s04_acceptance -run \'^TestOfficialMigrationUpDownUp$$\' ./acceptance/p0s04; \\'; do
  printf '%s\n' "$p0_s04_integration_recipe" | grep -Fqx "$line" ||
    fail "P0-S04 integration target lost a frozen static, guard, or PG call"
done

[[ "$(printf '%s\n' "$makefile" | grep -Ec '^ci-go:[[:space:]]' || true)" = "1" && "$ci_go_target" =~ (^|[[:space:]])p0-s04-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S04 acceptance target"

application_go_workflow="$(git show ':.github/workflows/application-go.yml')"
verify_postgres_step="$(
  awk '
    $0 == "      - name: Verify PostgreSQL version and migrations" { seen++; capture = 1; next }
    capture && /^      - name:/ { exit }
    capture { print }
    END { exit !(seen == 1) }
  ' <<<"$application_go_workflow"
)" || fail "application workflow must contain one PostgreSQL verification step"
for line in \
  '          ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST: "1"' \
  '          make migration-integration' \
  '          make p0-s04-integration'; do
  [[ "$(printf '%s\n' "$verify_postgres_step" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow lost the fixed P0-S04 integration environment or call"
done
for line in '      BASH_ENV: ""' '      ENV: ""'; do
  [[ "$(printf '%s\n' "$application_go_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow must clear BASH_ENV and ENV"
done

expected_workflows="$({
  printf '%s\n' .github/workflows/application-go.yml
  printf '%s\n' .github/workflows/repo-contract.yml
  printf '%s\n' .github/workflows/secret-scan.yml
} | LC_ALL=C sort)"
actual_workflows="$(
  git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml' |
    LC_ALL=C sort
)"
[[ "$actual_workflows" = "$expected_workflows" ]] ||
  fail "workflow file set drifted; every workflow requires a Codex-owned hash update"

for number in $(seq -w 1 10); do
  [[ -f "docs/adr/ADR-0${number}.md" ]] || fail "missing ADR-0${number}"
done

(cd docs/spec && sha256sum -c SHA256SUMS)

forbidden_path_pattern='(^|/)(\.env[^/]*|node_modules|vendor|dist|build|coverage|\.cache|playwright-report|test-results|\.auth|\.browser)(/|$)|^(data|runtime|logs|uploads|tmp)(/|$)|(^|/)(id_rsa[^/]*|cookies[^/]*\.json|credentials[^/]*\.json)$|\.(pem|key|p12|pfx|db|sqlite|sqlite3|dump|zip)$'
if git ls-files | grep -E "$forbidden_path_pattern" >/dev/null; then
  git ls-files | grep -E "$forbidden_path_pattern" >&2
  fail "forbidden generated, credential, data, or binary path is tracked"
fi

if git ls-files | grep -E '(^|/)handoffs(/|$)|AI-CRM-v2-v3\.(patch|zip)$' >/dev/null; then
  fail "rejected Pro handoff artifacts must not enter the repository"
fi

if git grep --cached -n 'pull_request_target' -- .github/workflows; then
  fail "pull_request_target is forbidden"
fi

if git grep --cached -n -F '\' -- .github/workflows; then
  fail "workflow backslashes are forbidden because YAML escapes bypass text policy"
fi

if git grep --cached -n -E '[&*]|!!|<<[[:space:]]*:' -- .github/workflows; then
  fail "workflow YAML anchors, aliases, tags, and merge keys are forbidden"
fi

if git grep --cached -n -E \
  "[\"'][[:alnum:]_-]+[\"'][[:space:]]*:" \
  -- .github/workflows; then
  fail "quoted workflow keys are forbidden because they bypass policy scanners"
fi

if git grep --cached -n -i -E \
  '(^|[^[:alnum:]_])write(-all)?([^[:alnum:]_]|$)|(^|[^[:alnum:]_])environment([^[:alnum:]_]|$)|(^|[^[:alnum:]_])deploy(ment|ments|ing)?([^[:alnum:]_]|$)' \
  -- .github/workflows; then
  fail "workflow write permission, environment, or deployment is forbidden during bootstrap"
fi

if git grep --cached -n -i -E \
  '(^|[^[:alnum:]_])secrets([^[:alnum:]_]|$)' \
  -- .github/workflows; then
  fail "workflow secrets context is forbidden during bootstrap"
fi

while IFS= read -r workflow; do
  awk '
    /^permissions:[[:space:]]*$/ { in_top_permissions = 1; saw_permissions = 1; next }
    in_top_permissions && /^[^[:space:]]/ { in_top_permissions = 0 }
    in_top_permissions && /^[[:space:]]*($|#)/ { next }
    in_top_permissions {
      permission_entries++
      if ($0 != "  contents: read") invalid_permission = 1
    }
    END {
      exit !(saw_permissions && permission_entries == 1 && !invalid_permission)
    }
  ' "$workflow" ||
    fail "$workflow must declare only top-level contents: read"

  [[ "$(grep -Ec '^[[:space:]]*permissions[[:space:]]*:' "$workflow")" -eq 1 ]] ||
    fail "$workflow must have exactly one canonical permissions key"

  canonical_uses_pattern='^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*[^[:space:]#]+([[:space:]]*#.*)?$'
  uses_key_pattern='(^|[^[:alnum:]_])uses[[:space:]]*:'
  while IFS= read -r workflow_line; do
    [[ ! "$workflow_line" =~ $uses_key_pattern ]] ||
      [[ "$workflow_line" =~ $canonical_uses_pattern ]] ||
      fail "$workflow contains a non-canonical uses mapping"
  done < "$workflow"
done < <(git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml')

while IFS= read -r action_ref; do
  [[ -z "$action_ref" ]] && continue
  [[ "$action_ref" != ./* ]] || continue
  action_name="${action_ref%@*}"
  action_sha="${action_ref##*@}"
  [[ -n "$action_name" && "$action_ref" == *@* &&
    "$action_sha" =~ ^[0-9a-f]{40}$ ]] ||
    fail "GitHub Action is not pinned to a full lowercase hex commit SHA: $action_ref"
done < <(
  git grep --cached -h -E '^[[:space:]]*(-[[:space:]]*)?uses:' \
    -- .github/workflows |
    sed -E 's/^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*([^[:space:]#]+).*/\2/'
)

scripts/scan_sensitive_paths.sh

echo "repo-contract: PASS"
