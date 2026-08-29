#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'final-v1-domain-migration-runtime: %s\n' "$1" >&2
  exit 2
}

runtime_env_file='' action='' services='' web='' expected_sha=''
for argument in "$@"; do
  case "$argument" in
    --runtime-env-file=*) [[ -z "$runtime_env_file" ]] || fail 'duplicate --runtime-env-file'; runtime_env_file="${argument#--runtime-env-file=}" ;;
    --stop=*) [[ -z "$action" ]] || fail 'duplicate runtime action'; action='stop'; services="${argument#--stop=}" ;;
    --check=stopped) [[ -z "$action" ]] || fail 'duplicate runtime action'; action='check' ;;
    --check=release) [[ -z "$action" ]] || fail 'duplicate runtime action'; action='release' ;;
    --check=compose-config) [[ -z "$action" ]] || fail 'duplicate runtime action'; action='config' ;;
    --expected-sha=*) [[ -z "$expected_sha" ]] || fail 'duplicate --expected-sha'; expected_sha="${argument#--expected-sha=}" ;;
    --services=*) [[ -z "$services" ]] || fail 'duplicate --services'; services="${argument#--services=}" ;;
    --start=*) [[ -z "$action" ]] || fail 'duplicate runtime action'; action='start'; services="${argument#--start=}" ;;
    --web=*) [[ -z "$web" ]] || fail 'duplicate --web'; web="${argument#--web=}" ;;
    *) fail 'invalid runtime argument' ;;
  esac
done
[[ "$runtime_env_file" = /* && -f "$runtime_env_file" && ! -L "$runtime_env_file" ]] || fail 'runtime environment file is invalid'
case "$action" in
  stop|check) [[ "$services" = 'app,api,worker' ]] || fail 'runtime services are invalid' ;;
  start) [[ "$services" = 'api,worker' ]] || fail 'runtime services are invalid' ;;
  release|config) ;;
  *) fail 'runtime action is required' ;;
esac
script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
compose_file="$repository_root/deploy/compose.yml"
[[ -f "$compose_file" && ! -L "$compose_file" ]] || fail 'compose file is missing'
docker_command="$(command -v docker 2>/dev/null || true)"
[[ "$docker_command" = /* && -x "$docker_command" ]] || fail 'docker must resolve to an executable absolute path'

# Docker Compose gives inherited environment precedence over --env-file. Clear
# every interpolation key declared by deploy/compose.yml so the restricted file
# is its only source; do not mutate the file or any existing container/release.
for compose_key in AICRM_IMAGE AICRM_GENERATED_ENV_FILE AICRM_DATABASE_URL AICRM_ENV AICRM_IDENTITY_HMAC_KEY AICRM_RELEASE_SHA AICRM_POSTGRES_DB AICRM_POSTGRES_USER AICRM_POSTGRES_PASSWORD AICRM_POSTGRESQL_CONF_FILE AICRM_HTTP_PORT; do
  unset "$compose_key"
done
for compose_control in COMPOSE_PROJECT_NAME COMPOSE_PROFILES COMPOSE_FILE COMPOSE_ENV_FILES COMPOSE_PATH_SEPARATOR; do
  unset "$compose_control"
done

read_env_value() {
  local key="$1"
  local value=''
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" = "$key="* ]] && value="${line#*=}"
  done <"$runtime_env_file"
  printf '%s' "$value"
}

case "$action" in
  stop) "$docker_command" compose --env-file "$runtime_env_file" -f "$compose_file" stop app api worker ;;
  check)
    running="$("$docker_command" compose --env-file "$runtime_env_file" -f "$compose_file" ps --status running --services)"
    for service in app api worker; do
      if grep -Fxq "$service" <<<"$running"; then fail "$service must be stopped"; fi
    done
    ;;
  config)
    "$docker_command" compose --env-file "$runtime_env_file" -f "$compose_file" config --quiet
    ;;
  release)
    [[ "$expected_sha" =~ ^[a-f0-9]{40}$ ]] || fail 'expected release SHA is invalid'
    release_sha="$(read_env_value AICRM_RELEASE_SHA)"
    image="$(read_env_value AICRM_IMAGE)"
    rollback_sha="$(read_env_value AICRM_ROLLBACK_RELEASE_SHA)"
    rollback_image="$(read_env_value AICRM_ROLLBACK_IMAGE)"
    [[ "$release_sha" = "$expected_sha" ]] || fail 'AICRM_RELEASE_SHA must equal expected SHA'
    [[ "$image" =~ @sha256:[a-f0-9]{64}$ ]] || fail 'AICRM_IMAGE must be pinned by lowercase digest'
    [[ "$rollback_sha" =~ ^[a-f0-9]{40}$ && "$rollback_sha" != "$expected_sha" ]] || fail 'rollback SHA must be distinct and valid'
    [[ "$rollback_image" =~ @sha256:[a-f0-9]{64}$ ]] || fail 'rollback image must be pinned by lowercase digest'
    "$docker_command" pull "$image" >/dev/null
    [[ "$("$docker_command" image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$image")" = "$expected_sha" ]] || fail 'AICRM_IMAGE revision label must equal expected SHA'
    "$docker_command" pull "$rollback_image" >/dev/null
    [[ "$("$docker_command" image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$rollback_image")" = "$rollback_sha" ]] || fail 'rollback image revision label must equal rollback SHA'
    ;;
  start)
    [[ "$services" = 'api,worker' && "$web" = 'api' ]] || fail 'only split api+worker with web=api may start'
    "$docker_command" compose --env-file "$runtime_env_file" -f "$compose_file" --profile split up -d --wait api worker
    ;;
  *) fail 'runtime action is required' ;;
esac
