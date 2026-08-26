#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'staging-restore-drill: %s\n' "$1" >&2
  exit 2
}

usage='Usage: staging_restore_drill.sh --snapshot=<custom-format.dump> [--edge-base-url=<http://host:port>] [--render-only|--apply]'
snapshot_file=''
restore_mode='render-only'
mode_seen=0
edge_base_url="http://127.0.0.1:${AICRM_HTTP_PORT:-8080}"
edge_base_url_seen=0
script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"

for argument in "$@"; do
  case "$argument" in
    --snapshot=*)
      [[ -z "$snapshot_file" ]] || fail 'duplicate --snapshot'
      snapshot_file="${argument#--snapshot=}"
      ;;
    --edge-base-url=*)
      [[ "$edge_base_url_seen" -eq 0 ]] || fail 'duplicate --edge-base-url'
      edge_base_url="${argument#--edge-base-url=}"
      edge_base_url_seen=1
      ;;
    --render-only)
      [[ "$mode_seen" -eq 0 ]] || fail 'restore mode may be specified once'
      restore_mode='render-only'
      mode_seen=1
      ;;
    --apply)
      [[ "$mode_seen" -eq 0 ]] || fail 'restore mode may be specified once'
      restore_mode='apply'
      mode_seen=1
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

[[ -n "$snapshot_file" ]] || fail '--snapshot is required'
[[ -f "$snapshot_file" && ! -L "$snapshot_file" ]] || fail 'snapshot must be a regular file'
[[ -s "$snapshot_file" ]] || fail 'snapshot must not be empty'

if [[ "$restore_mode" = 'render-only' ]]; then
  printf 'staging-restore-drill: planned pg_restore --exit-on-error --clean --if-exists --no-owner --dbname="$AICRM_DATABASE_URL" "%s"; NOT EXECUTED\n' "$snapshot_file"
  printf 'staging-restore-drill: apply requires AICRM_ALLOW_STAGING_RESTORE=1 and a staging AICRM_DATABASE_URL\n'
  exit 0
fi

[[ "${AICRM_ALLOW_STAGING_RESTORE:-}" = '1' ]] ||
  fail 'AICRM_ALLOW_STAGING_RESTORE=1 is required for --apply'
[[ -n "${AICRM_DATABASE_URL:-}" ]] || fail 'AICRM_DATABASE_URL is required for --apply'
[[ -n "${AICRM_STAGING_SESSION_COOKIE:-}" ]] || fail 'AICRM_STAGING_SESSION_COOKIE is required for post-restore authenticated smoke'
[[ "$edge_base_url_seen" -eq 1 && -n "$edge_base_url" ]] || fail '--edge-base-url is required for --apply'
command -v pg_restore >/dev/null 2>&1 || fail 'pg_restore is required for --apply'

pg_restore --exit-on-error --clean --if-exists --no-owner \
  --dbname="$AICRM_DATABASE_URL" "$snapshot_file"
AICRM_STAGING_REQUIRE_AUTH_SMOKE=1 "$script_directory/staging_smoke.sh" --base-url="$edge_base_url"
printf 'staging-restore-drill: pg_restore and post-restore authenticated smoke completed\n'
