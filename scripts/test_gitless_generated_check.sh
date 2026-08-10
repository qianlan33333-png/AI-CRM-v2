#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
test_root="$(mktemp -d -t aicrm-v2-gitless-generated.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fail() {
  echo "gitless-generated-tests: $*" >&2
  exit 1
}

make_fixture() {
  local name="$1"
  local fixture="$test_root/$name"
  mkdir -p "$fixture"
  git -C "$repo_root" archive --format=tar "$(git -C "$repo_root" write-tree)" |
    tar -xf - -C "$fixture"
  [[ ! -e "$fixture/.git" ]] || fail "fixture unexpectedly contains .git"
  printf '%s\n' "$fixture"
}

valid_fixture="$(make_fixture valid)"
(cd "$valid_fixture" && make generate-check >/dev/null) ||
  fail "valid gitless source archive was rejected"
(cd "$valid_fixture" && make mod-check >/dev/null) ||
  fail "module drift check failed in a valid gitless source archive"
if [[ -f "$valid_fixture/cmd/aicrm/main.go" ]]; then
  (cd "$valid_fixture" && acceptance/p0s01/static_contract.sh >/dev/null) ||
    fail "P0-S01 static contract failed in a valid gitless source archive"
  (cd "$valid_fixture" && acceptance/p0s01/process_blackbox.sh >/dev/null) ||
    fail "P0-S01 process contract failed in a valid gitless source archive"
else
  static_log="$test_root/gitless-static.log"
  if (cd "$test_root" &&
    "$valid_fixture/acceptance/p0s01/static_contract.sh" >"$static_log" 2>&1); then
    fail "P0-S01 static contract unexpectedly passed without implementation"
  fi
  grep -Fq 'p0-s01-static: missing required Slice file:' "$static_log" ||
    fail "P0-S01 static contract did not resolve its gitless repository root"
fi

tampered_fixture="$(make_fixture tampered-generated-source)"
printf '\n// forbidden manual edit\n' \
  >>"$tampered_fixture/internal/api/generated/server.gen.go"
if (cd "$tampered_fixture" && make generate-check >/dev/null 2>&1); then
  fail "tampered generated source was accepted without Git metadata"
fi

unexpected_fixture="$(make_fixture unexpected-generated-source)"
printf 'package generated\n' \
  >"$unexpected_fixture/internal/platform/store/generated/unexpected.go"
if (cd "$unexpected_fixture" && make generate-check >/dev/null 2>&1); then
  fail "unexpected generated source was accepted without Git metadata"
fi

unregistered_directory_fixture="$(make_fixture unregistered-generated-directory)"
mkdir -p "$unregistered_directory_fixture/internal/api/candidate/generated"
printf 'package generated\n' \
  >"$unregistered_directory_fixture/internal/api/candidate/generated/server.gen.go"
if (cd "$unregistered_directory_fixture" && make generate-check >/dev/null 2>&1); then
  fail "an unregistered generated directory bypassed reproducibility checks"
fi

rewritten_manifest_fixture="$(make_fixture rewritten-generated-manifest)"
rewritten_path="internal/platform/store/generated/unexpected.go"
printf 'package generated\n' >"$rewritten_manifest_fixture/$rewritten_path"
if command -v sha256sum >/dev/null 2>&1; then
  rewritten_hash="$(sha256sum "$rewritten_manifest_fixture/$rewritten_path" |
    awk '{print $1}')"
else
  rewritten_hash="$(shasum -a 256 "$rewritten_manifest_fixture/$rewritten_path" |
    awk '{print $1}')"
fi
printf '%s  %s\n' "$rewritten_hash" "$rewritten_path" \
  >>"$rewritten_manifest_fixture/scripts/generated-sources.sha256"
if (cd "$rewritten_manifest_fixture" &&
  make generate-check >/dev/null 2>&1); then
  fail "a rewritten manifest self-authorized an unexpected generated source"
fi

missing_fixture="$(make_fixture missing-generated-source)"
mv "$missing_fixture/internal/platform/store/generated/models.go" \
  "$test_root/removed-models.go"
if (cd "$missing_fixture" && make generate-check >/dev/null 2>&1); then
  fail "missing generated source was accepted without Git metadata"
fi

echo "gitless-generated-tests: PASS"
