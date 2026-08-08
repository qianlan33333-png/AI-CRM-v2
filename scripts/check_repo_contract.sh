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
  .github/CODEOWNERS
  .github/pull_request_template.md
  docs/architecture/canonical.md
  docs/architecture/port-contracts.md
  docs/architecture/table-ownership.yml
  docs/governance/limitations.md
  docs/execution/slice-card-template.md
  docs/execution/slice-ledger.yml
  docs/spec/AI-CRM-v2-执行方案.md
  docs/spec/AI-CRM-v2-重构详细设计.md
  docs/spec/SHA256SUMS
)

for path in "${required[@]}"; do
  [[ -f "$path" ]] || fail "missing required file: $path"
done

for number in $(seq -w 1 10); do
  [[ -f "docs/adr/ADR-0${number}.md" ]] || fail "missing ADR-0${number}"
done

(cd docs/spec && sha256sum -c SHA256SUMS)

forbidden_path_pattern='(^|/)(\.env[^/]*|node_modules|vendor|dist|build|coverage|\.cache|runtime|logs|uploads|playwright-report|test-results|\.auth|\.browser)(/|$)|(^|/)(id_rsa[^/]*|cookies[^/]*\.json|credentials[^/]*\.json)$|\.(pem|key|p12|pfx|db|sqlite|sqlite3|dump|zip)$'
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

if git grep --cached -n -E \
  '(contents|id-token|packages|deployments):[[:space:]]*write|^[[:space:]]*environment:' \
  -- .github/workflows; then
  fail "workflow write permission or environment is forbidden during bootstrap"
fi

if git grep --cached -n -E '\$\{\{[[:space:]]*secrets\.' -- .github/workflows; then
  fail "workflow secrets context is forbidden during bootstrap"
fi

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
