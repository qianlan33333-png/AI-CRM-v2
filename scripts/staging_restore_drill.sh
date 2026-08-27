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
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"

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
  printf 'staging-restore-drill: planned stop current app, switch to the approved rollback image, pg_restore "%s", restart and smoke; NOT EXECUTED\n' "$snapshot_file"
  printf 'staging-restore-drill: apply requires explicit staging approval plus current and rollback release identities\n'
  exit 0
fi

[[ "${AICRM_ALLOW_STAGING_RESTORE:-}" = '1' ]] ||
  fail 'AICRM_ALLOW_STAGING_RESTORE=1 is required for --apply'
[[ "${AICRM_ENV:-}" = 'staging' ]] || fail 'AICRM_ENV=staging is required for --apply'
[[ -n "${AICRM_DATABASE_URL:-}" ]] || fail 'AICRM_DATABASE_URL is required for --apply'
[[ "${AICRM_RELEASE_SHA:-}" =~ ^[a-f0-9]{40}$ ]] || fail 'AICRM_RELEASE_SHA must identify the current release for --apply'
[[ "${AICRM_ROLLBACK_RELEASE_SHA:-}" =~ ^[a-f0-9]{40}$ ]] || fail 'AICRM_ROLLBACK_RELEASE_SHA must identify the rollback release for --apply'
[[ "$AICRM_ROLLBACK_RELEASE_SHA" != "$AICRM_RELEASE_SHA" ]] || fail 'rollback release must differ from the current release'
[[ "${AICRM_ROLLBACK_IMAGE:-}" =~ @sha256:[a-f0-9]{64}$ ]] || fail 'AICRM_ROLLBACK_IMAGE must be pinned with @sha256:<64 lowercase hex>'
[[ -f "${AICRM_GENERATED_ENV_FILE:-}" && ! -L "${AICRM_GENERATED_ENV_FILE:-}" ]] || fail 'AICRM_GENERATED_ENV_FILE must be a regular file for --apply'
[[ -f "${AICRM_POSTGRESQL_CONF_FILE:-}" && ! -L "${AICRM_POSTGRESQL_CONF_FILE:-}" ]] || fail 'AICRM_POSTGRESQL_CONF_FILE must be a regular file for --apply'
[[ -n "${AICRM_POSTGRES_PASSWORD:-}" ]] || fail 'AICRM_POSTGRES_PASSWORD is required for --apply'
[[ -n "${AICRM_IDENTITY_HMAC_KEY:-}" ]] || fail 'AICRM_IDENTITY_HMAC_KEY is required for --apply'
[[ -n "${AICRM_STAGING_SESSION_COOKIE:-}" ]] || fail 'AICRM_STAGING_SESSION_COOKIE is required for post-restore authenticated smoke'
[[ "$edge_base_url_seen" -eq 1 && -n "$edge_base_url" ]] || fail '--edge-base-url is required for --apply'
command -v docker >/dev/null 2>&1 || fail 'docker with Compose v2 is required for --apply'
docker compose version >/dev/null 2>&1 || fail 'docker with Compose v2 is required for --apply'
command -v pg_restore >/dev/null 2>&1 || fail 'pg_restore is required for --apply'
command -v curl >/dev/null 2>&1 || fail 'curl is required for the post-restore smoke'

docker pull "$AICRM_ROLLBACK_IMAGE" >/dev/null
rollback_image_release_sha="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$AICRM_ROLLBACK_IMAGE")"
[[ "$rollback_image_release_sha" = "$AICRM_ROLLBACK_RELEASE_SHA" ]] || fail 'rollback image revision label must equal AICRM_ROLLBACK_RELEASE_SHA'

export AICRM_IMAGE="$AICRM_ROLLBACK_IMAGE"
export AICRM_RELEASE_SHA="$AICRM_ROLLBACK_RELEASE_SHA"
compose=(docker compose --env-file "$AICRM_GENERATED_ENV_FILE" -f "$repository_root/deploy/compose.yml")
"${compose[@]}" config --quiet
"${compose[@]}" stop app api worker

pg_restore --exit-on-error --clean --if-exists --no-owner \
  --dbname="$AICRM_DATABASE_URL" "$snapshot_file"
"${compose[@]}" up -d --wait
AICRM_STAGING_REQUIRE_AUTH_SMOKE=1 "$script_directory/staging_smoke.sh" --base-url="$edge_base_url"
printf 'staging-restore-drill: rollback image, pg_restore and post-restore authenticated smoke completed\n'
