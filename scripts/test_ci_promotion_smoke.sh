#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

test_root="$(mktemp -d -t aicrm-v2-ci-promotion-smoke.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fixture="$test_root/repo"
mkdir -p "$fixture/cmd/aicrm" "$fixture/internal/platform/runtime" "$fixture/internal/contact/app" "$fixture/migrations" "$fixture/scripts"
cp scripts/check_ci_promotion_smoke.sh "$fixture/scripts/"
chmod +x "$fixture/scripts/check_ci_promotion_smoke.sh"
printf '%s\n' 'package main' >"$fixture/cmd/aicrm/main.go"
printf '%s\n' 'package runtime' >"$fixture/internal/platform/runtime/run.go"
printf '%s\n' '-- migration' >"$fixture/migrations/00001_fixture.sql"
printf '%s\n' 'package app' >"$fixture/internal/contact/app/service.go"
: >"$fixture/scripts/generated-sources.sha256"

git -C "$fixture" init --quiet -b main
git -C "$fixture" config user.email ci@example.invalid
git -C "$fixture" config user.name ci-fixture
git -C "$fixture" add .
git -C "$fixture" commit --quiet -m base
printf '%s\n' 'package app // changed' >"$fixture/internal/contact/app/service.go"
git -C "$fixture" add internal/contact/app/service.go
git -C "$fixture" commit --quiet -m changed

blob_sha="$(git -C "$fixture" rev-parse HEAD:internal/contact/app/service.go)"
manifest="$test_root/affected.json"
printf '%s\n' "[{\"filename\":\"internal/contact/app/service.go\",\"status\":\"modified\",\"sha\":\"$blob_sha\"}]" >"$manifest"
(cd "$fixture" && CI_PROMOTION_AFFECTED_MANIFEST="$manifest" scripts/check_ci_promotion_smoke.sh) >/dev/null

printf '%s\n' '[{"filename":"internal/contact/app/service.go","status":"modified","sha":"0000000000000000000000000000000000000000"}]' >"$manifest"
if (cd "$fixture" && CI_PROMOTION_AFFECTED_MANIFEST="$manifest" scripts/check_ci_promotion_smoke.sh) >/dev/null 2>&1; then
  echo "ci-promotion-smoke-tests: mismatched affected blob was accepted" >&2
  exit 1
fi
if (cd "$fixture" && scripts/check_ci_promotion_smoke.sh) >/dev/null 2>&1; then
  echo "ci-promotion-smoke-tests: missing affected manifest was accepted" >&2
  exit 1
fi

echo "ci-promotion-smoke-tests: PASS"
