#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'staging-restore-drill: %s\n' "$1" >&2
  exit 2
}

sha256_text() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$1" | sha256sum | awk '{ print $1 }'
  else
    command -v shasum >/dev/null 2>&1 || fail 'sha256sum or shasum is required for the snapshot receipt'
    printf '%s' "$1" | shasum -a 256 | awk '{ print $1 }'
  fi
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  else
    command -v shasum >/dev/null 2>&1 || fail 'sha256sum or shasum is required for the snapshot receipt'
    shasum -a 256 "$1" | awk '{ print $1 }'
  fi
}

read_snapshot_receipt() {
  local receipt_file="$1"
  local line
  local key
  local value
  local seen_format=0
  local seen_target=0
  local seen_snapshot=0
  local seen_database=0
  local seen_edge=0
  local seen_current=0
  local seen_rollback_sha=0
  local seen_rollback_image=0

  [[ -f "$receipt_file" && ! -L "$receipt_file" && -s "$receipt_file" ]] ||
    fail 'snapshot receipt must be a non-empty regular file for --apply'
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" = *=* ]] || fail 'snapshot receipt contains an invalid line'
    key="${line%%=*}"
    value="${line#*=}"
    [[ -n "$value" ]] || fail 'snapshot receipt contains an empty value'
    case "$key" in
      format_version) (( seen_format == 0 )) || fail 'snapshot receipt repeats format_version'; snapshot_receipt_format="$value"; seen_format=1 ;;
      staging_target_id) (( seen_target == 0 )) || fail 'snapshot receipt repeats staging_target_id'; snapshot_receipt_target_id="$value"; seen_target=1 ;;
      snapshot_sha256) (( seen_snapshot == 0 )) || fail 'snapshot receipt repeats snapshot_sha256'; snapshot_receipt_sha256="$value"; seen_snapshot=1 ;;
      database_fingerprint) (( seen_database == 0 )) || fail 'snapshot receipt repeats database_fingerprint'; snapshot_receipt_database_fingerprint="$value"; seen_database=1 ;;
      edge_fingerprint) (( seen_edge == 0 )) || fail 'snapshot receipt repeats edge_fingerprint'; snapshot_receipt_edge_fingerprint="$value"; seen_edge=1 ;;
      current_release_sha) (( seen_current == 0 )) || fail 'snapshot receipt repeats current_release_sha'; snapshot_receipt_current_release_sha="$value"; seen_current=1 ;;
      rollback_release_sha) (( seen_rollback_sha == 0 )) || fail 'snapshot receipt repeats rollback_release_sha'; snapshot_receipt_rollback_release_sha="$value"; seen_rollback_sha=1 ;;
      rollback_image) (( seen_rollback_image == 0 )) || fail 'snapshot receipt repeats rollback_image'; snapshot_receipt_rollback_image="$value"; seen_rollback_image=1 ;;
      *) fail 'snapshot receipt contains an unknown field' ;;
    esac
  done <"$receipt_file"
  (( seen_format == 1 && seen_target == 1 && seen_snapshot == 1 && seen_database == 1 && seen_edge == 1 && seen_current == 1 && seen_rollback_sha == 1 && seen_rollback_image == 1 )) ||
    fail 'snapshot receipt is missing a required field'
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
[[ "${AICRM_STAGING_TARGET_ID:-}" =~ ^[A-Za-z0-9._-]{1,128}$ ]] || fail 'AICRM_STAGING_TARGET_ID must be a stable staging target identifier for --apply'
[[ -f "${AICRM_GENERATED_ENV_FILE:-}" && ! -L "${AICRM_GENERATED_ENV_FILE:-}" ]] || fail 'AICRM_GENERATED_ENV_FILE must be a regular file for --apply'
[[ -f "${AICRM_POSTGRESQL_CONF_FILE:-}" && ! -L "${AICRM_POSTGRESQL_CONF_FILE:-}" ]] || fail 'AICRM_POSTGRESQL_CONF_FILE must be a regular file for --apply'
[[ -n "${AICRM_POSTGRES_PASSWORD:-}" ]] || fail 'AICRM_POSTGRES_PASSWORD is required for --apply'
[[ -n "${AICRM_IDENTITY_HMAC_KEY:-}" ]] || fail 'AICRM_IDENTITY_HMAC_KEY is required for --apply'
[[ -n "${AICRM_STAGING_SESSION_COOKIE:-}" ]] || fail 'AICRM_STAGING_SESSION_COOKIE is required for post-restore authenticated smoke'
[[ "$edge_base_url_seen" -eq 1 && -n "$edge_base_url" ]] || fail '--edge-base-url is required for --apply'
edge_base_url="${edge_base_url%/}"
snapshot_receipt_file="${snapshot_file}.receipt"
read_snapshot_receipt "$snapshot_receipt_file"
[[ "$snapshot_receipt_format" = '1' ]] || fail 'snapshot receipt format is not supported'
[[ "$snapshot_receipt_target_id" =~ ^[A-Za-z0-9._-]{1,128}$ ]] || fail 'snapshot receipt staging target is invalid'
[[ "$snapshot_receipt_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'snapshot receipt snapshot digest is invalid'
[[ "$snapshot_receipt_database_fingerprint" =~ ^[a-f0-9]{64}$ ]] || fail 'snapshot receipt database fingerprint is invalid'
[[ "$snapshot_receipt_edge_fingerprint" =~ ^[a-f0-9]{64}$ ]] || fail 'snapshot receipt edge fingerprint is invalid'
[[ "$snapshot_receipt_current_release_sha" =~ ^[a-f0-9]{40}$ ]] || fail 'snapshot receipt current release is invalid'
[[ "$snapshot_receipt_rollback_release_sha" =~ ^[a-f0-9]{40}$ ]] || fail 'snapshot receipt rollback release is invalid'
[[ "$snapshot_receipt_rollback_image" =~ @sha256:[a-f0-9]{64}$ ]] || fail 'snapshot receipt rollback image is invalid'
[[ "$snapshot_receipt_target_id" = "$AICRM_STAGING_TARGET_ID" ]] || fail 'snapshot receipt staging target does not match AICRM_STAGING_TARGET_ID'
[[ "$snapshot_receipt_sha256" = "$(sha256_file "$snapshot_file")" ]] || fail 'snapshot receipt does not match the snapshot file'
[[ "$snapshot_receipt_database_fingerprint" = "$(sha256_text "$AICRM_DATABASE_URL")" ]] || fail 'snapshot receipt database does not match AICRM_DATABASE_URL'
[[ "$snapshot_receipt_edge_fingerprint" = "$(sha256_text "$edge_base_url")" ]] || fail 'snapshot receipt edge does not match --edge-base-url'
[[ "$snapshot_receipt_current_release_sha" = "$AICRM_RELEASE_SHA" ]] || fail 'snapshot receipt current release does not match AICRM_RELEASE_SHA'
[[ "$snapshot_receipt_rollback_release_sha" = "$AICRM_ROLLBACK_RELEASE_SHA" ]] || fail 'snapshot receipt rollback release does not match AICRM_ROLLBACK_RELEASE_SHA'
[[ "$snapshot_receipt_rollback_image" = "$AICRM_ROLLBACK_IMAGE" ]] || fail 'snapshot receipt rollback image does not match AICRM_ROLLBACK_IMAGE'

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
printf 'staging-restore-drill: receipt-bound rollback image, pg_restore and post-restore authenticated smoke completed\n'
