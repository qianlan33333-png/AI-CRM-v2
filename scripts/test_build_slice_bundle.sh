#!/usr/bin/env bash
set -euo pipefail

if [[ "${AICRM_BUNDLE_TEST_STRESS_CHILD:-}" != "1" ]]; then
  for run in {1..12}; do
    AICRM_BUNDLE_TEST_STRESS_CHILD=1 "$0"
  done
  echo "bundle-tests: PASS (stress_runs=12)"
  exit 0
fi

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
test_root_parent="$(cd "$(dirname "$test_root")" && pwd -P)"
test_root_name="$(basename "$test_root")"
test_root="$test_root_parent/$test_root_name"

cleanup_target_is_safe() {
  local target="$1"
  [[ "$target" = "$test_root" ]] || return 1
  [[ "$test_root_name" == aicrm-v2-bundle-test.* ]] || return 1
  [[ -d "$target" && ! -L "$target" ]] || return 1
}

cleanup_test_root() {
  local attempt
  cleanup_target_is_safe "$test_root" || {
    echo "bundle-tests: refusing unsafe cleanup target: $test_root" >&2
    return 1
  }
  for attempt in {1..5}; do
    if rm -rf -- "$test_root" && [[ ! -e "$test_root" ]]; then
      return 0
    fi
    [[ ! -e "$test_root" ]] && return 0
    sleep 0.1
  done
  echo "bundle-tests: bounded cleanup failed: $test_root" >&2
  return 1
}

cleanup_on_exit() {
  local status=$?
  trap - EXIT
  cleanup_test_root || exit 1
  exit "$status"
}
trap cleanup_on_exit EXIT

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
cleanup_target_is_safe "$fixture_repo/leak-link.txt" &&
  fail "cleanup accepted a nested symlink target"
cleanup_target_is_safe "$proof_file" &&
  fail "cleanup accepted an out-of-root target"
git -C "$fixture_repo" -c gc.auto=0 -c maintenance.auto=false init -q
git -C "$fixture_repo" -c gc.auto=0 -c maintenance.auto=false add -A
git -C "$fixture_repo" -c gc.auto=0 -c maintenance.auto=false \
  -c user.name=Bundle-Test \
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
