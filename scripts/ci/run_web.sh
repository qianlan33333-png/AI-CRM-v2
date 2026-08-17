#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

selection_mode="${1:-}"
run_audit="${2:-false}"

fail() {
  printf 'ci-web: %s\n' "$1" >&2
  exit 2
}

[[ "$selection_mode" = "build" || "$selection_mode" = "full" ]] ||
  fail "mode must be build or full"
[[ "$run_audit" = "true" || "$run_audit" = "false" ]] ||
  fail "audit flag must be true or false"
command -v node >/dev/null 2>&1 || fail "node is required"
command -v npm >/dev/null 2>&1 || fail "npm is required"
[[ "$(node --version)" = "v24.18.0" ]] || fail "Node.js 24.18.0 is required"
[[ "$(npm --version)" = "11.12.1" ]] || fail "npm 11.12.1 is required"

npm ci --ignore-scripts --no-audit --no-fund
npm run version:check
if [[ "$selection_mode" = "full" ]]; then
  npm run orval:check
  npm run orval:contract
  npm run lint
  npm test
fi
npm run typecheck
npm run build
if [[ "$run_audit" = "true" ]]; then
  npm run audit
fi
printf 'ci-web: PASS mode=%s audit=%s\n' "$selection_mode" "$run_audit"
