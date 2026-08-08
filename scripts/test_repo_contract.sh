#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
test_root="$(mktemp -d -t aicrm-v2-repo-contract-test.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fail() {
  echo "repo-contract-tests: $*" >&2
  exit 1
}

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

envrc_fixture="$(make_fixture envrc-path)"
touch "$envrc_fixture/.envrc"
git -C "$envrc_fixture" add -f .envrc
if (cd "$envrc_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail ".envrc path was accepted"
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
