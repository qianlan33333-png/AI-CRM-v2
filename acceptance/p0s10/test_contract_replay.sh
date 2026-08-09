#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -P "$script_dir/../.." && pwd)"
go_bin="${GO:-go}"
[[ "$go_bin" == /* ]] || go_bin="$(type -P "$go_bin" || true)"
[[ -x "$go_bin" ]] || { echo "contract-replay-tests: Go executable required" >&2; exit 1; }
for path in \
  tools/contract-replay/main.go \
  tools/contract-replay/main_test.go \
  tools/contract-replay/testdata/empty.v1.json; do
  [[ -f "$repo_root/$path" && ! -L "$repo_root/$path" ]] || {
    echo "contract-replay-tests: regular file required: $path" >&2
    exit 1
  }
done

test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-contract-replay.XXXXXX")"
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/tools/contract-replay/testdata" "$test_root/bin"
cp "$repo_root/tools/contract-replay/main.go" "$test_root/tools/contract-replay/main.go"
cp "$repo_root/tools/contract-replay/main_test.go" "$test_root/tools/contract-replay/main_test.go"
cp "$repo_root/tools/contract-replay/testdata/empty.v1.json" \
  "$test_root/tools/contract-replay/testdata/empty.v1.json"
printf '%s\n' 'module example.com/contract-replay-test' '' 'go 1.26.5' >"$test_root/tools/go.mod"

go_env=(env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off)
(cd / && "${go_env[@]}" "$go_bin" -C "$test_root/tools" test -race -timeout=15s ./contract-replay)
(cd / && "${go_env[@]}" "$go_bin" -C "$test_root/tools" build \
  -o "$test_root/bin/contract-replay" ./contract-replay)

runner="$test_root/bin/contract-replay"
expected='contract-replay: PASS (0 cases; validation only)'
[[ "$(cd / && "$runner" "$test_root/tools/contract-replay/testdata/empty.v1.json")" = "$expected" ]] || {
  echo "contract-replay-tests: canonical empty manifest was rejected" >&2
  exit 1
}

reject() {
  local name="$1" input="$2" expected_error="$3" path="$test_root/$1.json" output status
  printf '%s' "$input" >"$path"
  set +e
  output="$(cd / && "$runner" "$path" 2>&1)"
  status=$?
  set -e
  [[ "$status" -ne 0 && "$output" == *"$expected_error"* ]] || {
    echo "contract-replay-tests: accepted or misdiagnosed $name: $output" >&2
    exit 1
  }
}

reject malformed '{' 'decode manifest'
reject trailing '{"version":1,"cases":[]} {}' 'exactly one JSON value'
reject unknown '{"version":1,"cases":[],"extra":true}' 'unknown field'
reject duplicate-field '{"version":1,"version":1,"cases":[]}' 'duplicate field'
reject version '{"version":2,"cases":[]}' 'version must be 1'
reject missing-cases '{"version":1}' 'cases must be an array'
reject null-cases '{"version":1,"cases":null}' 'cases must be an array'
reject nonempty '{"version":1,"cases":[{"id":"one"}]}' 'execution adapter is not implemented'
reject duplicate '{"version":1,"cases":[{"id":"same"},{"id":"same"}]}' 'execution adapter is not implemented'

ln -s "$test_root/tools/contract-replay/testdata/empty.v1.json" "$test_root/symlink.json"
set +e
symlink_output="$(cd / && "$runner" "$test_root/symlink.json" 2>&1)"
symlink_status=$?
set -e
[[ "$symlink_status" -ne 0 && "$symlink_output" == *'manifest must be a regular file'* ]] || exit 1
echo "contract-replay-tests: PASS"
