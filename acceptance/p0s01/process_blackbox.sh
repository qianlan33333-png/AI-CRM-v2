#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
test_root="$(mktemp -d -t aicrm-v2-p0-s01.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT
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
  timeout_marker="$test_root/$role.timeout"
  (
    sleep 2
    if kill -0 "$process_id" 2>/dev/null; then
      touch "$timeout_marker"
      kill -KILL "$process_id" 2>/dev/null || true
    fi
  ) &
  watchdog_id=$!
  set +e
  wait "$process_id"
  exit_code=$?
  kill "$watchdog_id" 2>/dev/null
  wait "$watchdog_id" 2>/dev/null
  set -e
  [[ ! -f "$timeout_marker" ]] || {
    echo "$role process did not stop within 2 seconds" >&2
    exit 1
  }
  [[ "$exit_code" -eq 0 ]] || {
    echo "$role signal exit code = $exit_code, want 0" >&2
    exit 1
  }
done

echo "p0-s01-process-blackbox: PASS"
