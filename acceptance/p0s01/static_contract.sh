#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
[[ -f "$repo_root/go.mod" &&
  -f "$repo_root/internal/platform/runtime/contract.go" ]] || {
  echo "p0-s01-static: invalid repository root: $repo_root" >&2
  exit 1
}
cd "$repo_root"

implementation_files=(
  cmd/aicrm/main.go
  cmd/aicrm/components.go
  internal/platform/runtime/cli.go
  internal/platform/runtime/run.go
)

for path in "${implementation_files[@]}" internal/platform/runtime/runtime_test.go; do
  [[ -f "$path" ]] || {
    echo "p0-s01-static: missing required Slice file: $path" >&2
    exit 1
  }
done

reject_matches() {
  local label="$1" mode="$2" pattern="$3" output status=0
  shift 3
  if [[ "$mode" = insensitive ]]; then
    output="$(grep -Ein -- "$pattern" "$@" 2>&1)" || status=$?
  else
    output="$(grep -En -- "$pattern" "$@" 2>&1)" || status=$?
  fi
  case "$status" in
    0)
      printf '%s\n' "$output" >&2
      echo "p0-s01-static: $label" >&2
      exit 1
      ;;
    1) ;;
    *)
      printf '%s\n' "$output" >&2
      echo "p0-s01-static: scanner failed while checking $label" >&2
      exit 2
      ;;
  esac
}

forbidden='"net"|"net/http"|github\.com/jackc/pgx|github\.com/riverqueue/river|os\.Getenv|time\.(NewTicker|Tick|AfterFunc)|log\.(Fatal|Fatalf|Fatalln)'
runtime_contract_files=(
  cmd/aicrm/main.go
  internal/platform/runtime/cli.go
  internal/platform/runtime/run.go
  internal/platform/runtime/runtime_test.go
)
reject_matches "forbidden dependency or lifecycle escape found" sensitive \
  "$forbidden" "${runtime_contract_files[@]}"
composition_escape='os\.Getenv|time\.(NewTicker|Tick|AfterFunc)|log\.(Fatal|Fatalf|Fatalln)'
reject_matches "forbidden composition lifecycle escape found" sensitive \
  "$composition_escape" "${implementation_files[@]}" internal/platform/runtime/runtime_test.go

reject_matches "placeholder makes a readiness claim" insensitive \
  '(^|[^[:alnum:]_])(ready|healthy|river initialized)([^[:alnum:]_]|$)' \
  "${implementation_files[@]}"

module_path="$(go list -m -f '{{.Path}}')"
unexpected_dependencies="$(
  go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' \
    ./internal/platform/runtime |
    awk -v module="$module_path" 'NF && index($0, module) != 1'
)"
[[ -z "$unexpected_dependencies" ]] || {
  printf '%s\n' "$unexpected_dependencies" >&2
  echo "p0-s01-static: Slice imports a non-standard external dependency" >&2
  exit 1
}

echo "p0-s01-static: PASS"
