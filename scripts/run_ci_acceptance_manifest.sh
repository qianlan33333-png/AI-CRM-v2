#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

manifest="${CI_ACCEPTANCE_MANIFEST:-docs/ci/go-acceptance-manifest.tsv}"
database_url="${CI_ACCEPTANCE_DATABASE_URL:-}"
make_bin="${MAKE:-make}"

fail() {
  echo "ci-acceptance-manifest: $*" >&2
  exit 1
}

[[ -n "$database_url" ]] || fail "CI_ACCEPTANCE_DATABASE_URL is required"
[[ -f "$manifest" && ! -L "$manifest" ]] || fail "manifest must be a regular file: $manifest"
if [[ "$make_bin" = */* ]]; then
  [[ -x "$make_bin" ]] || fail "MAKE is not executable: $make_bin"
else
  command -v "$make_bin" >/dev/null 2>&1 || fail "MAKE command is unavailable: $make_bin"
fi

count=0
line_number=0
while IFS='|' read -r identifier environment target extra || [[ -n "${identifier}${environment}${target}${extra}" ]]; do
  line_number=$((line_number + 1))
  [[ -z "$identifier" || "$identifier" = \#* ]] && continue
  [[ -z "$extra" ]] || fail "line $line_number has more than three fields"
  [[ "$identifier" =~ ^[a-z0-9][a-z0-9-]*$ ]] || fail "line $line_number has an invalid id"
  [[ "$environment" = "-" || "$environment" =~ ^[A-Z][A-Z0-9_]*$ ]] || fail "line $line_number has an invalid environment"
  [[ "$target" =~ ^[a-z0-9][a-z0-9-]*$ ]] || fail "line $line_number has an invalid Make target"
  if [[ "$environment" = "-" ]]; then
    "$make_bin" --no-print-directory "$target"
  else
    env "$environment=$database_url" "$make_bin" --no-print-directory "$target"
  fi
  count=$((count + 1))
done <"$manifest"

(( count > 0 )) || fail "manifest has no acceptance targets"
echo "ci-acceptance-manifest: PASS entries=$count"
