#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
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

forbidden='"net"|"net/http"|github\.com/jackc/pgx|github\.com/riverqueue/river|os\.Getenv|time\.(NewTicker|Tick|AfterFunc)|log\.(Fatal|Fatalf|Fatalln)'
if rg -n "$forbidden" "${implementation_files[@]}" internal/platform/runtime/runtime_test.go; then
  echo "p0-s01-static: forbidden dependency or lifecycle escape found" >&2
  exit 1
fi

if rg -ni '\b(ready|healthy|river initialized)\b' "${implementation_files[@]}"; then
  echo "p0-s01-static: placeholder makes a readiness claim" >&2
  exit 1
fi

module_path="$(go list -m -f '{{.Path}}')"
unexpected_dependencies="$(
  go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' \
    ./cmd/aicrm ./internal/platform/runtime |
    awk -v module="$module_path" 'NF && index($0, module) != 1'
)"
[[ -z "$unexpected_dependencies" ]] || {
  printf '%s\n' "$unexpected_dependencies" >&2
  echo "p0-s01-static: Slice imports a non-standard external dependency" >&2
  exit 1
}

echo "p0-s01-static: PASS"
