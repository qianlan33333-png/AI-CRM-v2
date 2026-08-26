#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'staging-deploy: %s\n' "$1" >&2
  exit 2
}

usage='Usage: staging_deploy.sh --tier=<s|m|l> --output-dir=<directory> [--render-only|--apply]'
script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
tier_value=''
output_directory=''
deployment_mode='render-only'
mode_seen=0

for argument in "$@"; do
  case "$argument" in
    --tier=*)
      [[ -z "$tier_value" ]] || fail 'duplicate --tier'
      tier_value="${argument#--tier=}"
      ;;
    --output-dir=*)
      [[ -z "$output_directory" ]] || fail 'duplicate --output-dir'
      output_directory="${argument#--output-dir=}"
      ;;
    --render-only)
      [[ "$mode_seen" -eq 0 ]] || fail 'deployment mode may be specified once'
      deployment_mode='render-only'
      mode_seen=1
      ;;
    --apply)
      [[ "$mode_seen" -eq 0 ]] || fail 'deployment mode may be specified once'
      deployment_mode='apply'
      mode_seen=1
      ;;
    --help)
      printf '%s\n' "$usage"
      exit 0
      ;;
    *) fail 'invalid argument' ;;
  esac
done

[[ "$tier_value" = 's' || "$tier_value" = 'm' || "$tier_value" = 'l' ]] || fail '--tier must be one of s, m, l'
[[ -n "$output_directory" ]] || fail '--output-dir is required'
command -v go >/dev/null 2>&1 || fail 'Go 1.26.6 is required to render configuration'
[[ "$(go env GOVERSION)" = 'go1.26.6' ]] || fail 'Go 1.26.6 is required to render configuration'

case "$output_directory" in
  /*) ;;
  *) invocation_directory="$(pwd -P)"; output_directory="$invocation_directory/$output_directory" ;;
esac

(
  cd "$repository_root"
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
    go run ./cmd/aicrm-config --tier="$tier_value" --output-dir="$output_directory"
)

resolved_output_directory="$(CDPATH= cd -- "$output_directory" && pwd -P)"
environment_file="$resolved_output_directory/aicrm.env"
postgresql_file="$resolved_output_directory/postgresql.conf"
[[ -f "$environment_file" && ! -L "$environment_file" ]] || fail 'regular generated environment file is required'
[[ -f "$postgresql_file" && ! -L "$postgresql_file" ]] || fail 'regular generated PostgreSQL file is required'

if [[ "$deployment_mode" = 'render-only' ]]; then
  printf 'staging-deploy: rendered tier %s configuration; deployment NOT EXECUTED\n' "$tier_value"
  exit 0
fi

[[ "${AICRM_ALLOW_STAGING_DEPLOY:-}" = '1' ]] || fail 'AICRM_ALLOW_STAGING_DEPLOY=1 is required for --apply'
[[ -n "${AICRM_IMAGE:-}" ]] || fail 'AICRM_IMAGE is required for --apply'
[[ -n "${AICRM_DATABASE_URL:-}" ]] || fail 'AICRM_DATABASE_URL is required for --apply'
[[ -n "${AICRM_POSTGRES_PASSWORD:-}" ]] || fail 'AICRM_POSTGRES_PASSWORD is required for --apply'
command -v docker >/dev/null 2>&1 || fail 'docker with Compose v2 is required for --apply'
docker compose version >/dev/null 2>&1 || fail 'docker with Compose v2 is required for --apply'
command -v pg_dump >/dev/null 2>&1 || fail 'pg_dump is required for the pre-migration snapshot'
command -v curl >/dev/null 2>&1 || fail 'curl is required for the readiness smoke test'

swap_target_mib="$(awk -F= '$1 == "AICRM_SWAP_TARGET_MIB" { print $2 }' "$environment_file")"
swap_policy="$(awk -F= '$1 == "AICRM_SWAP_POLICY" { print $2 }' "$environment_file")"
[[ "$swap_target_mib" =~ ^[0-9]+$ ]] || fail 'generated swap target is invalid'
[[ "$swap_policy" = 'required' || "$swap_policy" = 'recommended' || "$swap_policy" = 'optional' ]] || fail 'generated swap policy is invalid'
if [[ "$swap_policy" = 'required' ]]; then
  command -v swapon >/dev/null 2>&1 || fail 'swapon is required to verify S tier swap'
  available_swap_bytes="$(swapon --show=SIZE --bytes --noheadings | awk '{ total += $1 } END { print total + 0 }')"
  if (( available_swap_bytes < swap_target_mib * 1024 * 1024 )); then
    fail 'S tier requires at least 4096 MiB active swap'
  fi
fi

export AICRM_GENERATED_ENV_FILE="$environment_file"
export AICRM_POSTGRESQL_CONF_FILE="$postgresql_file"
set -a
# The generated file is deterministic, contains no credentials, and is the
# deployment owner's source for COMPOSE_PROFILES and bounded runtime sizing.
. "$environment_file"
set +a
docker compose --env-file "$environment_file" -f "$repository_root/deploy/compose.yml" config --quiet
docker compose --env-file "$environment_file" -f "$repository_root/deploy/compose.yml" up -d --wait postgres

snapshot_file="$(mktemp "$resolved_output_directory/staging-pre-migration.XXXXXX.dump")"
pg_dump --format=custom --file="$snapshot_file" "$AICRM_DATABASE_URL"
chmod 600 "$snapshot_file"

(
  cd "$repository_root"
  GOOSE_DRIVER=postgres GOOSE_DBSTRING="$AICRM_DATABASE_URL" \
    GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
    go tool -modfile=tools/go.mod goose -dir migrations up
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
    go run ./cmd/aicrm-river-migrate --direction=up
)

docker compose --env-file "$environment_file" -f "$repository_root/deploy/compose.yml" up -d --wait
readiness_url="http://127.0.0.1:${AICRM_HTTP_PORT:-8080}/readyz"
curl --fail --silent --show-error --max-time 10 "$readiness_url" >/dev/null
printf 'staging-deploy: tier %s snapshot, Goose, River, Compose and /readyz completed; rollback NOT EXECUTED\n' "$tier_value"
