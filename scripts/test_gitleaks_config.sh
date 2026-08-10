#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'gitleaks-config-tests: %s\n' "$1" >&2; exit 1; }

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/.." && pwd -P)"
config="$repo_root/.gitleaks.toml"

[[ $# -eq 0 ]] || fail "unexpected argument"
[[ -f "$config" && ! -L "$config" ]] || fail "regular config required"
command -v gitleaks >/dev/null 2>&1 || fail "gitleaks is required"
[[ "$(gitleaks version)" == "8.30.1" ]] || fail "gitleaks 8.30.1 required"

fixture="$(mktemp -d "${TMPDIR:-/tmp}/gitleaks-config.XXXXXX")"
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
generated_dir="$fixture/web/src/api/generated"
mkdir -p "$generated_dir"

version_core='0.2.0'
version_suffix='-g1-frozen'
printf '/**\n * OpenAPI spec version: %s%s\n */\n' "$version_core" "$version_suffix" \
  >"$generated_dir/health.ts"
gitleaks dir --config "$config" --redact=100 --no-banner --exit-code 1 \
  "$fixture" >/dev/null 2>&1 || fail "exact generated version banner was rejected"

fragment_a='vT8zQ4mN2xR7cL5p'
fragment_b='K9sW3dF6hJ1uY0eB'
printf 'api_key = "%s%s"\n' "$fragment_a" "$fragment_b" \
  >"$generated_dir/real-secret.ts"
report="$fixture/report.json"
if gitleaks dir --config "$config" --redact=100 --no-banner --exit-code 1 \
  --report-format json --report-path "$report" "$fixture" >/dev/null 2>&1; then
  fail "real generic API key was accepted"
fi
grep -F '"RuleID": "generic-api-key"' "$report" >/dev/null 2>&1 || \
  fail "real key did not reach generic-api-key rule"
grep -F 'real-secret.ts' "$report" >/dev/null 2>&1 || \
  fail "real key finding did not identify its file"

printf 'gitleaks-config-tests: PASS\n'
