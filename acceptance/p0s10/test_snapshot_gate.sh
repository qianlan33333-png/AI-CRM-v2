#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -P "$script_dir/../.." && pwd)"
go_bin="${GO:-go}"
[[ "$go_bin" == /* ]] || go_bin="$(type -P "$go_bin" || true)"
[[ -x "$go_bin" ]] || { echo "snapshot-gate-tests: Go executable required" >&2; exit 1; }
for path in \
  tools/snapshot-gate/main.go \
  tools/snapshot-gate/main_test.go \
  acceptance/snapshots/catalog.v1.json; do
  [[ -f "$repo_root/$path" && ! -L "$repo_root/$path" ]] || {
    echo "snapshot-gate-tests: regular file required: $path" >&2
    exit 1
  }
done

test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-snapshot-gate.XXXXXX")"
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/tools/snapshot-gate" "$test_root/acceptance/snapshots" "$test_root/bin"
cp "$repo_root/tools/snapshot-gate/main.go" "$test_root/tools/snapshot-gate/main.go"
cp "$repo_root/tools/snapshot-gate/main_test.go" "$test_root/tools/snapshot-gate/main_test.go"
cp "$repo_root/acceptance/snapshots/catalog.v1.json" "$test_root/acceptance/snapshots/catalog.v1.json"
printf '%s\n' 'module example.com/snapshot-gate-test' '' 'go 1.26.5' >"$test_root/tools/go.mod"

go_env=(env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off)
(cd / && "${go_env[@]}" "$go_bin" -C "$test_root/tools" test -race -timeout=15s ./snapshot-gate)
(cd / && "${go_env[@]}" "$go_bin" -C "$test_root/tools" build \
  -o "$test_root/bin/snapshot-gate" ./snapshot-gate)

runner="$test_root/bin/snapshot-gate"
catalog="$test_root/acceptance/snapshots/catalog.v1.json"
expected='snapshot-gate: PASS (6 cases; validation only)'
[[ "$(cd / && "$runner" validate "$catalog")" = "$expected" ]] || {
  echo "snapshot-gate-tests: canonical six-case catalog was rejected" >&2
  exit 1
}

reject_validate() {
  local name="$1" input="$2" expected_error="$3" path="$test_root/$1.json" output status
  printf '%s' "$input" >"$path"
  set +e
  output="$(cd / && "$runner" validate "$path" 2>&1)"
  status=$?
  set -e
  [[ "$status" -ne 0 && "$output" == *"$expected_error"* ]] || {
    echo "snapshot-gate-tests: accepted or misdiagnosed $name: $output" >&2
    exit 1
  }
}

reject_validate malformed '{' 'decode catalog'
reject_validate unknown '{"version":1,"ignore_paths":[],"cases":[],"extra":true}' 'unknown field'
reject_validate duplicate-field '{"version":1,"version":1,"ignore_paths":[],"cases":[]}' 'duplicate field'
reject_validate null-ignores '{"version":1,"ignore_paths":null,"cases":[]}' 'ignore_paths must be an array'
reject_validate wildcard-ignore '{"version":1,"ignore_paths":["/cases/op/case/response/body/*"],"cases":[]}' 'invalid exact ignore path'

fixture_catalog="$test_root/fixture-catalog.json"
fixture_actual="$test_root/fixture-actual.json"
printf '%s' '{"version":1,"ignore_paths":[],"cases":[{"operation_id":"contacts.get","case_id":"happy","request":{"method":"GET","path":"/api/customers/1","body":null},"expected_response":{"status":200,"body":{"customer":{"id":"1","name":"Ada"}}}}]}' >"$fixture_catalog"
printf '%s' '{"version":1,"cases":[{"operation_id":"contacts.get","case_id":"happy","request":{"method":"GET","path":"/api/customers/1","body":null},"actual_response":{"status":200,"body":{"customer":{"id":"1","name":"Ada"}}}}]}' >"$fixture_actual"
[[ "$(cd / && "$runner" compare "$fixture_catalog" <"$fixture_actual")" = \
  'snapshot-gate: PASS (1 cases compared)' ]] || {
  echo "snapshot-gate-tests: equal generated response was rejected" >&2
  exit 1
}

changed_actual="$test_root/changed-actual.json"
sed 's/"name":"Ada"/"name":"Grace"/' "$fixture_actual" >"$changed_actual"
set +e
changed_output="$(cd / && "$runner" compare "$fixture_catalog" <"$changed_actual" 2>&1)"
changed_status=$?
set -e
[[ "$changed_status" -ne 0 && "$changed_output" == \
  *'snapshot mismatch at /cases/contacts.get/happy/response/body/customer/name'* ]] || {
  echo "snapshot-gate-tests: changed response field was accepted or lacked JSON pointer: $changed_output" >&2
  exit 1
}

ignored_catalog="$test_root/ignored-catalog.json"
sed 's#"ignore_paths":\[\]#"ignore_paths":["/cases/contacts.get/happy/response/body/customer/name"]#' \
  "$fixture_catalog" >"$ignored_catalog"
"$runner" compare "$ignored_catalog" <"$changed_actual" >/dev/null

missing_actual="$test_root/missing-actual.json"
sed 's/,"name":"Ada"//' "$fixture_actual" >"$missing_actual"
set +e
missing_output="$(cd / && "$runner" compare "$ignored_catalog" <"$missing_actual" 2>&1)"
missing_status=$?
set -e
[[ "$missing_status" -ne 0 && "$missing_output" == *'ignore path did not match actual response'* ]] || {
  echo "snapshot-gate-tests: unmatched ignore path was accepted: $missing_output" >&2
  exit 1
}

ln -s "$catalog" "$test_root/symlink.json"
set +e
symlink_output="$(cd / && "$runner" validate "$test_root/symlink.json" 2>&1)"
symlink_status=$?
set -e
[[ "$symlink_status" -ne 0 && "$symlink_output" == *'catalog must be a regular file'* ]] || exit 1

fifo="$test_root/catalog.fifo"
mkfifo "$fifo"
set +e
fifo_output="$(cd / && "$runner" validate "$fifo" 2>&1)"
fifo_status=$?
set -e
[[ "$fifo_status" -ne 0 && "$fifo_output" == *'catalog must be a regular file'* ]] || exit 1

echo "snapshot-gate-tests: PASS"
