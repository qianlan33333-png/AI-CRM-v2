#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

test_root="$(mktemp -d -t aicrm-v2-repo-fingerprints.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

fail() {
  echo "repo-fingerprint-tests: $*" >&2
  exit 1
}

sha256_index_path() {
  local fixture="$1" file_path="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    git -C "$fixture" show ":$file_path" | sha256sum | awk '{print $1}'
  else
    git -C "$fixture" show ":$file_path" | shasum -a 256 | awk '{print $1}'
  fi
}

refresh_manifest() {
  local fixture="$1" manifest file_path mode digest
  manifest="$fixture/docs/ci/repo-contract-fingerprints.tsv"
  printf '# mode\tsha256\tpath\n' >"$manifest"
  while IFS= read -r -d '' file_path; do
    [[ "$file_path" = "docs/ci/repo-contract-fingerprints.tsv" ]] && continue
    mode="$(git -C "$fixture" ls-files --stage -- "$file_path" | awk '{print $1}')"
    digest="$(sha256_index_path "$fixture" "$file_path")"
    printf '%s\t%s\t%s\n' "$mode" "$digest" "$file_path" >>"$manifest"
  done < <(git -C "$fixture" -c core.quotePath=false ls-files -z)
  git -C "$fixture" add docs/ci/repo-contract-fingerprints.tsv
}

make_fixture() {
  local name="$1" fixture
  fixture="$test_root/$name"
  git clone --quiet "$test_root/baseline" "$fixture"
  printf '%s\n' "$fixture"
}

mkdir -p "$test_root/baseline/scripts" "$test_root/baseline/docs/ci" "$test_root/baseline/internal/contact/app"
cp scripts/check_repo_fingerprints.sh "$test_root/baseline/scripts/"
cp scripts/verify_repo_receipts.pl "$test_root/baseline/scripts/"
chmod +x "$test_root/baseline/scripts/check_repo_fingerprints.sh"
chmod +x "$test_root/baseline/scripts/verify_repo_receipts.pl"
printf '%s\n' 'package app' >"$test_root/baseline/internal/contact/app/service.go"
git -C "$test_root/baseline" init --quiet -b main
git -C "$test_root/baseline" config user.email ci@example.invalid
git -C "$test_root/baseline" config user.name ci-fixture
git -C "$test_root/baseline" add \
  scripts/check_repo_fingerprints.sh \
  scripts/verify_repo_receipts.pl \
  internal/contact/app/service.go
refresh_manifest "$test_root/baseline"
git -C "$test_root/baseline" commit --quiet -m baseline

positive="$(make_fixture positive)"
(cd "$positive" && scripts/check_repo_fingerprints.sh >/dev/null) || fail "valid manifest was rejected"

business_drift="$(make_fixture business-drift)"
printf '%s\n' 'package app // changed' >"$business_drift/internal/contact/app/service.go"
git -C "$business_drift" add internal/contact/app/service.go
if (cd "$business_drift" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "business drift without a fingerprint update was accepted"
fi
refresh_manifest "$business_drift"
(cd "$business_drift" && scripts/check_repo_fingerprints.sh >/dev/null) ||
  fail "business drift with a declarative fingerprint update was rejected"

hash_mismatch="$(make_fixture hash-mismatch)"
ruby -e '
  path = ARGV.fetch(0)
  source = File.read(path)
  updated = source.sub(/^100644\t([0-9a-f])([0-9a-f]{63})\tinternal\/contact\/app\/service[.]go$/) do
    replacement = Regexp.last_match(1) == "0" ? "1" : "0"
    "100644\t#{replacement}#{Regexp.last_match(2)}\tinternal/contact/app/service.go"
  end
  abort "missing business fingerprint row" if updated == source
  File.write(path, updated)
' "$hash_mismatch/docs/ci/repo-contract-fingerprints.tsv"
git -C "$hash_mismatch" add docs/ci/repo-contract-fingerprints.tsv
if (cd "$hash_mismatch" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "hash mismatch was accepted"
fi

mode_mismatch="$(make_fixture mode-mismatch)"
sed -i.bak 's/^100644/100755/' "$mode_mismatch/docs/ci/repo-contract-fingerprints.tsv"
rm -f "$mode_mismatch/docs/ci/repo-contract-fingerprints.tsv.bak"
git -C "$mode_mismatch" add docs/ci/repo-contract-fingerprints.tsv
if (cd "$mode_mismatch" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "mode mismatch was accepted"
fi

unordered="$(make_fixture unordered)"
ruby -e 'path=ARGV.fetch(0); lines=File.readlines(path); lines[1],lines[2]=lines[2],lines[1]; File.write(path,lines.join)' "$unordered/docs/ci/repo-contract-fingerprints.tsv"
git -C "$unordered" add docs/ci/repo-contract-fingerprints.tsv
if (cd "$unordered" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "unordered manifest was accepted"
fi

duplicate="$(make_fixture duplicate)"
sed -n '2p' "$duplicate/docs/ci/repo-contract-fingerprints.tsv" >>"$duplicate/docs/ci/repo-contract-fingerprints.tsv"
git -C "$duplicate" add docs/ci/repo-contract-fingerprints.tsv
if (cd "$duplicate" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "duplicate manifest entry was accepted"
fi

traversal="$(make_fixture traversal)"
sed -i.bak 's#internal/contact/app/service[.]go#internal/contact/app/../service.go#' "$traversal/docs/ci/repo-contract-fingerprints.tsv"
rm -f "$traversal/docs/ci/repo-contract-fingerprints.tsv.bak"
git -C "$traversal" add docs/ci/repo-contract-fingerprints.tsv
if (cd "$traversal" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "path traversal was accepted"
fi

missing="$(make_fixture missing)"
sed -i.bak '/internal\/contact\/app\/service[.]go$/d' "$missing/docs/ci/repo-contract-fingerprints.tsv"
rm -f "$missing/docs/ci/repo-contract-fingerprints.tsv.bak"
git -C "$missing" add docs/ci/repo-contract-fingerprints.tsv
if (cd "$missing" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "incomplete manifest path set was accepted"
fi

self_auth="$(make_fixture self-auth)"
self_digest="$(sha256_index_path "$self_auth" docs/ci/repo-contract-fingerprints.tsv)"
printf '100644\t%s\tdocs/ci/repo-contract-fingerprints.tsv\n' "$self_digest" >>"$self_auth/docs/ci/repo-contract-fingerprints.tsv"
git -C "$self_auth" add docs/ci/repo-contract-fingerprints.tsv
if (cd "$self_auth" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "self-authenticating manifest was accepted"
fi

symlink_entry="$(make_fixture symlink-entry)"
ln -s service.go "$symlink_entry/internal/contact/app/service_link.go"
git -C "$symlink_entry" add internal/contact/app/service_link.go
refresh_manifest "$symlink_entry"
if (cd "$symlink_entry" && scripts/check_repo_fingerprints.sh >/dev/null 2>&1); then
  fail "symlink entry was accepted"
fi

echo "repo-fingerprint-tests: PASS"
