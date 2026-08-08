#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
tree_sha="$(git -C "$repo_root" write-tree)"
base_sha="$(
  GIT_AUTHOR_NAME=Bundle-Test \
  GIT_AUTHOR_EMAIL=bundle-test@example.invalid \
  GIT_AUTHOR_DATE=2000-01-01T00:00:00Z \
  GIT_COMMITTER_NAME=Bundle-Test \
  GIT_COMMITTER_EMAIL=bundle-test@example.invalid \
  GIT_COMMITTER_DATE=2000-01-01T00:00:00Z \
    git -C "$repo_root" commit-tree "$tree_sha" -m bundle-test-fixture
)"
test_root="$(mktemp -d -t aicrm-v2-bundle-test.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fail() {
  echo "bundle-tests: $*" >&2
  exit 1
}

safe_paths="$test_root/safe-paths.txt"
printf '%s\n' README.md >"$safe_paths"
safe_output="$test_root/safe-output"
mkdir -p "$safe_output"
"$repo_root/scripts/build_slice_bundle.sh" \
  "$base_sha" P0-TEST "$safe_paths" "$safe_output" >/dev/null
safe_archive="$(find "$safe_output" -type f -name '*.zip' -print -quit)"
[[ -n "$safe_archive" ]] || fail "safe build did not produce a ZIP"
zip -T "$safe_archive" >/dev/null
unzip -p "$safe_archive" P0-TEST-source/BUNDLE-MANIFEST.txt |
  grep -q '^secret_scan=PASS$' || fail "safe bundle manifest is incomplete"

fixture_repo="$test_root/symlink-repo"
mkdir -p "$fixture_repo"
git -C "$repo_root" archive --format=tar "$base_sha" | tar -xf - -C "$fixture_repo"
proof_file="$test_root/outside-data-proof.txt"
printf '%s\n' outside-data-proof >"$proof_file"
ln -s "$proof_file" "$fixture_repo/leak-link.txt"
git -C "$fixture_repo" init -q
git -C "$fixture_repo" add -A
git -C "$fixture_repo" -c user.name=Bundle-Test \
  -c user.email=bundle-test.invalid commit -qm fixture
fixture_sha="$(git -C "$fixture_repo" rev-parse HEAD)"
symlink_paths="$test_root/symlink-paths.txt"
printf '%s\n' leak-link.txt >"$symlink_paths"
symlink_output="$test_root/symlink-output"
mkdir -p "$symlink_output"
if (cd "$fixture_repo" && scripts/build_slice_bundle.sh \
  "$fixture_sha" P0-TEST "$symlink_paths" "$symlink_output" >/dev/null 2>&1); then
  fail "tracked symbolic link was accepted into a bundle"
fi

echo "bundle-tests: PASS"
