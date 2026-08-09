#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
orval="$repo_root/node_modules/.bin/orval"
test_root="$(mktemp -d -t aicrm-v2-orval-check.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fail() {
  echo "orval-generated-tests: $*" >&2
  exit 1
}

[[ -e "$orval" && "$($orval --version)" = "7.21.0" ]] ||
  fail "the pinned Orval 7.21.0 binary is required"

make_fixture() {
  local name="$1"
  local fixture="$test_root/$name"
  mkdir -p "$fixture"
  git -C "$repo_root" archive --format=tar "$(git -C "$repo_root" write-tree)" |
    tar -xf - -C "$fixture"
  git -C "$fixture" init -q
  git -C "$fixture" add -A
  git -C "$fixture" -c user.name=AI-CRM -c user.email=fixture.invalid \
    commit -q -m baseline
  printf '%s\n' "$fixture"
}

commit_fixture() {
  local fixture="$1"
  local message="$2"
  git -C "$fixture" add -A
  git -C "$fixture" -c user.name=AI-CRM -c user.email=fixture.invalid \
    commit -q -m "$message"
}

run_check() {
  local fixture="$1"
  (cd "$fixture" && ORVAL="$orval" npm run --silent orval:check)
}

valid_fixture="$(make_fixture valid)"
run_check "$valid_fixture" >/dev/null || fail "valid generated client was rejected"

tampered_fixture="$(make_fixture tampered)"
printf '%s\n' '// forbidden generated edit' \
  >>"$tampered_fixture/web/src/api/generated/health.ts"
commit_fixture "$tampered_fixture" tampered
if run_check "$tampered_fixture" >/dev/null 2>&1; then
  fail "committed generated-client drift was accepted"
fi

missing_fixture="$(make_fixture missing)"
git -C "$missing_fixture" rm -q web/src/api/generated/health.ts
git -C "$missing_fixture" -c user.name=AI-CRM -c user.email=fixture.invalid \
  commit -q -m missing
if run_check "$missing_fixture" >/dev/null 2>&1; then
  fail "missing generated client was accepted"
fi

extra_fixture="$(make_fixture extra)"
printf '%s\n' 'export const forbidden = true;' \
  >"$extra_fixture/web/src/api/generated/extra.ts"
commit_fixture "$extra_fixture" extra
if run_check "$extra_fixture" >/dev/null 2>&1; then
  fail "unexpected generated client was accepted"
fi

echo "orval-generated-tests: PASS"
