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

for role in api worker all; do
  "$binary" --role="$role" >"$test_root/$role.stdout" 2>"$test_root/$role.stderr" &
  process_id=$!
  sleep 0.2
  kill -0 "$process_id" 2>/dev/null || {
    echo "$role process exited before cancellation" >&2
    wait "$process_id" || true
    exit 1
  }
  if [[ "$role" == api ]]; then
    kill -INT "$process_id"
  else
    kill -TERM "$process_id"
  fi
  wait_for_signal_exit "$role" "$process_id"
done

echo "p0-s01-process-blackbox: PASS"
