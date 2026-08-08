#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
  echo "sensitive-path-scan: $*" >&2
  exit 1
}

name_pattern='(^|/)\.env[^/]*(/|$)|(^|/)(id_rsa[^/]*|cookies[^/]*\.json|credentials[^/]*\.json)$|\.(pem|key|p12|pfx)$'
if git ls-files | grep -E "$name_pattern" >/dev/null; then
  git ls-files | grep -E "$name_pattern" >&2
  fail "credential-like filename is tracked"
fi

secret_pattern='-----BEGIN ([A-Z0-9 ]+ )?PRIVATE KEY-----|AKIA[0-9A-Z]{16}|github_pat_[A-Za-z0-9_]{20,}|gh[opsu]_[A-Za-z0-9]{30,}|sk-(proj-)?[A-Za-z0-9_-]{32,}|Cookie:[[:space:]]*[^<[:space:]][^[:space:]]{16,}'
scan_output="$(mktemp -t aicrm-v2-sensitive-scan.XXXXXX)"
trap 'rm -f "$scan_output"' EXIT
set +e
git grep --cached -nI -E -e "$secret_pattern" -- . \
  ":(exclude)scripts/scan_sensitive_paths.sh" >"$scan_output"
scan_status=$?
set -e
case "$scan_status" in
  0)
    sed -n '1,80p' "$scan_output" >&2
    fail "high-confidence secret pattern found"
    ;;
  1) ;;
  *) fail "git grep failed with exit code $scan_status" ;;
esac

echo "sensitive-path-scan: PASS"
