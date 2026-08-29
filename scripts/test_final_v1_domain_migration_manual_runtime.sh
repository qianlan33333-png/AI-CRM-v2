#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'test-final-v1-domain-migration-manual-runtime: %s\n' "$1" >&2; exit 1; }
root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
runtime="$root/scripts/final_v1_domain_migration_runtime.sh"
fixture="$(mktemp -d -t aicrm-final-manual-runtime.XXXXXX)"
trap 'rm -rf -- "$fixture"' EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

new_sha='0123456789abcdef0123456789abcdef01234567'
old_sha='abcdef0123456789abcdef0123456789abcdef01'
new_id='sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
old_id='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
network='aicrm-prod_default'
old_api="aicrm-manual-api-${old_sha:0:8}"
old_worker="aicrm-manual-worker-${old_sha:0:8}"
new_api="aicrm-manual-api-${new_sha:0:8}"
new_worker="aicrm-manual-worker-${new_sha:0:8}"
old_web="$fixture/$old_sha"; new_web="$fixture/$new_sha"
mkdir -p "$old_web" "$new_web"
printf 'old web\n' >"$old_web/index.html"
printf 'new web\n' >"$new_web/index.html"
generated="$fixture/generated.env"
printf 'AICRM_WECOM_OUTBOUND_ENABLED=false\n' >"$generated"
chmod 600 "$generated"

caddy="$fixture/Caddyfile"; tail_file="$fixture/id-dev.tail"; caddy_log="$fixture/caddy.log"
cat >"$caddy" <<EOF
aa.youcangogogo.com {
\treverse_proxy 127.0.0.1:8080
}
# V2 independent manual-test login. Existing aa site is unchanged.
id-dev.example.test {
\treverse_proxy 127.0.0.1:18123
\troot * $old_web
}
EOF
cat >"$tail_file" <<EOF
# AICRM_ID_DEV_BEGIN
id-dev.example.test {
\treverse_proxy 127.0.0.1:18124
\troot * $new_web
}
# AICRM_ID_DEV_END
EOF
prefix_before="$(head -n 3 "$caddy" | sha256sum | awk '{print $1}')"
post_caddy="$fixture/post-switch.Caddyfile"
{
  head -n 3 "$caddy"
  cat "$tail_file"
} >"$post_caddy"

state="$fixture/state"; printf '%s=true\n%s=true\n%s=true\n' aicrm-prod-postgres-1 "$old_api" "$old_worker" >"$state"
docker_log="$fixture/docker.log"
cat >"$fixture/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${AICRM_FINAL_MANUAL_TEST_DOCKER_LOG:?}"
state_value() { awk -F= -v k="$1" '$1 == k {print $2}' "${AICRM_FINAL_MANUAL_TEST_STATE:?}"; }
set_state() { sed -i.bak "/^$1=/d" "${AICRM_FINAL_MANUAL_TEST_STATE:?}"; rm -f "${AICRM_FINAL_MANUAL_TEST_STATE:?}.bak"; printf '%s=%s\n' "$1" "$2" >>"${AICRM_FINAL_MANUAL_TEST_STATE:?}"; }
new_sha="${AICRM_FINAL_MANUAL_TEST_NEW_SHA:?}"; old_sha="${AICRM_FINAL_MANUAL_TEST_OLD_SHA:?}"
new_id="${AICRM_FINAL_MANUAL_TEST_NEW_ID:?}"; old_id="${AICRM_FINAL_MANUAL_TEST_OLD_ID:?}"
old_api="aicrm-manual-api-${old_sha:0:8}"; old_worker="aicrm-manual-worker-${old_sha:0:8}"
case "$1" in
  network) [[ "$2" = inspect && "$3" = aicrm-prod_default ]] ;;
  image)
    [[ "$2" = inspect ]]
    if [[ "$*" = *'{{.Id}}'* ]]; then
      [[ "$*" = *new-local-image* ]] && printf '%s\n' "$new_id" || printf '%s\n' "$old_id"
    else
      [[ "$*" = *new-local-image* ]] && printf '%s\n' "$new_sha" || printf '%s\n' "$old_sha"
    fi
    ;;
  port)
    [[ "$3" = 8080/tcp ]] || exit 90
    if [[ "$2" = "$old_api" ]]; then printf '127.0.0.1:18123\n'; else printf '127.0.0.1:18124\n'; fi
    ;;
  inspect)
    format=''; container=''
    if [[ "${2:-}" = --format ]]; then format="$3"; container="$4"; else container="$2"; fi
    current="$(state_value "$container")"
    [[ -n "$current" ]] || exit 1
    if [[ -z "$format" ]]; then exit 0; fi
    case "$format" in
      '{{.HostConfig.ReadonlyRootfs}}|'*) printf 'true|true|unless-stopped|["ALL"]|["no-new-privileges:true"]|{"/tmp":"size=64m,mode=1777"}\n' ;;
      '{{range $name, $_ := .NetworkSettings.Networks}}'* ) printf 'aicrm-prod_default\n' ;;
      '{{.Image}}')
        if [[ "$container" = "$old_api" || "$container" = "$old_worker" ]]; then printf '%s\n' "$old_id"
        elif [[ -f "${AICRM_FINAL_MANUAL_TEST_TAMPER_NEW_CONTAINER:?}" ]]; then printf 'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n'
        else printf '%s\n' "$new_id"; fi
        ;;
      '{{.State.Running}}') printf '%s\n' "$current" ;;
      *) exit 91 ;;
    esac
    ;;
  stop)
    set_state "$2" false
    set_state "$3" false
    ;;
  run)
    name=''
    shift
    while [[ "$#" -gt 0 ]]; do
      if [[ "$1" = --name ]]; then name="$2"; shift 2; continue; fi
      shift
    done
    [[ -n "$name" ]] || exit 92
    set_state "$name" true
    printf 'container-%s\n' "$name"
    ;;
  *) exit 93 ;;
esac
EOF
chmod 700 "$fixture/docker"
cat >"$fixture/ss" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF
chmod 700 "$fixture/ss"
cat >"$fixture/caddy" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${AICRM_FINAL_MANUAL_TEST_CADDY_LOG:?}"
EOF
chmod 700 "$fixture/caddy"
cat >"$fixture/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" = *'http://127.0.0.1:18124/healthz'* ]]
EOF
chmod 700 "$fixture/curl"

env_file="$fixture/runtime.env"
cat >"$env_file" <<EOF
AICRM_FINAL_RUNTIME_MODE=external-postgres-manual
AICRM_RELEASE_SHA=$new_sha
AICRM_IMAGE=new-local-image:$new_sha
AICRM_FINAL_MANUAL_IMAGE_ID=$new_id
AICRM_ROLLBACK_RELEASE_SHA=$old_sha
AICRM_ROLLBACK_IMAGE=old-local-image:$old_sha
AICRM_FINAL_MANUAL_ROLLBACK_IMAGE_ID=$old_id
AICRM_FINAL_MANUAL_CURRENT_RELEASE_SHA=$old_sha
AICRM_FINAL_MANUAL_CURRENT_API_CONTAINER=$old_api
AICRM_FINAL_MANUAL_CURRENT_WORKER_CONTAINER=$old_worker
AICRM_FINAL_MANUAL_CURRENT_API_PORT=18123
AICRM_FINAL_MANUAL_NETWORK=$network
AICRM_FINAL_MANUAL_POSTGRES_CONTAINER=aicrm-prod-postgres-1
AICRM_FINAL_MANUAL_NEW_API_CONTAINER=$new_api
AICRM_FINAL_MANUAL_NEW_WORKER_CONTAINER=$new_worker
AICRM_FINAL_MANUAL_NEW_API_PORT=18124
AICRM_FINAL_MANUAL_CURRENT_WEB_RELEASE_DIR=$old_web
AICRM_FINAL_MANUAL_WEB_RELEASE_DIR=$new_web
AICRM_FINAL_MANUAL_CADDY_FILE=$caddy
AICRM_FINAL_MANUAL_CADDY_SHA256=$(sha256_file "$caddy")
AICRM_FINAL_MANUAL_POST_SWITCH_CADDY_SHA256=$(sha256_file "$post_caddy")
AICRM_FINAL_MANUAL_CADDY_HOST=id-dev.example.test
AICRM_FINAL_MANUAL_CADDY_TAIL_FILE=$tail_file
AICRM_FINAL_MANUAL_CADDY_COMMAND=$fixture/caddy
AICRM_GENERATED_ENV_FILE=$generated
AICRM_DATABASE_URL=postgres://target-not-printed
AICRM_ENV=production
AICRM_IDENTITY_HMAC_KEY=identity-secret-not-printed
EOF
chmod 600 "$env_file"

run_runtime() {
  PATH="$fixture:$PATH" \
    AICRM_FINAL_MANUAL_TEST_DOCKER_LOG="$docker_log" \
    AICRM_FINAL_MANUAL_TEST_CADDY_LOG="$caddy_log" \
    AICRM_FINAL_MANUAL_TEST_STATE="$state" \
    AICRM_FINAL_MANUAL_TEST_NEW_SHA="$new_sha" \
    AICRM_FINAL_MANUAL_TEST_OLD_SHA="$old_sha" \
    AICRM_FINAL_MANUAL_TEST_NEW_ID="$new_id" \
    AICRM_FINAL_MANUAL_TEST_OLD_ID="$old_id" \
    AICRM_FINAL_MANUAL_TEST_TAMPER_NEW_CONTAINER="$fixture/tamper-new-container" \
    "$runtime" "$@"
}

run_runtime --check=compose-config --runtime-env-file="$env_file"
run_runtime --check=release --expected-sha="$new_sha" --runtime-env-file="$env_file"
run_runtime --stop=app,api,worker --runtime-env-file="$env_file"
run_runtime --check=stopped --services=app,api,worker --runtime-env-file="$env_file"
run_runtime --start=api,worker --web=api --runtime-env-file="$env_file"

! grep -Fq 'compose ' "$docker_log" || fail 'manual mode invoked Compose'
grep -Fq "network inspect $network" "$docker_log" || fail 'external PostgreSQL network was not validated'
grep -Fq "stop $old_api $old_worker" "$docker_log" || fail 'only the declared API and worker were stopped'
! grep -Fq 'stop aicrm-prod-postgres-1' "$docker_log" || fail 'external PostgreSQL container was stopped'
[[ "$(grep -c '^run ' "$docker_log")" = 2 ]] || fail 'manual mode did not create exactly API and worker containers'
grep '^run ' "$docker_log" | grep -Fq -- "--name $new_api" || fail 'new API name was not SHA-bound'
grep '^run ' "$docker_log" | grep -Fq -- '--read-only --tmpfs /tmp:size=64m,mode=1777 --security-opt no-new-privileges:true --cap-drop ALL --init' || fail 'new containers were not hardened'
grep '^run ' "$docker_log" | grep -Fq -- "-p 127.0.0.1:18124:8080" || fail 'new API did not bind only the declared loopback port'
grep '^run ' "$docker_log" | grep -Fq -- "--env-file $generated" || fail 'restricted generated environment was not used'
[[ "$(awk -F= -v k="$old_api" '$1 == k {print $2}' "$state")" = false ]] || fail 'old API was not preserved stopped'
[[ "$(awk -F= -v k="$old_worker" '$1 == k {print $2}' "$state")" = false ]] || fail 'old worker was not preserved stopped'
[[ "$(head -n 3 "$caddy" | sha256sum | awk '{print $1}')" = "$prefix_before" ]] || fail 'protected aa prefix changed'
grep -Fq 'reverse_proxy 127.0.0.1:18124' "$caddy" || fail 'id-dev tail did not switch to new API'
grep -Fq "root * $new_web" "$caddy" || fail 'id-dev tail did not switch to staged exact-SHA web'
[[ "$(grep -c '^validate ' "$caddy_log")" = 1 && "$(grep -c '^reload ' "$caddy_log")" = 1 ]] || fail 'Caddy tail was not validated and gracefully reloaded'
run_runtime --check=release --expected-sha="$new_sha" --runtime-env-file="$env_file"

before_runs="$(grep -c '^run ' "$docker_log")"
printf 'drift\n' >>"$caddy"
if run_runtime --check=release --expected-sha="$new_sha" --runtime-env-file="$env_file" >"$fixture/caddy-drift.log" 2>&1; then fail 'new Caddy drift was accepted'; fi
grep -Fq 'manual post-switch Caddy file SHA-256 drifted' "$fixture/caddy-drift.log" || fail 'new Caddy drift rejection changed'
cp "$post_caddy" "$caddy"
touch "$fixture/tamper-new-container"
if run_runtime --check=release --expected-sha="$new_sha" --runtime-env-file="$env_file" >"$fixture/container-drift.log" 2>&1; then fail 'new container drift was accepted'; fi
grep -Fq 'manual new API does not use the release image ID' "$fixture/container-drift.log" || fail 'new container drift rejection changed'
rm -f "$fixture/tamper-new-container"
if run_runtime --start=api,worker --web=api --runtime-env-file="$env_file" >"$fixture/replay.log" 2>&1; then fail 'post-switch start was accepted'; fi
[[ "$(grep -c '^run ' "$docker_log")" = "$before_runs" ]] || fail 'replay rejection started another container'
grep -Fq 'manual new container names already exist; refusing replay' "$fixture/replay.log" || fail 'post-switch start rejection was not explicit'

printf 'test-final-v1-domain-migration-manual-runtime: PASS\n'
