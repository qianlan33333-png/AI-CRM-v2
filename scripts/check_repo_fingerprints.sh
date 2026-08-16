#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

manifest="${REPO_FINGERPRINT_MANIFEST:-docs/ci/repo-contract-fingerprints.tsv}"

fail() {
  echo "repo-fingerprints: $*" >&2
  exit 1
}

validate_path() {
  local file_path="$1" segment partial=""
  [[ -n "$file_path" && "$file_path" != /* && "$file_path" != ./* ]] ||
    fail "non-canonical path: $file_path"
  [[ "$file_path" != *$'\n'* && "$file_path" != *$'\r'* && "$file_path" != *$'\t'* ]] ||
    fail "path contains a control character"
  [[ "$file_path" != *\\* && "$file_path" != *//* ]] ||
    fail "non-canonical path: $file_path"

  IFS='/' read -r -a segments <<<"$file_path"
  for segment in "${segments[@]}"; do
    [[ -n "$segment" && "$segment" != "." && "$segment" != ".." ]] ||
      fail "path traversal is forbidden: $file_path"
    partial="${partial:+$partial/}$segment"
    if [[ "$partial" != "$file_path" && -L "$partial" ]]; then
      fail "path has a symlink ancestor: $file_path"
    fi
  done
}

[[ -f "$manifest" && ! -L "$manifest" ]] || fail "manifest must be a regular file: $manifest"
validate_path "$manifest"

manifest_entry="$(git -c core.quotePath=false ls-files --stage -- "$manifest")"
[[ -n "$manifest_entry" && "$(printf '%s\n' "$manifest_entry" | wc -l | tr -d ' ')" = "1" ]] ||
  fail "manifest must have exactly one staged entry"
manifest_meta="${manifest_entry%%$'\t'*}"
manifest_path="${manifest_entry#*$'\t'}"
read -r manifest_mode manifest_blob manifest_stage <<<"$manifest_meta"
[[ "$manifest_mode" = "100644" && "$manifest_stage" = "0" && "$manifest_path" = "$manifest" ]] ||
  fail "manifest must be a staged 100644 regular file"

declare -a fingerprint_paths=()
declare -a mode_arguments=()
declare -a receipt_arguments=()
line_number=0
previous_path=""
while IFS= read -r line || [[ -n "$line" ]]; do
  line_number=$((line_number + 1))
  if (( line_number == 1 )); then
    [[ "$line" = $'# mode\tsha256\tpath' ]] || fail "manifest header is not canonical"
    continue
  fi

  [[ -n "$line" && "$line" != \#* ]] || fail "line $line_number is blank or a comment"
  line_without_tabs="${line//$'\t'/}"
  (( ${#line} - ${#line_without_tabs} == 2 )) || fail "line $line_number must have three tab-separated fields"
  IFS=$'\t' read -r expected_mode expected_sha256 file_path <<<"$line"

  [[ "$expected_mode" = "100644" || "$expected_mode" = "100755" ]] ||
    fail "line $line_number has an invalid mode"
  [[ "$expected_sha256" =~ ^[0-9a-f]{64}$ ]] || fail "line $line_number has an invalid sha256"
  validate_path "$file_path"
  [[ "$file_path" != "$manifest" ]] || fail "manifest must not authenticate itself"
  [[ -z "$previous_path" || "$previous_path" < "$file_path" ]] ||
    fail "line $line_number is duplicated or not in canonical path order"

  [[ -f "$file_path" && ! -L "$file_path" ]] || fail "path is not a regular file: $file_path"

  fingerprint_paths+=("$file_path")
  mode_arguments+=("$expected_mode" "$file_path")
  receipt_arguments+=("$expected_sha256" "$file_path")
  previous_path="$file_path"
done < <(git cat-file blob "$manifest_blob")

(( line_number > 1 && ${#fingerprint_paths[@]} > 0 )) || fail "manifest has no fingerprint entries"

declare -a tracked_paths=()
while IFS= read -r -d '' file_path; do
  validate_path "$file_path"
  [[ "$file_path" = "$manifest" ]] || tracked_paths+=("$file_path")
done < <(git -c core.quotePath=false ls-files -z)

[[ "${#fingerprint_paths[@]}" = "${#tracked_paths[@]}" ]] ||
  fail "manifest path set differs from the staged index"
for ((index = 0; index < ${#tracked_paths[@]}; index++)); do
  [[ "${fingerprint_paths[index]}" = "${tracked_paths[index]}" ]] ||
    fail "manifest path set differs from the staged index at: ${tracked_paths[index]}"
done

[[ -x scripts/verify_repo_receipts.pl && ! -L scripts/verify_repo_receipts.pl ]] ||
  fail "staged verifier is not an executable regular file"
scripts/verify_repo_receipts.pl modes "${mode_arguments[@]}"
scripts/verify_repo_receipts.pl receipts "${receipt_arguments[@]}"

echo "repo-fingerprints: PASS entries=${#fingerprint_paths[@]}"
