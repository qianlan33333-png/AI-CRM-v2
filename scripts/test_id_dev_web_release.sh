#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'test-id-dev-web-release: %s\n' "$1" >&2
  exit 1
}

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
source_sha="$(git -C "$repository_root" rev-parse HEAD)"
test_root="$(mktemp -d)"
cleanup() { rm -rf -- "$test_root"; }
trap cleanup EXIT
mkdir -p "$test_root/release-one" "$test_root/release-two" "$test_root/tampered" "$test_root/tampered-html" "$test_root/extra" "$test_root/extra-root"
cp -R "$repository_root/web/dist/." "$test_root/release-one/"
cp -R "$repository_root/web/dist/." "$test_root/release-two/"
cp -R "$repository_root/web/dist/." "$test_root/tampered/"
cp -R "$repository_root/web/dist/." "$test_root/tampered-html/"
cp -R "$repository_root/web/dist/." "$test_root/extra/"
cp -R "$repository_root/web/dist/." "$test_root/extra-root/"

"$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/release-one" --source-sha="$source_sha" --check >/dev/null
tampered_file="$(find "$test_root/tampered/assets" -type f -name '*.js' -print -quit)"
[[ -n "$tampered_file" ]] || fail 'fixture has no JavaScript asset'
printf 'tampered' >>"$tampered_file"
if "$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/tampered" --source-sha="$source_sha" --check >/dev/null 2>&1; then
  fail 'tampered asset was accepted'
fi
printf 'tampered' >>"$test_root/tampered-html/admin/customers.html"
if "$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/tampered-html" --source-sha="$source_sha" --check >/dev/null 2>&1; then
  fail 'tampered HTML was accepted'
fi
cp "$tampered_file" "$test_root/extra/assets/unlisted.js"
if "$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/extra" --source-sha="$source_sha" --check >/dev/null 2>&1; then
  fail 'unlisted asset was accepted'
fi
printf 'unlisted' >"$test_root/extra-root/unlisted.txt"
if "$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/extra-root" --source-sha="$source_sha" --check >/dev/null 2>&1; then
  fail 'unlisted release file was accepted'
fi

"$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/release-one" --source-sha="$source_sha" --activate >/dev/null
next_sha='2222222222222222222222222222222222222222'
node -e 'const fs=require("node:fs"); const file=process.argv[1]; const manifest=JSON.parse(fs.readFileSync(file,"utf8")); manifest.source_sha=process.argv[2]; fs.writeFileSync(file, JSON.stringify(manifest,null,2)+"\n")' "$test_root/release-two/asset-manifest.json" "$next_sha"
"$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/release-two" --source-sha="$next_sha" --activate >/dev/null
[[ "$(readlink "$test_root/id-dev-current")" = "$test_root/release-two" ]] || fail 'current link did not switch'
[[ "$(readlink "$test_root/id-dev-previous")" = "$test_root/release-one" ]] || fail 'previous link was not retained'
"$script_directory/id_dev_web_release.sh" --root="$test_root" --release="$test_root/release-two" --source-sha="$next_sha" --rollback >/dev/null
[[ "$(readlink "$test_root/id-dev-current")" = "$test_root/release-one" ]] || fail 'rollback did not restore previous link'
printf 'test-id-dev-web-release: PASS\n'
