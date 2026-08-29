#!/usr/bin/env bash
set -euo pipefail
fail() { printf 'test-final-v1-domain-migration-runtime: %s\n' "$1" >&2; exit 1; }
root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
runtime="$root/scripts/final_v1_domain_migration_runtime.sh"
fixture="$(mktemp -d -t aicrm-final-runtime.XXXXXX)"
trap 'rm -rf -- "$fixture"' EXIT
sha='0123456789abcdef0123456789abcdef01234567'
rollback='abcdef0123456789abcdef0123456789abcdef01'
env_file="$fixture/runtime.env"; log="$fixture/docker.log"
cat >"$env_file" <<EOF
AICRM_RELEASE_SHA=$sha
AICRM_IMAGE=registry.example/aicrm@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
AICRM_ROLLBACK_RELEASE_SHA=$rollback
AICRM_ROLLBACK_IMAGE=registry.example/aicrm@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789
EOF
chmod 600 "$env_file"
cat >"$fixture/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${AICRM_FINAL_RUNTIME_TEST_LOG:?}"
printf 'env-image=%s\n' "${AICRM_IMAGE:-}" >>"${AICRM_FINAL_RUNTIME_TEST_LOG:?}"
printf 'env-project=%s\n' "${COMPOSE_PROJECT_NAME:-}" >>"${AICRM_FINAL_RUNTIME_TEST_LOG:?}"
if [[ "$1" = image && "$2" = inspect ]]; then
  case "$*" in *'@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789'*) printf '%s\n' "${AICRM_FINAL_RUNTIME_TEST_ROLLBACK_SHA:?}" ;; *) printf '%s\n' "${AICRM_FINAL_RUNTIME_TEST_SHA:?}" ;; esac
fi
EOF
chmod 700 "$fixture/docker"
run_runtime() {
  PATH="$fixture:$PATH" AICRM_FINAL_RUNTIME_TEST_LOG="$log" AICRM_FINAL_RUNTIME_TEST_SHA="$sha" AICRM_FINAL_RUNTIME_TEST_ROLLBACK_SHA="$rollback" "$runtime" "$@"
}
AICRM_IMAGE=parent-must-not-win COMPOSE_PROJECT_NAME=parent-must-not-win run_runtime --check=compose-config --runtime-env-file="$env_file"
run_runtime --check=release --expected-sha="$sha" --runtime-env-file="$env_file"
run_runtime --stop=app,api,worker --runtime-env-file="$env_file"
run_runtime --check=stopped --services=app,api,worker --runtime-env-file="$env_file"
run_runtime --start=api,worker --web=api --runtime-env-file="$env_file"
grep -Fq 'compose --env-file' "$log" || fail 'compose was not used'
grep -Fq 'config --quiet' "$log" || fail 'compose configuration was not checked'
grep -Fq -- "-f $root/deploy/compose.yml" "$log" || fail 'runtime helper did not validate the repository compose file'
if grep -Fq 'env-image=parent-must-not-win' "$log"; then fail 'parent AICRM_IMAGE overrode the env file'; fi
if grep -Fq 'env-project=parent-must-not-win' "$log"; then fail 'parent Compose project overrode the env file'; fi
grep -Fq 'stop app api worker' "$log" || fail 'services were not stopped'
grep -Fq -- '--profile split up -d --wait api worker' "$log" || fail 'split runtime did not start with wait'
[[ "$(grep -c '^pull ' "$log")" = 2 ]] || fail 'release images were not both verified'
printf 'test-final-v1-domain-migration-runtime: PASS\n'
