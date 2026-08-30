#!/usr/bin/env bash
set -euo pipefail

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
grep -Fq "CREATE DATABASE aicrm_v2_core" "$root/scripts/whitelist_deploy.sh"
grep -Fq "AICRM_FINAL_RUNTIME_MODE" "$root/scripts/whitelist_deploy.sh"
grep -Fq -- "--mode=reconcile" "$root/scripts/whitelist_deploy.sh"
grep -Fq "whitelist_smoke.sh" "$root/scripts/whitelist_deploy.sh"
grep -Fq "legacy campaign route is absent" "$root/scripts/whitelist_smoke.sh"
grep -Fq "edit_and_readback product" "$root/scripts/whitelist_smoke.sh"
grep -Fq "edit_and_readback audience" "$root/scripts/whitelist_smoke.sh"
grep -Fq "edit_and_readback automation" "$root/scripts/whitelist_smoke.sh"
if grep -Eqi 'nightly|workflow_dispatch|gh workflow run' "$root/scripts/whitelist_deploy.sh" "$root/scripts/whitelist_smoke.sh"; then
  printf 'whitelist release scripts must not invoke Nightly\n' >&2
  exit 1
fi
printf 'whitelist release contract passed\n'
