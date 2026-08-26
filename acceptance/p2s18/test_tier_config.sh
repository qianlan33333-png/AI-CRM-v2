#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'p2-s18-acceptance: %s\n' "$1" >&2
  exit 1
}

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/../.." && pwd -P)"
test_directory="$(mktemp -d -t aicrm-v2-p2s18.XXXXXX)"
trap 'rm -rf "$test_directory"' EXIT HUP INT TERM

mode_of() {
  if stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    stat -c '%a' "$1"
  fi
}

env_value() {
  awk -F= -v wanted_key="$1" '$1 == wanted_key { print $2 }' "$2"
}

validate_environment_file() {
  local environment_file="$1"
  local queue_key
  local queue_count=0
  local queue_total=0
  local queue_value
  local worker_pool

  if grep -Eiq '(database_url|password|token|cookie|secret|wecom)' "$environment_file"; then
    return 1
  fi
  if grep -Eiq '(^|_)default(_|=)' "$environment_file"; then
    return 1
  fi
  for queue_key in CRITICAL EVENT OUTBOUND SYNC HEAVY AI; do
    queue_value="$(env_value "AICRM_RIVER_${queue_key}_MAX_WORKERS" "$environment_file")"
    [[ "$queue_value" =~ ^[1-9][0-9]*$ ]] || return 1
    queue_total=$((queue_total + queue_value))
    queue_count=$((queue_count + 1))
  done
  [[ "$(grep -Ec '^AICRM_RIVER_[A-Z_]+_MAX_WORKERS=' "$environment_file")" -eq "$queue_count" ]] || return 1
  worker_pool="$(env_value AICRM_WORKER_PGX_MAX_CONNS "$environment_file")"
  [[ "$worker_pool" =~ ^[1-9][0-9]*$ ]] || return 1
  (( worker_pool >= queue_total + 2 )) || return 1
}

validate_postgresql_file() {
  local postgresql_file="$1"
  [[ -f "$postgresql_file" && ! -L "$postgresql_file" ]] || return 1
  [[ "$(grep -Fxc "listen_addresses = '*'" "$postgresql_file")" -eq 1 ]] || return 1
}

validate_compose_file() {
  local compose_file="$1"
  [[ -f "$compose_file" && ! -L "$compose_file" ]] || return 1
  grep -Fq 'image: postgres:16.14-trixie@sha256:95206741a5b214807675e14165369d05b93a9cf692223b616d07cca227e74b0b' "$compose_file" || return 1
  grep -Fq 'profiles: [combined]' "$compose_file" || return 1
  grep -Fq 'command: ["--role=all"]' "$compose_file" || return 1
  [[ "$(grep -Fxc '    profiles: [split]' "$compose_file")" = '2' ]] || return 1
  grep -Fq 'command: ["--role=api"]' "$compose_file" || return 1
  grep -Fq 'command: ["--role=worker"]' "$compose_file" || return 1
  grep -Fq 'AICRM_ENV: ${AICRM_ENV:?AICRM_ENV is required}' "$compose_file" || return 1
  grep -Fq 'AICRM_IDENTITY_HMAC_KEY: ${AICRM_IDENTITY_HMAC_KEY:?AICRM_IDENTITY_HMAC_KEY is required}' "$compose_file" || return 1
  grep -Fq 'AICRM_RELEASE_SHA: ${AICRM_RELEASE_SHA:?AICRM_RELEASE_SHA is required}' "$compose_file" || return 1
  ! grep -Eiq '(redis|kafka|rabbitmq|nats|kubernetes)' "$compose_file"
}

for tier_name in s m l; do
  first_directory="$test_directory/$tier_name-first"
  second_directory="$test_directory/$tier_name-second"
  (
    cd "$repository_root"
    GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
      go run ./cmd/aicrm-config --tier="$tier_name" --output-dir="$first_directory" >/dev/null
    GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
      go run ./cmd/aicrm-config --tier="$tier_name" --output-dir="$second_directory" >/dev/null
  )
  cmp "$first_directory/aicrm.env" "$second_directory/aicrm.env" >/dev/null ||
    fail "$tier_name environment generation is not deterministic"
  cmp "$first_directory/postgresql.conf" "$second_directory/postgresql.conf" >/dev/null ||
    fail "$tier_name PostgreSQL generation is not deterministic"
  [[ "$(mode_of "$first_directory")" = '750' ]] || fail "$tier_name directory mode drifted"
  [[ "$(mode_of "$first_directory/aicrm.env")" = '640' ]] || fail "$tier_name environment mode drifted"
  [[ "$(mode_of "$first_directory/postgresql.conf")" = '640' ]] || fail "$tier_name PostgreSQL mode drifted"

  environment_file="$first_directory/aicrm.env"
  validate_environment_file "$environment_file" || fail "$tier_name generated environment failed its contract"
  validate_postgresql_file "$first_directory/postgresql.conf" ||
    fail "$tier_name PostgreSQL container-network listener contract drifted"
done

[[ "$(env_value COMPOSE_PROFILES "$test_directory/s-first/aicrm.env")" = 'combined' ]] || fail 'S profile must be combined'
[[ "$(env_value COMPOSE_PROFILES "$test_directory/m-first/aicrm.env")" = 'split' ]] || fail 'M profile must be split'
[[ "$(env_value COMPOSE_PROFILES "$test_directory/l-first/aicrm.env")" = 'split' ]] || fail 'L profile must be split'
[[ "$(env_value AICRM_WORKER_PGX_MAX_CONNS "$test_directory/s-first/aicrm.env")" = '9' ]] || fail 'S worker pool drifted'
[[ "$(env_value AICRM_WORKER_PGX_MAX_CONNS "$test_directory/m-first/aicrm.env")" = '18' ]] || fail 'M worker pool drifted'
[[ "$(env_value AICRM_WORKER_PGX_MAX_CONNS "$test_directory/l-first/aicrm.env")" = '30' ]] || fail 'L worker pool drifted'
[[ "$(env_value AICRM_SWAP_TARGET_MIB "$test_directory/s-first/aicrm.env")" = '4096' ]] || fail 'S swap target drifted'
[[ "$(env_value AICRM_SWAP_POLICY "$test_directory/s-first/aicrm.env")" = 'required' ]] || fail 'S swap policy drifted'
[[ "$(env_value AICRM_SWAP_TARGET_MIB "$test_directory/m-first/aicrm.env")" = '2048' ]] || fail 'M swap target drifted'
[[ "$(env_value AICRM_SWAP_POLICY "$test_directory/m-first/aicrm.env")" = 'recommended' ]] || fail 'M swap policy drifted'
[[ "$(env_value AICRM_SWAP_TARGET_MIB "$test_directory/l-first/aicrm.env")" = '0' ]] || fail 'L swap target drifted'
[[ "$(env_value AICRM_SWAP_POLICY "$test_directory/l-first/aicrm.env")" = 'optional' ]] || fail 'L swap policy drifted'

negative_environment="$test_directory/negative.env"
cp "$test_directory/s-first/aicrm.env" "$negative_environment"
printf '%s\n' 'AICRM_DATABASE_URL=forbidden' >>"$negative_environment"
if validate_environment_file "$negative_environment"; then fail 'credential-bearing generated environment was accepted'; fi
cp "$test_directory/s-first/aicrm.env" "$negative_environment"
sed -i.bak 's/AICRM_WORKER_PGX_MAX_CONNS=9/AICRM_WORKER_PGX_MAX_CONNS=8/' "$negative_environment"
rm -f "$negative_environment.bak"
if validate_environment_file "$negative_environment"; then fail 'undersized worker pool was accepted'; fi
cp "$test_directory/s-first/aicrm.env" "$negative_environment"
sed -i.bak '/^AICRM_RIVER_AI_MAX_WORKERS=/d' "$negative_environment"
rm -f "$negative_environment.bak"
if validate_environment_file "$negative_environment"; then fail 'missing fixed queue was accepted'; fi
cp "$test_directory/s-first/aicrm.env" "$negative_environment"
printf '%s\n' 'AICRM_RIVER_DEFAULT_MAX_WORKERS=1' >>"$negative_environment"
if validate_environment_file "$negative_environment"; then fail 'default queue was accepted'; fi

negative_postgresql="$test_directory/negative-postgresql.conf"
grep -Fv "listen_addresses = '*'" "$test_directory/s-first/postgresql.conf" >"$negative_postgresql"
if validate_postgresql_file "$negative_postgresql"; then
  fail 'PostgreSQL localhost-only default was accepted for Compose'
fi

compose_file="$repository_root/deploy/compose.yml"
validate_compose_file "$compose_file" || fail 'Compose failed its fixed topology contract'
negative_compose="$test_directory/negative-compose.yml"
cp "$compose_file" "$negative_compose"
printf '%s\n' '  redis:' '    image: redis:latest' >>"$negative_compose"
if validate_compose_file "$negative_compose"; then fail 'extra stateful Compose component was accepted'; fi

render_output="$test_directory/render-only.log"
(
  cd "$repository_root"
  scripts/staging_deploy.sh --tier=s --output-dir="$test_directory/rendered" >"$render_output"
)
grep -Fq 'deployment NOT EXECUTED' "$render_output" || fail 'render-only did not preserve the external gate'

fake_binary_directory="$test_directory/fake-bin"
mkdir -p "$fake_binary_directory"
docker_log="$test_directory/docker.log"
printf '%s\n' '#!/usr/bin/env bash' 'printf "%s\n" "$*" >>"$AICRM_DOCKER_LOG"' >"$fake_binary_directory/docker"
chmod 755 "$fake_binary_directory/docker"
printf '%s\n' '#!/usr/bin/env bash' 'for argument in "$@"; do case "$argument" in --file=*) snapshot_file="${argument#--file=}" ;; esac; done' ': >"$snapshot_file"' >"$fake_binary_directory/pg_dump"
printf '%s\n' '#!/usr/bin/env bash' 'if [[ -n "${AICRM_CURL_LOG:-}" ]]; then printf "%s\\n" "$*" >>"$AICRM_CURL_LOG"; fi' 'exit 0' >"$fake_binary_directory/curl"
restore_log="$test_directory/restore.log"
printf '%s\n' '#!/usr/bin/env bash' 'printf "%s\\n" "$*" >>"$AICRM_PG_RESTORE_LOG"' >"$fake_binary_directory/pg_restore"
real_go="$(command -v go)"
go_log="$test_directory/go.log"
printf '%s\n' '#!/usr/bin/env bash' 'if [[ "$1" = "env" && "$2" = "GOVERSION" ]]; then exec "$AICRM_REAL_GO" "$@"; fi' 'if [[ "$*" = *"run ./cmd/aicrm-config"* ]]; then exec "$AICRM_REAL_GO" "$@"; fi' 'printf "%s\n" "$*" >>"$AICRM_GO_LOG"' >"$fake_binary_directory/go"
chmod 755 "$fake_binary_directory/pg_dump" "$fake_binary_directory/curl" "$fake_binary_directory/go"
chmod 755 "$fake_binary_directory/pg_restore"

public_smoke="$test_directory/public-smoke.log"
(
  cd "$repository_root"
  AICRM_CURL_LOG="$test_directory/public-curl.log" \
  PATH="$fake_binary_directory:$PATH" \
    scripts/staging_smoke.sh --base-url='http://127.0.0.1:8080' >"$public_smoke" 2>&1
)
grep -Fq 'authenticated session and core read were NOT EXECUTED' "$public_smoke" ||
  fail 'public-only staging smoke did not report the missing session blocker'

restore_snapshot="$test_directory/staging-pre-migration.dump"
printf '%s\n' 'fake custom-format snapshot' >"$restore_snapshot"
restore_plan="$test_directory/restore-plan.log"
(
  cd "$repository_root"
  scripts/staging_restore_drill.sh --snapshot="$restore_snapshot" --render-only >"$restore_plan"
)
grep -Fq 'NOT EXECUTED' "$restore_plan" || fail 'restore drill render-only did not preserve the external gate'
[[ ! -e "$restore_log" ]] || fail 'restore drill render-only invoked pg_restore'

restore_apply="$test_directory/restore-apply.log"
(
  cd "$repository_root"
  AICRM_ALLOW_STAGING_RESTORE=1 \
  AICRM_DATABASE_URL='postgres://test-only:test-only@127.0.0.1:5432/aicrm?sslmode=disable' \
  AICRM_STAGING_SESSION_COOKIE='aicrm_session=test-only-session' \
  AICRM_CURL_LOG="$test_directory/restore-curl.log" \
  AICRM_PG_RESTORE_LOG="$restore_log" \
  PATH="$fake_binary_directory:$PATH" \
    scripts/staging_restore_drill.sh --snapshot="$restore_snapshot" --edge-base-url='https://staging.invalid' --apply >"$restore_apply"
)
grep -Fq -- '--exit-on-error --clean --if-exists --no-owner' "$restore_log" || fail 'restore drill apply did not execute pg_restore'
grep -Fq 'post-restore authenticated smoke completed' "$restore_apply" || fail 'restore drill apply did not report completion'
grep -Fq '/readyz' "$test_directory/restore-curl.log" || fail 'restore drill did not run readiness smoke'
grep -Fq '/api/v1/products' "$test_directory/restore-curl.log" || fail 'restore drill did not run authenticated core read smoke'

if (
  cd "$repository_root"
  AICRM_DOCKER_LOG="$docker_log" PATH="$fake_binary_directory:$PATH" \
    scripts/staging_deploy.sh --tier=m --output-dir="$test_directory/unauthorized" --apply >/dev/null 2>&1
); then
  fail 'unauthorized staging apply was accepted'
fi
[[ ! -e "$docker_log" ]] || fail 'unauthorized staging apply invoked Docker'

if (
  cd "$repository_root"
  AICRM_ALLOW_STAGING_DEPLOY=1 \
  AICRM_IMAGE='registry.invalid/aicrm:test-only' \
  AICRM_DATABASE_URL='postgres://test-only:test-only@127.0.0.1:5432/aicrm?sslmode=disable' \
  AICRM_POSTGRES_PASSWORD='test-only' \
  AICRM_IDENTITY_HMAC_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
  AICRM_RELEASE_SHA='0123456789abcdef0123456789abcdef01234567' \
  AICRM_ENV='staging' \
  AICRM_STAGING_PROVIDER_MODE='disabled' \
  AICRM_STAGING_SESSION_COOKIE='aicrm_session=test-only-session' \
  PATH="$fake_binary_directory:$PATH" \
    scripts/staging_deploy.sh --tier=m --output-dir="$test_directory/unpinned-image" --edge-base-url='https://staging.invalid' --apply >/dev/null 2>&1
); then
  fail 'unpinned staging image was accepted'
fi

if (
  cd "$repository_root"
  AICRM_ALLOW_STAGING_DEPLOY=1 \
  AICRM_IMAGE='registry.invalid/aicrm@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef' \
  AICRM_DATABASE_URL='postgres://test-only:test-only@127.0.0.1:5432/aicrm?sslmode=disable' \
  AICRM_POSTGRES_PASSWORD='test-only' \
  AICRM_IDENTITY_HMAC_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
  AICRM_RELEASE_SHA='0123456789abcdef0123456789abcdef01234567' \
  AICRM_ENV='staging' \
  AICRM_STAGING_PROVIDER_MODE='disabled' \
  PATH="$fake_binary_directory:$PATH" \
    scripts/staging_deploy.sh --tier=m --output-dir="$test_directory/missing-auth" --edge-base-url='https://staging.invalid' --apply >/dev/null 2>&1
); then
  fail 'staging apply without authenticated smoke session was accepted'
fi

(
  cd "$repository_root"
  AICRM_ALLOW_STAGING_DEPLOY=1 \
  AICRM_IMAGE='registry.invalid/aicrm@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef' \
  AICRM_DATABASE_URL='postgres://test-only:test-only@127.0.0.1:5432/aicrm?sslmode=disable' \
  AICRM_POSTGRES_PASSWORD='test-only' \
  AICRM_IDENTITY_HMAC_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
  AICRM_RELEASE_SHA='0123456789abcdef0123456789abcdef01234567' \
  AICRM_ENV='staging' \
  AICRM_STAGING_PROVIDER_MODE='disabled' \
  AICRM_STAGING_SESSION_COOKIE='aicrm_session=test-only-session' \
  AICRM_DOCKER_LOG="$docker_log" \
  AICRM_CURL_LOG="$test_directory/curl.log" \
  AICRM_GO_LOG="$go_log" \
  AICRM_REAL_GO="$real_go" \
  PATH="$fake_binary_directory:$PATH" \
    scripts/staging_deploy.sh --tier=m --output-dir="$test_directory/authorized" --edge-base-url='https://staging.invalid' --apply >/dev/null
)
[[ "$(wc -l <"$docker_log" | tr -d ' ')" = '4' ]] || fail 'authorized staging apply did not execute four Compose checks/actions'
grep -Fq 'compose version' "$docker_log" || fail 'Compose version check was not executed'
grep -Fq 'config --quiet' "$docker_log" || fail 'Compose config check was not executed'
grep -Fq 'up -d --wait postgres' "$docker_log" || fail 'PostgreSQL was not started before migrations'
[[ "$(grep -Fc 'up -d --wait' "$docker_log")" = '2' ]] || fail 'Compose application did not resume after migrations'
grep -Fq 'tool -modfile=tools/go.mod goose -dir migrations up' "$go_log" || fail 'Goose migration was not executed'
grep -Fq 'run ./cmd/aicrm-river-migrate --direction=up' "$go_log" || fail 'River migration was not executed'
compgen -G "$test_directory/authorized/staging-pre-migration.*.dump" >/dev/null || fail 'pre-migration snapshot was not created'
grep -Fq '/login' "$test_directory/curl.log" || fail 'login entry smoke was not executed'
grep -Fq '/healthz' "$test_directory/curl.log" || fail 'API health smoke was not executed'
grep -Fq '/readyz' "$test_directory/curl.log" || fail 'worker queue readiness smoke was not executed'
grep -Fq '/api/v1/auth/session' "$test_directory/curl.log" || fail 'authenticated session smoke was not executed'
grep -Fq '/api/v1/products' "$test_directory/curl.log" || fail 'authenticated core read smoke was not executed'

printf 'p2-s18-acceptance: PASS (render-only, guarded smoke and restore drill; real Staging deployment and restore drill NOT EXECUTED)\n'
