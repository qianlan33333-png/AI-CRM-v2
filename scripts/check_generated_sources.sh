#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/.." && pwd -P)"
cd "$repo_root"

go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
expected_manifest="scripts/generated-sources.sha256"
fingerprint_manifest="docs/ci/repo-contract-fingerprints.tsv"

fail() {
  echo "generated-check: $*" >&2
  exit 1
}

generated_roots=()
while IFS= read -r root; do
  generated_roots[${#generated_roots[@]}]="$root"
done < <(find internal -type d -name generated -print | LC_ALL=C sort)
[[ "${#generated_roots[@]}" -gt 0 ]] || fail "no generated source directories found"
if find internal -name generated ! -type d -print -quit | grep -q .; then
  fail "generated file_path must be a real directory"
fi

hash_file() {
  local file_path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file_path"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file_path"
  else
    fail "sha256sum or shasum is required"
  fi
}

canonical_receipt() {
  local manifest_source="$1" receipt
  receipt="$(awk -F $'\t' -v path="$expected_manifest" '
    NR == 1 { next }
    $3 == path {
      if ($1 != "100644" || $2 !~ /^[0-9a-f]{64}$/ || ++count > 1) exit 1
      digest = $2
    }
    END { if (count != 1) exit 1; print digest }
  ' "$manifest_source")" || fail "canonical fingerprint has no valid generated-source receipt"
  printf '%s\n' "$receipt"
}

receipt_source=""
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  [[ -f "$fingerprint_manifest" && ! -L "$fingerprint_manifest" ]] ||
    fail "canonical fingerprint must be a regular non-symlink file"
  git diff --quiet -- "$expected_manifest" "$fingerprint_manifest" ||
    fail "generated source receipt must be staged before verification"
  fingerprint_entry="$(git -c core.quotePath=false ls-files --stage -- "$fingerprint_manifest")"
  [[ "$(printf '%s\n' "$fingerprint_entry" | wc -l | tr -d ' ')" = "1" ]] ||
    fail "canonical fingerprint must have exactly one staged entry"
  read -r fingerprint_mode fingerprint_blob fingerprint_stage <<<"${fingerprint_entry%%$'\t'*}"
  [[ "$fingerprint_mode" = "100644" && "$fingerprint_stage" = "0" ]] ||
    fail "canonical fingerprint must be a staged 100644 regular file"
  receipt_source="$(mktemp -t aicrm-v2-generated-receipt.XXXXXX)"
  trap 'rm -f "$receipt_source"' EXIT
  git cat-file blob "$fingerprint_blob" >"$receipt_source" ||
    fail "could not read canonical staged fingerprint"
else
  [[ -f "$fingerprint_manifest" && ! -L "$fingerprint_manifest" ]] ||
    fail "canonical fingerprint must be a regular non-symlink file"
  receipt_source="$fingerprint_manifest"
fi

write_actual_manifest() {
  local root file_path
  for root in "${generated_roots[@]}"; do
    [[ -d "$root" ]] || fail "missing generated directory: $root"
  done

  if find "${generated_roots[@]}" -mindepth 1 \
    \( -type l -o ! -type d ! -type f \) -print -quit | grep -q .; then
    find "${generated_roots[@]}" -mindepth 1 \
      \( -type l -o ! -type d ! -type f \) -print >&2
    fail "generated directories contain a symlink or special file"
  fi

  if find "${generated_roots[@]}" -type f \
    \( -perm -0100 -o -perm -0010 -o -perm -0001 \) \
    -print -quit | grep -q .; then
    find "${generated_roots[@]}" -type f \
      \( -perm -0100 -o -perm -0010 -o -perm -0001 \) -print >&2
    fail "generated source files must not be executable"
  fi

  while IFS= read -r file_path; do
    [[ "$file_path" != *[$'\t\r\n ']* && "$file_path" != *\\* ]] ||
      fail "generated file_path contains whitespace or a backslash: $file_path"
    hash_file "$file_path"
  done < <(find "${generated_roots[@]}" -type f -print | LC_ALL=C sort)
}

verify_manifest() {
  local label="$1"
  local actual
  actual="$(mktemp -t aicrm-v2-generated-manifest.XXXXXX)"
  write_actual_manifest >"$actual"
  if ! cmp -s "$expected_manifest" "$actual"; then
    echo "generated-check: generated source drift after $label" >&2
    diff -u "$expected_manifest" "$actual" >&2 || true
    rm -f "$actual"
    exit 1
  fi
  rm -f "$actual"
}

run_generators() {
  GOWORK=off "$go_command" tool -modfile="$tools_mod" oapi-codegen \
    --config api/oapi-codegen.yaml api/openapi.yaml
  GOWORK=off "$go_command" tool -modfile="$tools_mod" oapi-codegen \
    --config api/oapi-codegen-p1-candidate.yaml api/openapi.yaml
  GOWORK=off "$go_command" tool -modfile="$tools_mod" sqlc generate
}

[[ -f "$expected_manifest" && ! -L "$expected_manifest" ]] ||
  fail "manifest must be a regular non-symlink file: $expected_manifest"
actual_manifest_sha256="$(hash_file "$expected_manifest" | awk '{print $1}')"
expected_manifest_sha256="$(canonical_receipt "$receipt_source")"
[[ "$actual_manifest_sha256" = "$expected_manifest_sha256" ]] ||
  fail "generated source manifest disagrees with the canonical fingerprint receipt"

verify_manifest "the initial tree"
run_generators
verify_manifest "generation pass 1"
run_generators
verify_manifest "generation pass 2"

echo "generated-check: PASS"
