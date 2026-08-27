#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'staging-smoke: %s\n' "$1" >&2
  exit 2
}

usage='Usage: staging_smoke.sh --base-url=<http://host:port> [--core-read-path=</api/...>]'
base_url=''
core_read_path='/api/v1/products'

for argument in "$@"; do
  case "$argument" in
    --base-url=*)
      [[ -z "$base_url" ]] || fail 'duplicate --base-url'
      base_url="${argument#--base-url=}"
      ;;
    --core-read-path=*)
      [[ "$core_read_path" = '/api/v1/products' ]] || fail 'duplicate --core-read-path'
      core_read_path="${argument#--core-read-path=}"
      ;;
    --help)
      printf '%s\n' "$usage"
      exit 0
      ;;
    *)
      fail 'invalid argument'
      ;;
  esac
done

[[ -n "$base_url" ]] || fail '--base-url is required'
case "$base_url" in
  http://*|https://*) ;;
  *) fail '--base-url must use http or https' ;;
esac
base_url="${base_url%/}"
[[ "$core_read_path" = /* && "$core_read_path" != *[[:space:]]* ]] ||
  fail '--core-read-path must be an absolute path without whitespace'
command -v curl >/dev/null 2>&1 || fail 'curl is required'

timeout_seconds="${AICRM_STAGING_SMOKE_TIMEOUT_SECONDS:-10}"
[[ "$timeout_seconds" =~ ^[1-9][0-9]*$ ]] || fail 'AICRM_STAGING_SMOKE_TIMEOUT_SECONDS must be a positive integer'

get() {
  local label="$1"
  local path="$2"
  shift 2
  if ! curl --fail --silent --show-error --max-time "$timeout_seconds" \
    --output /dev/null "$@" "$base_url$path"; then
    fail "$label failed: $path"
  fi
  printf 'staging-smoke: %s passed\n' "$label"
}

get_contains() {
  local label="$1"
  local path="$2"
  local marker="$3"
  local body
  if ! body="$(curl --fail --silent --show-error --max-time "$timeout_seconds" "$base_url$path")"; then
    fail "$label failed: $path"
  fi
  [[ "$body" == *"$marker"* ]] || fail "$label response marker missing: $path"
  printf 'staging-smoke: %s passed\n' "$label"
}

# /readyz is the public worker/queue signal: the API only returns 200 when
# the readiness evaluator reports queues healthy (or an explicitly allowed
# warning state). No queue-specific endpoint exists in the current contract.
get_contains 'login entry' '/login' '登录运营工作台'
get 'API health' '/healthz'
get 'readiness and worker queue health' '/readyz'

if [[ -n "${AICRM_STAGING_SESSION_COOKIE:-}" ]]; then
  # The value is the complete cookie pair, for example
  # aicrm_session=<opaque-value>; it is never printed by this script.
  get 'authenticated session' '/api/v1/auth/session' \
    --header "Cookie: ${AICRM_STAGING_SESSION_COOKIE}"
  get 'authenticated core read' "$core_read_path" \
    --header "Cookie: ${AICRM_STAGING_SESSION_COOKIE}"
  printf 'staging-smoke: authenticated session and core read passed\n'
else
  blocker='AICRM_STAGING_SESSION_COOKIE is not set; authenticated session and core read were NOT EXECUTED'
  if [[ "${AICRM_STAGING_REQUIRE_AUTH_SMOKE:-0}" = '1' ]]; then
    fail "$blocker (blocker)"
  fi
  printf 'staging-smoke: %s (blocker)\n' "$blocker" >&2
fi
