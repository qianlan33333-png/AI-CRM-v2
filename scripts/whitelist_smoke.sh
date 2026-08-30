#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'whitelist-smoke: %s\n' "$1" >&2; exit 2; }

base_url='' output=''
for argument in "$@"; do
  case "$argument" in
    --base-url=*) base_url="${argument#--base-url=}" ;;
    --output=*) output="${argument#--output=}" ;;
    --help) printf 'Usage: whitelist_smoke.sh --base-url=<https://id-dev-host> --output=<absolute receipt.json>\n'; exit 0 ;;
    *) fail 'invalid argument' ;;
  esac
done
[[ "$base_url" = https://* || "$base_url" = http://127.0.0.1:* ]] || fail 'base URL must be id-dev HTTPS or loopback HTTP'
[[ "$output" = /* ]] || fail 'absolute --output is required'
[[ -n "${AICRM_WHITELIST_SESSION_COOKIE:-}" ]] || fail 'AICRM_WHITELIST_SESSION_COOKIE is required'
[[ -n "${AICRM_WHITELIST_CSRF_TOKEN:-}" ]] || fail 'AICRM_WHITELIST_CSRF_TOKEN is required'
[[ "${AICRM_RELEASE_SHA:-}" =~ ^[a-f0-9]{40}$ ]] || fail 'AICRM_RELEASE_SHA is invalid'
[[ "${AICRM_WHITELIST_SOURCE_DIGEST:-}" =~ ^[a-f0-9]{64}$ ]] || fail 'AICRM_WHITELIST_SOURCE_DIGEST is invalid'
command -v curl >/dev/null 2>&1 || fail 'curl is required'
base_url="${base_url%/}"
timeout_seconds="${AICRM_WHITELIST_SMOKE_TIMEOUT_SECONDS:-10}"
[[ "$timeout_seconds" =~ ^[1-9][0-9]*$ ]] || fail 'timeout must be positive'

request() {
  local label="$1" method="$2" path="$3" expected="$4" body_file status
  body_file="$(mktemp)"
  status="$(curl --silent --show-error --max-time "$timeout_seconds" --output "$body_file" --write-out '%{http_code}' \
    --request "$method" --cookie "$AICRM_WHITELIST_SESSION_COOKIE" "$base_url$path")" || { rm -f "$body_file"; fail "$label request failed"; }
  [[ "$status" = "$expected" ]] || { rm -f "$body_file"; fail "$label returned HTTP $status, expected $expected"; }
  if grep -Eqi '(^|[^a-z])(mock|seed)([^a-z]|$)' "$body_file"; then rm -f "$body_file"; fail "$label returned Mock or Seed content"; fi
  rm -f "$body_file"
  printf 'whitelist-smoke: %s passed\n' "$label"
}

request 'health' GET '/healthz' 200
request 'readiness' GET '/readyz' 200
request 'login' GET '/login' 200
request 'WeCom OAuth start' GET '/auth/wecom/start' 302
request 'authenticated session' GET '/api/v1/auth/session' 200
request 'products' GET '/api/v1/products' 200
request 'orders' GET '/api/admin/orders' 200
request 'questionnaires' GET '/api/admin/questionnaires' 200
request 'memberships' GET '/api/admin/service-period-products' 200
request 'radar' GET '/api/admin/radar-links' 200
request 'channels' GET '/api/admin/channels' 200
request 'audiences' GET '/api/admin/ai-audience/packages' 200
request 'automation agents' GET '/api/admin/automation-agents' 200
request 'legacy campaign route is absent' GET '/api/admin/campaigns' 404
request 'legacy message route is absent' GET '/api/admin/messages' 404

edit_and_readback() {
  local label="$1" method="$2" path="$3" payload="$4" marker="$5" body_file status
  [[ "$payload" = /* && -f "$payload" && ! -L "$payload" ]] || fail "$label payload file is invalid"
  [[ -n "$marker" ]] || fail "$label readback marker is required"
  body_file="$(mktemp)"
  status="$(curl --silent --show-error --max-time "$timeout_seconds" --output "$body_file" --write-out '%{http_code}' \
    --request "$method" --cookie "$AICRM_WHITELIST_SESSION_COOKIE" \
    --header "X-CSRF-Token: $AICRM_WHITELIST_CSRF_TOKEN" \
    --header "Idempotency-Key: whitelist-smoke-${AICRM_RELEASE_SHA}-${label}" \
    --header 'Content-Type: application/json' --data-binary "@$payload" "$base_url$path")" || { rm -f "$body_file"; fail "$label edit failed"; }
  [[ "$status" = 200 ]] || { rm -f "$body_file"; fail "$label edit returned HTTP $status"; }
  rm -f "$body_file"
  body_file="$(mktemp)"
  curl --fail --silent --show-error --max-time "$timeout_seconds" --cookie "$AICRM_WHITELIST_SESSION_COOKIE" --output "$body_file" "$base_url$path" || { rm -f "$body_file"; fail "$label readback failed"; }
  grep -Fq "$marker" "$body_file" || { rm -f "$body_file"; fail "$label readback marker missing"; }
  rm -f "$body_file"
  printf 'whitelist-smoke: %s edit and readback passed\n' "$label"
}

edit_and_readback product PUT "/api/v1/products/${AICRM_WHITELIST_PRODUCT_ID:?}" "${AICRM_WHITELIST_PRODUCT_EDIT_JSON:?}" "${AICRM_WHITELIST_PRODUCT_READBACK_MARKER:?}"
edit_and_readback audience PATCH "/api/admin/ai-audience/packages/${AICRM_WHITELIST_AUDIENCE_ID:?}" "${AICRM_WHITELIST_AUDIENCE_EDIT_JSON:?}" "${AICRM_WHITELIST_AUDIENCE_READBACK_MARKER:?}"
edit_and_readback automation PATCH "/api/admin/automation-agents/${AICRM_WHITELIST_AUTOMATION_ID:?}" "${AICRM_WHITELIST_AUTOMATION_EDIT_JSON:?}" "${AICRM_WHITELIST_AUTOMATION_READBACK_MARKER:?}"

temporary="$(mktemp "${output}.tmp.XXXXXX")"
printf '{"status":"passed","target_database":"aicrm_v2_core","source_digest":"%s","release_sha":"%s"}\n' \
  "$AICRM_WHITELIST_SOURCE_DIGEST" "$AICRM_RELEASE_SHA" >"$temporary"
chmod 600 "$temporary"
mv "$temporary" "$output"
printf 'whitelist-smoke: receipt=%s\n' "$output"
