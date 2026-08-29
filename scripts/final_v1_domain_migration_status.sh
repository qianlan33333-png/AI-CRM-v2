#!/usr/bin/env bash
set -euo pipefail
script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
go_command="$(command -v go 2>/dev/null || true)"
[[ "$go_command" = /* && -x "$go_command" ]] || { echo 'final-v1-domain-migration-status: go must resolve to an executable absolute path' >&2; exit 2; }
cd "$repository_root"
exec env GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./scripts/final_v1_domain_migration_status "$@"
