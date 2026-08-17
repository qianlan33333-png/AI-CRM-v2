#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
  printf 'ci-api-codegen: %s\n' "$1" >&2
  exit 2
}

[[ $# -eq 0 ]] || fail "unexpected argument"
command -v go >/dev/null 2>&1 || fail "go is required"
command -v node >/dev/null 2>&1 || fail "node is required"
command -v npm >/dev/null 2>&1 || fail "npm is required"
[[ "$(node --version)" = "v24.18.0" ]] || fail "Node.js 24.18.0 is required"
[[ "$(npm --version)" = "11.12.1" ]] || fail "npm 11.12.1 is required"

make --no-print-directory generate-check openapi-p1-contract
npm ci --ignore-scripts --no-audit --no-fund
npm run version:check
npm run orval:check
npm run orval:contract
printf 'ci-api-codegen: PASS\n'
