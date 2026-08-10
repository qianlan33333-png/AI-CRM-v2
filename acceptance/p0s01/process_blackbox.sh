#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
[[ -f "$repo_root/go.mod" &&
  -f "$repo_root/internal/platform/runtime/contract.go" ]] || {
  echo "p0-s01-process: invalid repository root: $repo_root" >&2
  exit 1
}
cd "$repo_root"
test_root="$(mktemp -d -t aicrm-v2-p0-s01.XXXXXX)"
test_root_owner_pid="${BASHPID:-$$}"
test_root_owner_subshell="${BASH_SUBSHELL:-0}"
cleanup_test_root() {
  [[ "${BASHPID:-$$}" == "$test_root_owner_pid" &&
    "${BASH_SUBSHELL:-0}" == "$test_root_owner_subshell" ]] || return 0
  rm -rf -- "$test_root"
}
trap cleanup_test_root EXIT
binary="$test_root/aicrm"

startup_environment=(
  AICRM_DATABASE_URL
  AICRM_HTTP_LISTEN_ADDRESS
  AICRM_API_PGX_MAX_CONNS
  AICRM_WORKER_PGX_MAX_CONNS
  AICRM_RIVER_CRITICAL_MAX_WORKERS
  AICRM_RIVER_EVENT_MAX_WORKERS
  AICRM_RIVER_OUTBOUND_MAX_WORKERS
  AICRM_RIVER_SYNC_MAX_WORKERS
  AICRM_RIVER_HEAVY_MAX_WORKERS
  AICRM_RIVER_AI_MAX_WORKERS
)
for config_key in "${startup_environment[@]}"; do
  unset "$config_key"
done

go build -trimpath -o "$binary" ./cmd/aicrm

expect_exit() {
  local want="$1"
  shift
  set +e
  "$binary" "$@" >"$test_root/stdout" 2>"$test_root/stderr"
  local got=$?
  set -e
  [[ "$got" -eq "$want" ]] || {
    echo "exit code for [$*] = $got, want $want" >&2
    exit 1
  }
}

expect_exit 0 --help
grep -Fqx 'Usage: aicrm --role=<api|worker|all>' "$test_root/stdout"
[[ ! -s "$test_root/stderr" ]]

expect_exit 2
grep -Fqx 'aicrm: --role is required' "$test_root/stderr"
grep -Fqx 'Usage: aicrm --role=<api|worker|all>' "$test_root/stderr"

for bad_role in unknown API api,worker; do
  expect_exit 2 --role="$bad_role"
  [[ "$(sed -n '1p' "$test_root/stderr")" == \
    'aicrm: --role must be one of api, worker, all' ]]
  grep -Fqx 'Usage: aicrm --role=<api|worker|all>' "$test_root/stderr"
  ! grep -Fq -- "$bad_role" "$test_root/stderr"
done

wait_for_signal_exit() {
  local role="$1"
  local process_id="$2"
  local timeout_ticks=20
  local ticks=0
  local timed_out=0
  local exit_code

  while kill -0 "$process_id" 2>/dev/null; do
    if [[ "$ticks" -ge "$timeout_ticks" ]]; then
      timed_out=1
      kill -KILL "$process_id" 2>/dev/null || true
      break
    fi
    sleep 0.1
    ticks=$((ticks + 1))
  done

  set +e
  wait "$process_id"
  exit_code=$?
  set -e
  [[ "$timed_out" -eq 0 ]] || {
    echo "$role process did not stop within 2 seconds" >&2
    return 1
  }
  [[ "$exit_code" -eq 0 ]] || {
    echo "$role signal exit code = $exit_code, want 0" >&2
    return 1
  }
}

for bad_args in '--role=api --role=worker' 'api' '--debug'; do
  read -r -a args <<<"$bad_args"
  expect_exit 2 "${args[@]}"
  [[ "$(sed -n '1p' "$test_root/stderr")" == 'aicrm: invalid arguments' ]]
  grep -Fqx 'Usage: aicrm --role=<api|worker|all>' "$test_root/stderr"
done

expect_exit 1 --role=all
for field_name in database.url api.listen_address api.pool_max_conns worker.pool_max_conns \
  worker.queues.critical worker.queues.event worker.queues.outbound \
  worker.queues.sync worker.queues.heavy worker.queues.ai; do
  grep -Fq "$field_name" "$test_root/stderr"
done
! grep -Fq 'Usage:' "$test_root/stderr"

export AICRM_DATABASE_URL='not-a-url-database-password-sentinel'
export AICRM_HTTP_LISTEN_ADDRESS='127.0.0.1:8080'
export AICRM_API_PGX_MAX_CONNS='not-a-number'
export AICRM_WORKER_PGX_MAX_CONNS='0'
export AICRM_RIVER_CRITICAL_MAX_WORKERS='2'
export AICRM_RIVER_EVENT_MAX_WORKERS='1'
export AICRM_RIVER_OUTBOUND_MAX_WORKERS='1'
export AICRM_RIVER_SYNC_MAX_WORKERS='1'
export AICRM_RIVER_HEAVY_MAX_WORKERS='1'
export AICRM_RIVER_AI_MAX_WORKERS='1'
expect_exit 1 --role=all
grep -Fq 'database.url must be a valid postgres URL' "$test_root/stderr"
grep -Fq 'api.pool_max_conns must be a positive integer' "$test_root/stderr"
grep -Fq 'worker.pool_max_conns must be a positive integer' "$test_root/stderr"
! grep -Fq 'database-password-sentinel' "$test_root/stderr"

export AICRM_DATABASE_URL='postgres://aicrm:secret@127.0.0.1:5432/aicrm?sslmode=disable'
export AICRM_API_PGX_MAX_CONNS='10'
export AICRM_WORKER_PGX_MAX_CONNS='9'

unset AICRM_WORKER_PGX_MAX_CONNS AICRM_RIVER_CRITICAL_MAX_WORKERS \
  AICRM_RIVER_EVENT_MAX_WORKERS AICRM_RIVER_OUTBOUND_MAX_WORKERS \
  AICRM_RIVER_SYNC_MAX_WORKERS AICRM_RIVER_HEAVY_MAX_WORKERS AICRM_RIVER_AI_MAX_WORKERS
"$binary" --role=api >"$test_root/api.stdout" 2>"$test_root/api.stderr" &
process_id=$!
sleep 0.2
kill -0 "$process_id" 2>/dev/null || {
  echo "api process exited before cancellation" >&2
  wait "$process_id" || true
  exit 1
}
kill -INT "$process_id"
wait_for_signal_exit api "$process_id"

echo "p0-s01-process-blackbox: PASS"
