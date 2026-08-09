#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "slice-input-contract: $*" >&2
  exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$script_dir/.." && pwd -P)"
cards_dir="$repo_root/docs/execution/slices"
[[ -d "$cards_dir" && ! -L "$cards_dir" ]] || fail "real slice-card directory required"

checked=0
for card in "$cards_dir"/*.md; do
  [[ -f "$card" && ! -L "$card" ]] || fail "regular slice card required: ${card#"$repo_root/"}"
  [[ "$(grep -Fxc -- '- slice_kind: implementation' "$card" || true)" = "1" ]] || continue
  checked=$((checked + 1))
  block_count="$(grep -Fxc -- '- task_inputs:' "$card" || true)"
  [[ "$block_count" = "1" ]] || fail "implementation slice must declare exactly one task_inputs block: ${card##*/}"
  inputs="$(awk '
    $0 == "- task_inputs:" { inside = 1; next }
    inside && /^  - / { print substr($0, 5); count++; next }
    inside && /^[^[:space:]]/ { inside = 0 }
    END { exit !(count > 0) }
  ' "$card")" || fail "implementation slice task_inputs must not be empty: ${card##*/}"
  while IFS= read -r input; do
    input="${input#\`}"
    input="${input%\`}"
    lower="$(printf '%s' "$input" | tr '[:upper:]' '[:lower:]')"
    case "$lower" in
      /*|*..*|*\\*|*\?*|*\#*|*%*|*.py|*.py/*|*aicrm_next/*|*legacy-snapshots/*|*ai-crm-main@*)
        fail "legacy Python input forbidden for implementation slice: ${card##*/}: $input"
        ;;
    esac
    case "$input" in
      docs/rules/*.md|docs/evidence/p1/api-facts-*.md|docs/spec/*.md|api/openapi.yaml) ;;
      *) fail "implementation task input is not allowed: ${card##*/}: $input" ;;
    esac
  done <<<"$inputs"
done

printf 'slice-input-contract: PASS (implementation_cards=%s)\n' "$checked"
