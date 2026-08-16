#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

test_root="$(mktemp -d -t aicrm-v2-ci-promotion-smoke.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fail() {
  echo "ci-promotion-smoke-tests: $*" >&2
  exit 1
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

refresh_repo_fingerprints() {
  local fixture="$1" manifest file_path mode digest
  manifest="$fixture/docs/ci/repo-contract-fingerprints.tsv"
  printf '# mode\tsha256\tpath\n' >"$manifest"
  while IFS= read -r -d '' file_path; do
    [[ "$file_path" = "docs/ci/repo-contract-fingerprints.tsv" ]] && continue
    mode="$(git -C "$fixture" ls-files --stage -- "$file_path" | awk '{print $1}')"
    if command -v sha256sum >/dev/null 2>&1; then
      digest="$(git -C "$fixture" show ":$file_path" | sha256sum | awk '{print $1}')"
    else
      digest="$(git -C "$fixture" show ":$file_path" | shasum -a 256 | awk '{print $1}')"
    fi
    printf '%s\t%s\t%s\n' "$mode" "$digest" "$file_path" >>"$manifest"
  done < <(git -C "$fixture" -c core.quotePath=false ls-files -z)
  git -C "$fixture" add docs/ci/repo-contract-fingerprints.tsv
}

write_affected_manifest() {
  local fixture="$1" output="$2"
  shift 2
  jq -n '$ARGS.positional | map({filename:.,status:"modified",sha:""})' --args "$@" >"$output"
  for file_path in "$@"; do
    blob="$(git -C "$fixture" rev-parse "HEAD:$file_path")"
    jq --arg path "$file_path" --arg blob "$blob" '(.[] | select(.filename == $path)).sha = $blob' "$output" >"$output.tmp"
    mv "$output.tmp" "$output"
  done
}

run_smoke() {
  local fixture="$1" affected="$2"
  (cd "$fixture" && CI_PROMOTION_AFFECTED_MANIFEST="$affected" \
    MAKE="$test_root/not-installed-make" GO="$test_root/not-installed-go" \
    scripts/check_ci_promotion_smoke.sh)
}

commit_fixture() {
  local fixture="$1" message="$2"
  git -C "$fixture" -c user.name=ci-fixture -c user.email=ci@example.invalid \
    commit --quiet --no-gpg-sign -m "$message"
}

fixture="$test_root/repo"
mkdir -p \
  "$fixture/cmd/aicrm" \
  "$fixture/internal/platform/runtime" \
  "$fixture/internal/contact/app" \
  "$fixture/internal/api/generated" \
  "$fixture/migrations" \
  "$fixture/scripts" \
  "$fixture/docs/ci"
cp \
  scripts/check_ci_promotion_smoke.sh \
  scripts/check_repo_fingerprints.sh \
  scripts/run_ci_acceptance_manifest.sh \
  scripts/verify_repo_receipts.pl \
  "$fixture/scripts/"
chmod +x "$fixture/scripts/"*.sh
chmod +x "$fixture/scripts/verify_repo_receipts.pl"
printf '%s\n' 'package main' >"$fixture/cmd/aicrm/main.go"
printf '%s\n' 'package runtime' >"$fixture/internal/platform/runtime/run.go"
printf '%s\n' 'package app' >"$fixture/internal/contact/app/service.go"
printf '%s\n' 'package generated' >"$fixture/internal/api/generated/server.gen.go"
printf '%s\n' '-- migration' >"$fixture/migrations/00001_fixture.sql"
printf '%s\n' 'fixture-acceptance:' >"$fixture/Makefile"
printf '%s\n' \
  '# id|sequence|database environment (- when not needed)|executor|subject|selector (- when not needed)' \
  'fixture|0001|-|legacy-make|fixture-acceptance|-' >"$fixture/docs/ci/go-acceptance-manifest.tsv"
printf '%s  %s\n' \
  "$(sha256_file "$fixture/internal/api/generated/server.gen.go")" \
  'internal/api/generated/server.gen.go' >"$fixture/scripts/generated-sources.sha256"

git -C "$fixture" init --quiet -b main
git -C "$fixture" add .
refresh_repo_fingerprints "$fixture"
commit_fixture "$fixture" base

printf '%s\n' 'package app // changed' >"$fixture/internal/contact/app/service.go"
git -C "$fixture" add internal/contact/app/service.go
refresh_repo_fingerprints "$fixture"
commit_fixture "$fixture" changed

affected="$test_root/affected.json"
write_affected_manifest "$fixture" "$affected" \
  docs/ci/repo-contract-fingerprints.tsv \
  internal/contact/app/service.go
run_smoke "$fixture" "$affected" >/dev/null || fail "valid promotion smoke was rejected"

jq '(.[] | select(.filename == "internal/contact/app/service.go")).sha = "0000000000000000000000000000000000000000"' "$affected" >"$affected.tmp"
mv "$affected.tmp" "$affected"
if run_smoke "$fixture" "$affected" >/dev/null 2>&1; then
  fail "mismatched affected blob was accepted"
fi
if (cd "$fixture" && scripts/check_ci_promotion_smoke.sh) >/dev/null 2>&1; then
  fail "missing affected manifest was accepted"
fi

fingerprint_drift="$test_root/fingerprint-drift"
git clone --quiet "$fixture" "$fingerprint_drift"
printf '%s\n' 'package app // drift without fingerprint' >"$fingerprint_drift/internal/contact/app/service.go"
git -C "$fingerprint_drift" add internal/contact/app/service.go
commit_fixture "$fingerprint_drift" drift
write_affected_manifest "$fingerprint_drift" "$test_root/fingerprint-drift.json" internal/contact/app/service.go
if run_smoke "$fingerprint_drift" "$test_root/fingerprint-drift.json" >/dev/null 2>&1; then
  fail "business drift without a repo fingerprint update was accepted"
fi

generated_drift="$test_root/generated-drift"
git clone --quiet "$fixture" "$generated_drift"
printf '%s\n' 'package generated // drift' >"$generated_drift/internal/api/generated/server.gen.go"
git -C "$generated_drift" add internal/api/generated/server.gen.go
refresh_repo_fingerprints "$generated_drift"
commit_fixture "$generated_drift" generated-drift
write_affected_manifest "$generated_drift" "$test_root/generated-drift.json" \
  docs/ci/repo-contract-fingerprints.tsv internal/api/generated/server.gen.go
if run_smoke "$generated_drift" "$test_root/generated-drift.json" >/dev/null 2>&1; then
  fail "generated hash mismatch was accepted"
fi

symlink_fixture="$test_root/symlink"
git clone --quiet "$fixture" "$symlink_fixture"
ln -s service.go "$symlink_fixture/internal/contact/app/service_link.go"
git -C "$symlink_fixture" add internal/contact/app/service_link.go
refresh_repo_fingerprints "$symlink_fixture"
commit_fixture "$symlink_fixture" symlink
write_affected_manifest "$symlink_fixture" "$test_root/symlink.json" \
  docs/ci/repo-contract-fingerprints.tsv internal/contact/app/service_link.go
if run_smoke "$symlink_fixture" "$test_root/symlink.json" >/dev/null 2>&1; then
  fail "affected symlink was accepted"
fi

delete_fixture="$test_root/delete"
git clone --quiet "$fixture" "$delete_fixture"
git -C "$delete_fixture" rm --quiet internal/contact/app/service.go
refresh_repo_fingerprints "$delete_fixture"
commit_fixture "$delete_fixture" delete
delete_manifest="$test_root/delete.json"
printf '%s\n' '[{"filename":"internal/contact/app/service.go","status":"removed","sha":"0000000000000000000000000000000000000000"}]' >"$delete_manifest"
if run_smoke "$delete_fixture" "$delete_manifest" >/dev/null 2>&1; then
  fail "delete was accepted"
fi

echo "ci-promotion-smoke-tests: PASS"
