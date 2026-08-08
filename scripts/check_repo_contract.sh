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
  acceptance/p0s01/runtime_contract_test.go
  acceptance/p0s01/process_blackbox.sh
  acceptance/p0s01/static_contract.sh
  acceptance/p0s02/health_contract_test.go
  acceptance/p0s02/static_contract.sh
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

verify_index_sha256() {
  local path="$1"
  local expected="$2"
  local actual
  actual="$(git show ":$path" | sha256sum | awk '{print $1}')"
  [[ "$actual" = "$expected" ]] ||
    fail "central workflow content drifted: $path ($actual)"
}

verify_index_sha256 .github/workflows/application-go.yml \
  ff98340ac1bf905338a687366a5fafa30235187d9f61a7acea80d9011bd5ada1
verify_index_sha256 .github/workflows/repo-contract.yml \
  32ae51c23bffdc930bbf2cbec4098089d4eb46c879fb79b141665523f93547e5
verify_index_sha256 .github/workflows/secret-scan.yml \
  157db46e8147cdca2c71d3044e46d20ddae82374a0368e0fe0b4958d8d3c2488
verify_index_sha256 scripts/check_generated_sources.sh \
  f5454daac1f26512bd09292a805fc722e51bcd2efbf77e0f202c13e80c63644d
verify_index_sha256 scripts/generated-sources.sha256 \
  babd2070d3b7c52ad0c2f6d04e6f288e68e733b5f6ccbd707e60a85384521ff8

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
