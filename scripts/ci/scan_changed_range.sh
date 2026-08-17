#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

base_sha="${1:-}"
head_sha="${2:-}"
expanded_scan="${3:-false}"

fail() {
  printf 'ci-secret-diff: %s\n' "$1" >&2
  exit 2
}

[[ "$base_sha" =~ ^[0-9a-f]{40}$ ]] || fail "base SHA must be full lowercase hex"
[[ "$head_sha" =~ ^[0-9a-f]{40}$ ]] || fail "head SHA must be full lowercase hex"
[[ "$expanded_scan" = "true" || "$expanded_scan" = "false" ]] ||
  fail "expanded flag must be true or false"
git rev-parse --verify --quiet "${base_sha}^{commit}" >/dev/null || fail "base commit is unavailable"
git rev-parse --verify --quiet "${head_sha}^{commit}" >/dev/null || fail "head commit is unavailable"
command -v gitleaks >/dev/null 2>&1 || fail "gitleaks is required"
[[ "$(gitleaks version)" = "8.30.1" ]] || fail "gitleaks 8.30.1 is required"

name_pattern='(^|/)\.env[^/]*(/|$)|(^|/)(id_rsa[^/]*|cookies[^/]*\.json|credentials[^/]*\.json)$|\.(pem|key|p12|pfx)$'
while IFS= read -r -d '' changed_name; do
  [[ "$changed_name" != *$'\n'* && "$changed_name" != *$'\r'* && "$changed_name" != *$'\t'* ]] ||
    fail "changed filename contains a control character"
  if grep -Eq "$name_pattern" <<<"$changed_name"; then
    printf '%s\n' "$changed_name" >&2
    fail "credential-like changed filename"
  fi
done < <(git -c core.quotePath=false diff --name-only -z --no-renames --diff-filter=ACMRTUXB "$base_sha" "$head_sha" --)

gitleaks git . --config .gitleaks.toml --redact --no-banner --exit-code 1 \
  --log-opts="${base_sha}..${head_sha}"
if [[ "$expanded_scan" = "true" ]]; then
  scripts/test_gitleaks_config.sh
fi
printf 'ci-secret-diff: PASS expanded=%s\n' "$expanded_scan"
