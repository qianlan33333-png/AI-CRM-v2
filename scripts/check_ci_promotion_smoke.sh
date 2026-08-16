#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
  echo "ci-promotion-smoke: $*" >&2
  exit 1
}

affected_manifest="${CI_PROMOTION_AFFECTED_MANIFEST:-}"
[[ -n "$affected_manifest" ]] || fail "CI_PROMOTION_AFFECTED_MANIFEST is required"
[[ -f "$affected_manifest" && ! -L "$affected_manifest" ]] || fail "affected manifest must be a regular file"
jq -e '
  type == "array" and length > 0 and
  ([.[].filename] | length == (unique | length)) and
  all(.[];
    (.filename | type == "string" and test("^[A-Za-z0-9._/-]+$") and
      (startswith("/") | not) and (startswith("./") | not) and
      (contains("//") | not) and
      (split("/") | all(. != "" and . != "." and . != ".."))) and
    (.status == "added" or .status == "modified") and
    (.sha | type == "string" and test("^[0-9a-f]{40}$"))
  )
' "$affected_manifest" >/dev/null || fail "affected manifest has an invalid entry"

read -r -a commit_line <<<"$(git rev-list --parents -n 1 HEAD)"
[[ "${#commit_line[@]}" = "2" ]] || fail "HEAD must have exactly one parent"
if git diff --name-status --no-renames HEAD^ HEAD | awk '$1 != "A" && $1 != "M" { bad=1 } END { exit bad ? 0 : 1 }'; then
  fail "HEAD contains a delete, rename, copy, or unsupported change"
fi
actual_paths="$(git diff --name-only --no-renames HEAD^ HEAD | LC_ALL=C sort)"
manifest_paths="$(jq -r '.[].filename' "$affected_manifest" | LC_ALL=C sort)"
[[ "$actual_paths" = "$manifest_paths" ]] || fail "affected manifest path set differs from HEAD"
while IFS=$'\t' read -r file_path expected_blob; do
  tree_entry="$(git -c core.quotePath=false ls-tree HEAD -- "$file_path")"
  read -r actual_mode object_type tree_blob tree_path <<<"${tree_entry//$'\t'/ }"
  [[ "$actual_mode" = "100644" || "$actual_mode" = "100755" ]] || fail "affected path has an invalid mode: $file_path"
  [[ "$object_type" = "blob" && "$tree_path" = "$file_path" ]] || fail "affected path is not a regular blob: $file_path"
  [[ -f "$file_path" && ! -L "$file_path" ]] || fail "affected path is not a regular file: $file_path"
  actual_blob="$(git rev-parse "HEAD:${file_path}")" || fail "affected path is absent from HEAD: $file_path"
  [[ "$actual_blob" = "$tree_blob" && "$actual_blob" = "$expected_blob" ]] || fail "affected fingerprint mismatch: $file_path"
  printf 'affected %s %s\n' "$file_path" "$actual_blob"
done < <(jq -r '.[] | [.filename, .sha] | @tsv' "$affected_manifest")

scripts/check_repo_fingerprints.sh >/dev/null
CI_ACCEPTANCE_VALIDATE_ONLY=1 scripts/run_ci_acceptance_manifest.sh >/dev/null

for startup_path in cmd/aicrm/main.go internal/platform/runtime/run.go; do
  [[ -f "$startup_path" && ! -L "$startup_path" ]] || fail "invalid startup path: $startup_path"
  git cat-file -e "HEAD:${startup_path}" || fail "startup path is absent from HEAD: $startup_path"
  printf 'startup %s %s\n' "$startup_path" "$(git rev-parse "HEAD:${startup_path}")"
done

waterline="$(find migrations -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9][0-9]_*.sql' -print | LC_ALL=C sort | tail -n 1)"
[[ -n "$waterline" ]] || fail "migration waterline is empty"
if find migrations -maxdepth 1 \( -type l -o ! -type d ! -type f \) -print -quit | grep -q .; then
  fail "migration waterline contains a symlink or special file"
fi
printf 'waterline %s %s\n' "$waterline" "$(git rev-parse "HEAD:${waterline}")"

manifest=scripts/generated-sources.sha256
[[ -f "$manifest" && ! -L "$manifest" ]] || fail "generated manifest is not a regular file"
previous_generated_path=""
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ "$line" =~ ^([0-9a-f]{64})'  '([A-Za-z0-9._/-]+)$ ]] || fail "invalid generated manifest entry"
  expected="${BASH_REMATCH[1]}"
  file_path="${BASH_REMATCH[2]}"
  [[ "$file_path" != /* && "$file_path" != ./* && "$file_path" != *..* && "$file_path" != *//* ]] ||
    fail "non-canonical generated manifest path"
  [[ -z "$previous_generated_path" || "$previous_generated_path" < "$file_path" ]] ||
    fail "generated manifest is duplicated or not in canonical order"
  [[ "$expected" =~ ^[0-9a-f]{64}$ && -f "$file_path" && ! -L "$file_path" ]] || fail "invalid generated manifest entry"
  if command -v sha256sum >/dev/null 2>&1; then
    actual_sha256="$(sha256sum "$file_path" | awk '{print $1}')"
  else
    actual_sha256="$(shasum -a 256 "$file_path" | awk '{print $1}')"
  fi
  [[ "$actual_sha256" = "$expected" ]] || fail "generated fingerprint mismatch: $file_path"
  actual="$(git hash-object "$file_path")"
  git_object="$(git rev-parse "HEAD:${file_path}")"
  [[ "$actual" = "$git_object" ]] || fail "generated file differs from HEAD: $file_path"
  previous_generated_path="$file_path"
done <"$manifest"
[[ -n "$previous_generated_path" ]] || fail "generated manifest is empty"
printf 'generated-manifest %s entries=%s\n' "$(git rev-parse "HEAD:${manifest}")" "$(wc -l <"$manifest" | tr -d ' ')"

echo "ci-promotion-smoke: PASS"
