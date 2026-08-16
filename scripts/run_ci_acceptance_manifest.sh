#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

manifest="${CI_ACCEPTANCE_MANIFEST:-docs/ci/go-acceptance-manifest.tsv}"
database_url="${CI_ACCEPTANCE_DATABASE_URL:-}"
make_bin="${MAKE:-make}"
go_bin="${GO:-go}"
validate_only="${CI_ACCEPTANCE_VALIDATE_ONLY:-0}"
makefile="${CI_ACCEPTANCE_MAKEFILE:-Makefile}"
base_ref="${CI_ACCEPTANCE_BASE_REF:-}"
base_manifest_path="${CI_ACCEPTANCE_BASE_MANIFEST_PATH:-$manifest}"

fail() {
  echo "ci-acceptance-manifest: $*" >&2
  exit 1
}

[[ -f "$manifest" && ! -L "$manifest" ]] || fail "manifest must be a regular file: $manifest"
[[ "$validate_only" = "0" || "$validate_only" = "1" ]] || fail "CI_ACCEPTANCE_VALIDATE_ONLY must be 0 or 1"
[[ -f "$makefile" && ! -L "$makefile" ]] || fail "Makefile must be a regular file: $makefile"

validate_ancestor_symlinks() {
  local file_path="$1" segment partial=""
  IFS='/' read -r -a segments <<<"$file_path"
  for segment in "${segments[@]}"; do
    partial="${partial:+$partial/}$segment"
    if [[ "$partial" != "$file_path" && -L "$partial" ]]; then
      fail "path has a symlink ancestor: $file_path"
    fi
  done
}

resolve_command() {
  local command_name="$1" label="$2"
  if [[ "$command_name" = */* ]]; then
    [[ -x "$command_name" && ! -L "$command_name" ]] || fail "$label is not an executable regular file: $command_name"
  else
    command -v "$command_name" >/dev/null 2>&1 || fail "$label is unavailable: $command_name"
  fi
}

run_with_database_environment() {
  local environment="$1"
  shift
  if [[ "$environment" = "-" ]]; then
    env -u BASH_ENV -u ENV "$@"
  else
    [[ -n "$database_url" ]] || fail "CI_ACCEPTANCE_DATABASE_URL is required for $environment"
    env -u BASH_ENV -u ENV "$environment=$database_url" "$@"
  fi
}

validate_legacy_make_target() {
  local target="$1" count
  count="$(awk -v target="$target" '
    /^[^[:space:]#][^:]*:/ && $0 !~ /:[[:space:]]+override[[:space:]]+(SHELL|[.]SHELLFLAGS|GO)[[:space:]]*:=/ {
      header = $0
      sub(/:.*/, "", header)
      fields = split(header, names, /[[:space:]]+/)
      for (position = 1; position <= fields; position++) {
        if (names[position] == target) count++
      }
    }
    END { print count + 0 }
  ' "$makefile")"
  [[ "$count" = "1" ]] || fail "legacy Make target must exist exactly once: $target"
}

declare -A base_identifiers=()
base_order=()
if [[ -n "$base_ref" ]]; then
  [[ "$base_manifest_path" =~ ^[A-Za-z0-9._/-]+$ && "$base_manifest_path" != /* && "$base_manifest_path" != ./* && "$base_manifest_path" != *..* && "$base_manifest_path" != *//* ]] ||
    fail "base manifest path is not canonical"
  git rev-parse --verify --quiet "$base_ref^{commit}" >/dev/null || fail "base manifest ref is not a commit"
  base_manifest="$(git show "$base_ref:$base_manifest_path")" || fail "base acceptance manifest is unavailable"
  base_line_number=0
  while IFS= read -r base_line || [[ -n "$base_line" ]]; do
    base_line_number=$((base_line_number + 1))
    (( base_line_number > 1 )) || continue
    [[ -n "$base_line" && "$base_line" != \#* ]] || fail "base manifest has a blank or comment row"
    IFS='|' read -r base_identifier _ <<<"$base_line"
    [[ "$base_identifier" =~ ^[a-z0-9][a-z0-9-]*$ && -z "${base_identifiers[$base_identifier]:-}" ]] ||
      fail "base manifest has an invalid or duplicate id"
    base_identifiers["$base_identifier"]=1
    base_order+=("$base_identifier")
  done <<<"$base_manifest"
  (( ${#base_identifiers[@]} > 0 )) || fail "base manifest has no acceptance ids"
fi

count=0
line_number=0
base_position=0
seen_addition=0
declare -A current_identifiers=()
while IFS= read -r line || [[ -n "$line" ]]; do
  line_number=$((line_number + 1))
  if (( line_number == 1 )); then
    [[ "$line" = "# id|sequence|database environment (- when not needed)|executor|subject|selector (- when not needed)" ]] ||
      fail "manifest header is not canonical"
    continue
  fi

  [[ -n "$line" && "$line" != \#* ]] || fail "line $line_number is blank or a comment"
  line_without_separators="${line//|/}"
  (( ${#line} - ${#line_without_separators} == 5 )) || fail "line $line_number must have six fields"
  IFS='|' read -r identifier sequence environment executor subject selector <<<"$line"

  [[ "$identifier" =~ ^[a-z0-9][a-z0-9-]*$ ]] || fail "line $line_number has an invalid id"
  printf -v expected_sequence '%04d' "$((line_number - 1))"
  [[ "$sequence" = "$expected_sequence" ]] || fail "line $line_number is not in canonical sequence order"
  [[ "$environment" = "-" || "$environment" =~ ^[A-Z][A-Z0-9_]*$ ]] || fail "line $line_number has an invalid environment"
  [[ -z "${current_identifiers[$identifier]:-}" ]] || fail "line $line_number has a duplicate id"
  if [[ -n "$base_ref" ]]; then
    if [[ -n "${base_identifiers[$identifier]:-}" ]]; then
      (( seen_addition == 0 )) || fail "base acceptance id appears after a new id: $identifier"
      [[ "${base_order[$base_position]-}" = "$identifier" ]] || fail "base acceptance ids were reordered"
      base_position=$((base_position + 1))
    else
      (( base_position == ${#base_order[@]} )) || fail "new acceptance id must be appended after base ids: $identifier"
      seen_addition=1
    fi
  fi

  case "$executor" in
    legacy-make)
      [[ "$subject" =~ ^[a-z0-9][a-z0-9-]*(acceptance|integration)$ && "$selector" = "-" ]] ||
        fail "line $line_number has an invalid legacy Make declaration"
      validate_legacy_make_target "$subject"
      if [[ "$validate_only" = "0" ]]; then
        resolve_command "$make_bin" "MAKE command"
        run_with_database_environment "$environment" "$make_bin" --no-print-directory "$subject"
      fi
      ;;
    go-test)
      [[ "$subject" =~ ^\./(acceptance|internal)/[A-Za-z0-9_./-]+$ ]] ||
        fail "line $line_number has an invalid Go package"
      [[ "$subject" != *..* && "$subject" != *//* && "$subject" != */ ]] ||
        fail "line $line_number has a non-canonical Go package"
      package_path="${subject#./}"
      validate_ancestor_symlinks "$package_path"
      [[ -d "$package_path" && ! -L "$package_path" ]] || fail "line $line_number Go package is not a regular directory"
      [[ "$selector" = "-" || "$selector" =~ ^[A-Za-z0-9_().^$|*+?-]+$ ]] ||
        fail "line $line_number has an invalid Go test selector"
      if [[ "$validate_only" = "0" ]]; then
        resolve_command "$go_bin" "GO command"
        go_arguments=(test -race -count=1 -timeout=240s)
        [[ "$selector" = "-" ]] || go_arguments+=(-run "$selector")
        go_arguments+=("$subject")
        run_with_database_environment "$environment" \
          env GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_bin" "${go_arguments[@]}"
      fi
      ;;
    script)
      [[ "$subject" =~ ^acceptance/[A-Za-z0-9_./-]+\.sh$ && "$subject" != *..* && "$subject" != *//* && "$selector" = "-" ]] ||
        fail "line $line_number has an invalid acceptance script"
      validate_ancestor_symlinks "$subject"
      [[ -f "$subject" && ! -L "$subject" && -x "$subject" ]] ||
        fail "line $line_number acceptance script is not an executable regular file"
      if [[ "$validate_only" = "0" ]]; then
        run_with_database_environment "$environment" "$subject"
      fi
      ;;
    *)
      fail "line $line_number has an unknown executor"
      ;;
  esac

  count=$((count + 1))
  current_identifiers["$identifier"]=1
done <"$manifest"

(( count > 0 )) || fail "manifest has no acceptance targets"
for base_identifier in "${!base_identifiers[@]}"; do
  [[ -n "${current_identifiers[$base_identifier]:-}" ]] || fail "base acceptance id was removed: $base_identifier"
done
echo "ci-acceptance-manifest: PASS entries=$count"
