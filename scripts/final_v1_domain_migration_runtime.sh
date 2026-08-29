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

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
  else
    fail 'sha256sum or shasum is required'
  fi
}

file_mode() {
  case "$(uname -s)" in
    Darwin) stat -f '%Lp' "$1" ;;
    Linux) stat -c '%a' "$1" ;;
    *) fail 'unsupported platform for file mode checks' ;;
  esac
}

manual_mode() {
  [[ "$(read_env_value AICRM_FINAL_RUNTIME_MODE)" = 'external-postgres-manual' ]]
}

require_manual_value() {
  local key="$1" value
  value="$(read_env_value "$key")"
  [[ -n "$value" ]] || fail "$key is required for external-postgres-manual"
  printf '%s' "$value"
}

require_manual_name() {
  local value="$1" label="$2"
  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]] || fail "$label is invalid"
}

require_manual_sha() {
  local value="$1" label="$2"
  [[ "$value" =~ ^[a-f0-9]{40}$ ]] || fail "$label is invalid"
}

require_manual_image_id() {
  local value="$1" label="$2"
  [[ "$value" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "$label is invalid"
}

require_manual_port() {
  local value="$1" label="$2"
  [[ "$value" =~ ^[1-9][0-9]{0,4}$ ]] && (( value <= 65535 )) || fail "$label is invalid"
}

manual_container_hardened() {
  local container="$1" state
  state="$("$docker_command" inspect --format '{{.HostConfig.ReadonlyRootfs}}|{{.HostConfig.Init}}|{{.HostConfig.RestartPolicy.Name}}|{{json .HostConfig.CapDrop}}|{{json .HostConfig.SecurityOpt}}|{{json .HostConfig.Tmpfs}}' "$container")"
  [[ "$state" = true\|true\|unless-stopped\|* ]] || fail "$container is not read-only/init/restart hardened"
  [[ "$state" = *'"ALL"'* && "$state" = *'no-new-privileges:true'* && "$state" = *'"/tmp"'* && "$state" = *'size=64m,mode=1777'* ]] || fail "$container is missing hardened tmpfs/capability/security settings"
}

manual_container_network() {
  local container="$1" network="$2" actual
  actual="$("$docker_command" inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}' "$container")"
  [[ "$actual" = "$network" ]] || fail "$container is not attached only to the expected network"
}

manual_image_matches() {
  local image="$1" expected_id="$2" expected_sha="$3" label="$4" actual_id actual_sha
  actual_id="$("$docker_command" image inspect --format '{{.Id}}' "$image")"
  [[ "$actual_id" = "$expected_id" ]] || fail "$label image ID does not match the declared local image"
  actual_sha="$("$docker_command" image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$image")"
  [[ "$actual_sha" = "$expected_sha" ]] || fail "$label image revision label does not match its SHA"
}

manual_tail_bounds() {
  local file="$1" host="$2" begin_line end_line trailing block
  begin_line="$(grep -n -F -x '# AICRM_ID_DEV_BEGIN' "$file" | cut -d: -f1 || true)"
  end_line="$(grep -n -F -x '# AICRM_ID_DEV_END' "$file" | cut -d: -f1 || true)"
  [[ "$begin_line" =~ ^[1-9][0-9]*$ && "$end_line" =~ ^[1-9][0-9]*$ && "$begin_line" -lt "$end_line" ]] || fail 'id-dev Caddy markers must occur exactly once in order'
  block="$(sed -n "${begin_line},${end_line}p" "$file")"
  grep -F -x -q "$host {" <<<"$block" || fail 'id-dev Caddy host is missing from the marked tail'
  trailing="$(tail -n +$((end_line + 1)) "$file")"
  [[ -z "${trailing//[[:space:]]/}" ]] || fail 'id-dev Caddy block must be the last block'
  printf '%s:%s' "$begin_line" "$end_line"
}

manual_legacy_tail_bounds() {
  local file="$1" host="$2" marker_line host_line total_lines trailing
  marker_line="$(grep -n -F -x '# V2 independent manual-test login. Existing aa site is unchanged.' "$file" | cut -d: -f1 || true)"
  host_line="$(grep -n -F -x "$host {" "$file" | cut -d: -f1 || true)"
  [[ "$marker_line" =~ ^[1-9][0-9]*$ && "$host_line" =~ ^[1-9][0-9]*$ && "$host_line" -eq $((marker_line + 1)) ]] || fail 'legacy id-dev marker and host must occur exactly once in order'
  total_lines="$(wc -l <"$file" | tr -d '[:space:]')"
  [[ "$total_lines" =~ ^[1-9][0-9]*$ && "$marker_line" -lt "$total_lines" ]] || fail 'legacy id-dev tail is invalid'
  trailing="$(tail -n +$((total_lines + 1)) "$file")"
  [[ -z "${trailing//[[:space:]]/}" ]] || fail 'legacy id-dev block must be the last block'
  printf '%s:%s' "$marker_line" "$total_lines"
}

manual_pre_tail_bounds() {
  local file="$1" host="$2"
  if grep -F -x -q '# AICRM_ID_DEV_BEGIN' "$file" || grep -F -x -q '# AICRM_ID_DEV_END' "$file"; then
    manual_tail_bounds "$file" "$host"
  else
    manual_legacy_tail_bounds "$file" "$host"
  fi
}

manual_tail_contains() {
  local file="$1" host="$2" needle="$3" bounds begin_line end_line
  bounds="$(manual_tail_bounds "$file" "$host")"
  begin_line="${bounds%%:*}"
  end_line="${bounds##*:}"
  sed -n "${begin_line},${end_line}p" "$file" | grep -F -q "$needle"
}

manual_pre_tail_contains() {
  local file="$1" host="$2" needle="$3" bounds begin_line end_line
  bounds="$(manual_pre_tail_bounds "$file" "$host")"
  begin_line="${bounds%%:*}"
  end_line="${bounds##*:}"
  sed -n "${begin_line},${end_line}p" "$file" | grep -F -q "$needle"
}

manual_validate_caddy_service() {
  local main_pid exec_start exec_path
  "$manual_systemctl_command" is-active --quiet "$manual_caddy_service"
  main_pid="$("$manual_systemctl_command" show --property=MainPID --value "$manual_caddy_service")"
  [[ "$main_pid" =~ ^[1-9][0-9]*$ ]] || fail 'manual Caddy service MainPID is invalid'
  exec_start="$("$manual_systemctl_command" show --property=ExecStart --value "$manual_caddy_service")"
  exec_path="$(grep -o 'path=[^ ;}]*' <<<"$exec_start" || true)"
  [[ "$exec_path" = "path=$manual_caddy_command" ]] || fail 'manual Caddy service ExecStart path does not match the declared executable'
}

manual_validate_caddy() {
  local caddy_file="$1" caddy_sha="$2" host="$3" old_port="$4" old_web="$5" tail_file="$6" new_port="$7" new_web="$8"
  [[ "$caddy_file" = /* && -f "$caddy_file" && ! -L "$caddy_file" ]] || fail 'manual Caddy file is invalid'
  [[ "$caddy_sha" =~ ^[a-f0-9]{64}$ ]] || fail 'manual Caddy SHA-256 is invalid'
  [[ "$(sha256_file "$caddy_file")" = "$caddy_sha" ]] || fail 'manual Caddy file SHA-256 drifted'
  [[ "$host" =~ ^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$ ]] || fail 'manual id-dev host is invalid'
  manual_pre_tail_bounds "$caddy_file" "$host" >/dev/null
  manual_pre_tail_contains "$caddy_file" "$host" "reverse_proxy 127.0.0.1:$old_port" || fail 'manual Caddy tail does not reference the current API port'
  manual_pre_tail_contains "$caddy_file" "$host" "root * $old_web" || fail 'manual Caddy tail does not reference the current web release'
  [[ "$tail_file" = /* && -f "$tail_file" && ! -L "$tail_file" ]] || fail 'staged id-dev Caddy tail is invalid'
  manual_tail_bounds "$tail_file" "$host" >/dev/null
  manual_tail_contains "$tail_file" "$host" "reverse_proxy 127.0.0.1:$new_port" || fail 'staged id-dev Caddy tail does not reference the new API port'
  manual_tail_contains "$tail_file" "$host" "root * $new_web" || fail 'staged id-dev Caddy tail does not reference the staged web release'
  manual_validate_caddy_service
}

manual_validate_post_switch_caddy() {
  local caddy_file="$1" caddy_sha="$2" host="$3" new_port="$4" new_web="$5"
  [[ "$caddy_file" = /* && -f "$caddy_file" && ! -L "$caddy_file" ]] || fail 'manual Caddy file is invalid'
  [[ "$caddy_sha" =~ ^[a-f0-9]{64}$ ]] || fail 'manual post-switch Caddy SHA-256 is invalid'
  [[ "$(sha256_file "$caddy_file")" = "$caddy_sha" ]] || fail 'manual post-switch Caddy file SHA-256 drifted'
  manual_tail_bounds "$caddy_file" "$host" >/dev/null
  manual_tail_contains "$caddy_file" "$host" "reverse_proxy 127.0.0.1:$new_port" || fail 'manual post-switch Caddy tail does not reference the new API port'
  manual_tail_contains "$caddy_file" "$host" "root * $new_web" || fail 'manual post-switch Caddy tail does not reference the staged web release'
  manual_validate_caddy_service
}

manual_switch_caddy_tail() {
  local caddy_file="$1" caddy_sha="$2" host="$3" tail_file="$4" caddy_command="$5" bounds begin_line prefix_hash tmp prefix_after
  manual_validate_caddy "$caddy_file" "$caddy_sha" "$host" "$manual_current_port" "$manual_current_web" "$tail_file" "$manual_new_port" "$manual_new_web"
  bounds="$(manual_pre_tail_bounds "$caddy_file" "$host")"
  begin_line="${bounds%%:*}"
  prefix_hash="$(head -n $((begin_line - 1)) "$caddy_file" | sha256sum | awk '{ print $1 }')"
  tmp="$(mktemp "${caddy_file}.next.XXXXXX")"
  head -n $((begin_line - 1)) "$caddy_file" >"$tmp"
  cat "$tail_file" >>"$tmp"
  prefix_after="$(head -n $((begin_line - 1)) "$tmp" | sha256sum | awk '{ print $1 }')"
  [[ "$prefix_after" = "$prefix_hash" ]] || fail 'protected Caddy prefix changed while staging id-dev tail'
  [[ "$(sha256_file "$tmp")" = "$manual_post_caddy_sha" ]] || fail 'staged post-switch Caddy file does not match its declared SHA-256'
  "$caddy_command" validate --config "$tmp" --adapter caddyfile
  chmod "$(file_mode "$caddy_file")" "$tmp"
  mv -f "$tmp" "$caddy_file"
  "$manual_systemctl_command" kill --kill-who=main --signal=USR1 "$manual_caddy_service"
}

manual_check_unused_port() {
  local port="$1" ss_command output
  ss_command="$(command -v ss 2>/dev/null || true)"
  [[ "$ss_command" = /* && -x "$ss_command" ]] || fail 'ss is required to verify the declared unused API port'
  output="$("$ss_command" -ltnH "sport = :$port")"
  [[ -z "$output" ]] || fail 'declared new API port is already listening'
}

manual_load() {
  manual_release_sha="$(require_manual_value AICRM_RELEASE_SHA)"
  manual_image="$(require_manual_value AICRM_IMAGE)"
  manual_image_id="$(require_manual_value AICRM_FINAL_MANUAL_IMAGE_ID)"
  manual_rollback_sha="$(require_manual_value AICRM_ROLLBACK_RELEASE_SHA)"
  manual_rollback_image="$(require_manual_value AICRM_ROLLBACK_IMAGE)"
  manual_rollback_image_id="$(require_manual_value AICRM_FINAL_MANUAL_ROLLBACK_IMAGE_ID)"
  manual_current_sha="$(require_manual_value AICRM_FINAL_MANUAL_CURRENT_RELEASE_SHA)"
  manual_current_api="$(require_manual_value AICRM_FINAL_MANUAL_CURRENT_API_CONTAINER)"
  manual_current_worker="$(require_manual_value AICRM_FINAL_MANUAL_CURRENT_WORKER_CONTAINER)"
  manual_current_port="$(require_manual_value AICRM_FINAL_MANUAL_CURRENT_API_PORT)"
  manual_network="$(require_manual_value AICRM_FINAL_MANUAL_NETWORK)"
  manual_postgres="$(require_manual_value AICRM_FINAL_MANUAL_POSTGRES_CONTAINER)"
  manual_new_api="$(require_manual_value AICRM_FINAL_MANUAL_NEW_API_CONTAINER)"
  manual_new_worker="$(require_manual_value AICRM_FINAL_MANUAL_NEW_WORKER_CONTAINER)"
  manual_new_port="$(require_manual_value AICRM_FINAL_MANUAL_NEW_API_PORT)"
  manual_current_web="$(require_manual_value AICRM_FINAL_MANUAL_CURRENT_WEB_RELEASE_DIR)"
  manual_new_web="$(require_manual_value AICRM_FINAL_MANUAL_WEB_RELEASE_DIR)"
  manual_caddy_file="$(require_manual_value AICRM_FINAL_MANUAL_CADDY_FILE)"
  manual_caddy_sha="$(require_manual_value AICRM_FINAL_MANUAL_CADDY_SHA256)"
  manual_post_caddy_sha="$(require_manual_value AICRM_FINAL_MANUAL_POST_SWITCH_CADDY_SHA256)"
  manual_caddy_host="$(require_manual_value AICRM_FINAL_MANUAL_CADDY_HOST)"
  manual_caddy_tail="$(require_manual_value AICRM_FINAL_MANUAL_CADDY_TAIL_FILE)"
  manual_caddy_command="$(require_manual_value AICRM_FINAL_MANUAL_CADDY_COMMAND)"
  manual_caddy_service="$(require_manual_value AICRM_FINAL_MANUAL_CADDY_SERVICE)"
  manual_systemctl_command="$(require_manual_value AICRM_FINAL_MANUAL_CADDY_SYSTEMCTL_COMMAND)"
  manual_generated_env="$(require_manual_value AICRM_GENERATED_ENV_FILE)"
  for pair in \
    "$manual_release_sha:release SHA" "$manual_rollback_sha:rollback SHA" "$manual_current_sha:current SHA"; do
    require_manual_sha "${pair%%:*}" "${pair#*:}"
  done
  [[ -z "$expected_sha" || "$manual_release_sha" = "$expected_sha" ]] || fail 'manual release SHA must equal expected SHA'
  [[ "$manual_current_sha" = "$manual_rollback_sha" && "$manual_current_sha" != "$manual_release_sha" ]] || fail 'manual current release must be the declared rollback release'
  require_manual_image_id "$manual_image_id" 'manual image ID'
  require_manual_image_id "$manual_rollback_image_id" 'manual rollback image ID'
  require_manual_name "$manual_network" 'manual network'
  require_manual_name "$manual_postgres" 'manual PostgreSQL container'
  [[ "$manual_network" = 'aicrm-prod_default' ]] || fail 'manual external PostgreSQL network must be aicrm-prod_default'
  [[ "$manual_postgres" = 'aicrm-prod-postgres-1' ]] || fail 'manual external PostgreSQL container must be aicrm-prod-postgres-1'
  require_manual_name "$manual_current_api" 'manual current API container'
  require_manual_name "$manual_current_worker" 'manual current worker container'
  require_manual_name "$manual_new_api" 'manual new API container'
  require_manual_name "$manual_new_worker" 'manual new worker container'
  [[ "$manual_current_api" = "aicrm-manual-api-${manual_current_sha:0:8}" && "$manual_current_worker" = "aicrm-manual-worker-${manual_current_sha:0:8}" ]] || fail 'manual current container names must bind the current SHA prefix'
  [[ "$manual_new_api" = "aicrm-manual-api-${manual_release_sha:0:8}" && "$manual_new_worker" = "aicrm-manual-worker-${manual_release_sha:0:8}" ]] || fail 'manual new container names must bind the release SHA prefix'
  [[ "$manual_current_api" != "$manual_new_api" && "$manual_current_worker" != "$manual_new_worker" ]] || fail 'manual new containers must differ from current containers'
  require_manual_port "$manual_current_port" 'manual current API port'
  require_manual_port "$manual_new_port" 'manual new API port'
  [[ "$manual_current_port" != "$manual_new_port" ]] || fail 'manual new API port must differ from the current API port'
  [[ "$manual_current_web" = /* && -d "$manual_current_web" && ! -L "$manual_current_web" && "$(basename -- "$manual_current_web")" = "$manual_current_sha" ]] || fail 'manual current web release is invalid'
  [[ "$manual_new_web" = /* && -d "$manual_new_web" && ! -L "$manual_new_web" && "$(basename -- "$manual_new_web")" = "$manual_release_sha" ]] || fail 'manual staged web release must be the exact SHA directory'
  [[ "$manual_generated_env" = /* && -f "$manual_generated_env" && ! -L "$manual_generated_env" ]] || fail 'manual generated environment is invalid'
  [[ "$manual_caddy_command" = /* && -f "$manual_caddy_command" && ! -L "$manual_caddy_command" && -x "$manual_caddy_command" ]] || fail 'manual Caddy command is invalid'
  [[ "$manual_caddy_service" = 'aicrm-edge.service' ]] || fail 'manual Caddy service must be aicrm-edge.service'
  [[ "$manual_systemctl_command" = /* && -f "$manual_systemctl_command" && ! -L "$manual_systemctl_command" && -x "$manual_systemctl_command" ]] || fail 'manual Caddy systemctl command is invalid'
}

manual_validate_current() {
  "$docker_command" network inspect "$manual_network" >/dev/null
  "$docker_command" inspect "$manual_postgres" >/dev/null
  "$docker_command" inspect "$manual_current_api" >/dev/null
  "$docker_command" inspect "$manual_current_worker" >/dev/null
  [[ "$("$docker_command" inspect --format '{{.State.Running}}' "$manual_postgres")" = true ]] || fail 'manual PostgreSQL container must be running'
  manual_container_network "$manual_postgres" "$manual_network"
  manual_container_network "$manual_current_api" "$manual_network"
  manual_container_network "$manual_current_worker" "$manual_network"
  manual_container_hardened "$manual_current_api"
  manual_container_hardened "$manual_current_worker"
  [[ "$("$docker_command" port "$manual_current_api" 8080/tcp)" = "127.0.0.1:$manual_current_port" ]] || fail 'manual current API port does not match the declared loopback port'
  manual_image_matches "$manual_rollback_image" "$manual_rollback_image_id" "$manual_rollback_sha" 'manual rollback'
  [[ "$("$docker_command" inspect --format '{{.Image}}' "$manual_current_api")" = "$manual_rollback_image_id" ]] || fail 'manual current API does not use the rollback image ID'
  [[ "$("$docker_command" inspect --format '{{.Image}}' "$manual_current_worker")" = "$manual_rollback_image_id" ]] || fail 'manual current worker does not use the rollback image ID'
}

manual_validate_release() {
  manual_image_matches "$manual_image" "$manual_image_id" "$manual_release_sha" 'manual release'
}

manual_expect_current_state() {
  local expected="$1"
  [[ "$("$docker_command" inspect --format '{{.State.Running}}' "$manual_current_api")" = "$expected" ]] || fail "manual current API must be $expected"
  [[ "$("$docker_command" inspect --format '{{.State.Running}}' "$manual_current_worker")" = "$expected" ]] || fail "manual current worker must be $expected"
}

manual_validate_pre_switch() {
  manual_validate_current
  manual_expect_current_state false
  manual_validate_caddy "$manual_caddy_file" "$manual_caddy_sha" "$manual_caddy_host" "$manual_current_port" "$manual_current_web" "$manual_caddy_tail" "$manual_new_port" "$manual_new_web"
}

manual_validate_post_switch() {
  manual_validate_current
  manual_expect_current_state false
  "$docker_command" inspect "$manual_new_api" >/dev/null
  "$docker_command" inspect "$manual_new_worker" >/dev/null
  [[ "$("$docker_command" inspect --format '{{.State.Running}}' "$manual_new_api")" = true ]] || fail 'manual new API is not running'
  [[ "$("$docker_command" inspect --format '{{.State.Running}}' "$manual_new_worker")" = true ]] || fail 'manual new worker is not running'
  manual_container_network "$manual_new_api" "$manual_network"
  manual_container_network "$manual_new_worker" "$manual_network"
  manual_container_hardened "$manual_new_api"
  manual_container_hardened "$manual_new_worker"
  [[ "$("$docker_command" inspect --format '{{.Image}}' "$manual_new_api")" = "$manual_image_id" ]] || fail 'manual new API does not use the release image ID'
  [[ "$("$docker_command" inspect --format '{{.Image}}' "$manual_new_worker")" = "$manual_image_id" ]] || fail 'manual new worker does not use the release image ID'
  [[ "$("$docker_command" port "$manual_new_api" 8080/tcp)" = "127.0.0.1:$manual_new_port" ]] || fail 'manual new API port does not match the declared loopback port'
  manual_validate_post_switch_caddy "$manual_caddy_file" "$manual_post_caddy_sha" "$manual_caddy_host" "$manual_new_port" "$manual_new_web"
}

manual_release_state() {
  local api_exists=0 worker_exists=0
  "$docker_command" inspect "$manual_new_api" >/dev/null 2>&1 && api_exists=1
  "$docker_command" inspect "$manual_new_worker" >/dev/null 2>&1 && worker_exists=1
  if [[ "$api_exists" = 0 && "$worker_exists" = 0 ]]; then
    manual_validate_current
  elif [[ "$api_exists" = 1 && "$worker_exists" = 1 ]]; then
    manual_validate_post_switch
  else
    fail 'manual runtime is in a half-switched state'
  fi
}

manual_export_container_environment() {
  local key value
  for key in AICRM_DATABASE_URL AICRM_ENV AICRM_IDENTITY_HMAC_KEY AICRM_RELEASE_SHA; do
    value="$(require_manual_value "$key")"
    export "$key=$value"
  done
}

manual_start_container() {
  local name="$1" role="$2"
  if [[ "$role" = api ]]; then
    "$docker_command" run -d --name "$name" --network "$manual_network" --restart unless-stopped \
      --read-only --tmpfs /tmp:size=64m,mode=1777 --security-opt no-new-privileges:true --cap-drop ALL --init \
      --env-file "$manual_generated_env" \
      -e AICRM_DATABASE_URL -e AICRM_ENV -e AICRM_IDENTITY_HMAC_KEY -e AICRM_RELEASE_SHA \
      -p "127.0.0.1:$manual_new_port:8080" "$manual_image" --role=api >/dev/null
  else
    "$docker_command" run -d --name "$name" --network "$manual_network" --restart unless-stopped \
      --read-only --tmpfs /tmp:size=64m,mode=1777 --security-opt no-new-privileges:true --cap-drop ALL --init \
      --env-file "$manual_generated_env" \
      -e AICRM_DATABASE_URL -e AICRM_ENV -e AICRM_IDENTITY_HMAC_KEY -e AICRM_RELEASE_SHA \
      "$manual_image" --role=worker >/dev/null
  fi
}

manual_start() {
  if "$docker_command" inspect "$manual_new_api" >/dev/null 2>&1 || "$docker_command" inspect "$manual_new_worker" >/dev/null 2>&1; then
    fail 'manual new container names already exist; refusing replay'
  fi
  manual_validate_pre_switch
  manual_check_unused_port "$manual_new_port"
  manual_export_container_environment
  manual_start_container "$manual_new_api" api
  manual_start_container "$manual_new_worker" worker
  [[ "$("$docker_command" inspect --format '{{.State.Running}}' "$manual_new_api")" = true ]] || fail 'manual new API is not running'
  [[ "$("$docker_command" inspect --format '{{.State.Running}}' "$manual_new_worker")" = true ]] || fail 'manual new worker is not running'
  curl --fail --silent --show-error --max-time 10 "http://127.0.0.1:$manual_new_port/healthz" >/dev/null
  manual_switch_caddy_tail "$manual_caddy_file" "$manual_caddy_sha" "$manual_caddy_host" "$manual_caddy_tail" "$manual_caddy_command"
}

if manual_mode; then
  manual_load
  case "$action" in
    config)
      manual_validate_current
      manual_expect_current_state true
      manual_validate_caddy "$manual_caddy_file" "$manual_caddy_sha" "$manual_caddy_host" "$manual_current_port" "$manual_current_web" "$manual_caddy_tail" "$manual_new_port" "$manual_new_web"
      ;;
    release) manual_validate_release; manual_release_state ;;
    stop)
      manual_validate_current
      manual_expect_current_state true
      "$docker_command" stop "$manual_current_api" "$manual_current_worker" >/dev/null
      ;;
    check)
      manual_validate_current
      manual_expect_current_state false
      ;;
    start)
      [[ "$services" = 'api,worker' && "$web" = 'api' ]] || fail 'only split api+worker with web=api may start'
      manual_validate_release
      manual_start
      ;;
    *) fail 'runtime action is required' ;;
  esac
  exit 0
fi

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
