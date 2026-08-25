#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

selection_mode="${1:-}"
selected_groups="${2:-}"
run_vulnerability="${3:-false}"

fail() {
  printf 'ci-go-selected: %s\n' "$1" >&2
  exit 2
}

[[ "$selection_mode" = "selected" || "$selection_mode" = "full" ]] ||
  fail "mode must be selected or full"
[[ "$run_vulnerability" = "true" || "$run_vulnerability" = "false" ]] ||
  fail "vulnerability flag must be true or false"
command -v go >/dev/null 2>&1 || fail "go is required"

if [[ "$selection_mode" = "full" ]]; then
  make --no-print-directory mod-check generate-check gitless-generate-test fmt-check vet test build
  if [[ "$run_vulnerability" = "true" ]]; then
    make --no-print-directory vuln
  fi
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go build ./cmd/aicrm
  printf 'ci-go-selected: PASS mode=full\n'
  exit 0
fi

[[ -n "$selected_groups" ]] || fail "selected mode requires at least one group"
declare -a packages=()
declare -A package_seen=()

add_package() {
  local package_name="$1"
  local package_directory="${package_name%/...}"
  package_directory="${package_directory#./}"
  if [[ ! -d "$package_directory" ]]; then
    return
  fi
  if [[ -z "${package_seen[$package_name]:-}" ]]; then
    packages+=("$package_name")
    package_seen["$package_name"]=1
  fi
}

add_domain_group() {
  local domain_name="$1"
  add_package "./internal/${domain_name}/..."
  add_package "./acceptance/${domain_name}/..."
  add_package "./acceptance/${domain_name}fixture/..."
  add_package "./cmd/aicrm"
}

remaining_groups="$selected_groups"
while [[ -n "$remaining_groups" ]]; do
  if [[ "$remaining_groups" = *,* ]]; then
    group_name="${remaining_groups%%,*}"
    remaining_groups="${remaining_groups#*,}"
  else
    group_name="$remaining_groups"
    remaining_groups=""
  fi
  [[ "$group_name" =~ ^[a-z0-9][a-z0-9-]*$ ]] || fail "invalid group: $group_name"
  case "$group_name" in
    composition)
      add_package ./cmd/aicrm
      ;;
    migration)
      add_package ./internal/migration/...
      add_package ./acceptance/datamigration/...
      ;;
    adminops|ai|auth|automation|config|contact|coupon|events|externaleffects|gateway|identity|media|operationcycle|ops|order|outbound|product|pushcenter|radar|segment|stats|survey|wecom)
      add_domain_group "$group_name"
      ;;
    *)
      fail "unsupported selected Go group: $group_name"
      ;;
  esac
done

(( ${#packages[@]} > 0 )) || fail "selected groups resolved to no packages"
make --no-print-directory fmt-check
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go vet "${packages[@]}"
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go test -race -count=1 -timeout=240s "${packages[@]}"
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go build ./cmd/aicrm
printf 'ci-go-selected: PASS mode=selected groups=%s\n' "$selected_groups"
