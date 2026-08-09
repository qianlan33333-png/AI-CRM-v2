#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "slice-input-contract-tests: $*" >&2
  exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
canonical="$script_dir/check_slice_inputs.sh"
test_root="$(mktemp -d -t aicrm-v2-slice-inputs.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

make_fixture() {
  local name="$1" fixture
  fixture="$test_root/$name"
  mkdir -p "$fixture/scripts" "$fixture/docs/execution/slices"
  cp "$canonical" "$fixture/scripts/check_slice_inputs.sh"
  chmod 755 "$fixture/scripts/check_slice_inputs.sh"
  printf '%s\n' "$fixture"
}

run_rejected() {
  local name="$1" fixture="$2" expected="$3" log
  log="$test_root/$name.log"
  if (cd / && BASH_ENV= ENV= PATH=/usr/bin:/bin /bin/bash "$fixture/scripts/check_slice_inputs.sh" >"$log" 2>&1); then
    fail "negative fixture was accepted: $name"
  fi
  grep -Fq "$expected" "$log" || fail "negative fixture missed diagnostic: $name"
}

valid="$(make_fixture valid)"
printf '%s\n' '# Slice valid' '- slice_kind: implementation' '- task_inputs:' \
  '  - `docs/rules/contact.md`' '  - `docs/spec/design.md`' '  - `api/openapi.yaml`' \
  >"$valid/docs/execution/slices/valid.md"
output="$(cd / && BASH_ENV= ENV= PATH=/usr/bin:/bin /bin/bash "$valid/scripts/check_slice_inputs.sh")"
[[ "$output" = 'slice-input-contract: PASS (implementation_cards=1)' ]] || fail "valid implementation fixture was rejected"

evidence="$(make_fixture evidence)"
printf '%s\n' '# Slice evidence' '- slice_kind: evidence' '- task_inputs:' \
  '  - `/legacy/aicrm_next/routes.py`' >"$evidence/docs/execution/slices/evidence.md"
/bin/bash "$evidence/scripts/check_slice_inputs.sh" >/dev/null || fail "evidence slice may cite legacy Python"

missing="$(make_fixture missing)"
printf '%s\n' '# Slice missing' '- slice_kind: implementation' >"$missing/docs/execution/slices/missing.md"
run_rejected missing "$missing" 'must declare exactly one task_inputs block'

python="$(make_fixture python)"
printf '%s\n' '# Slice python' '- slice_kind: implementation' '- task_inputs:' \
  '  - `aicrm_next/routes.py`' >"$python/docs/execution/slices/python.md"
run_rejected python "$python" 'legacy Python input forbidden'

absolute="$(make_fixture absolute)"
printf '%s\n' '# Slice absolute' '- slice_kind: implementation' '- task_inputs:' \
  '  - `/Users/example/legacy/route.py`' >"$absolute/docs/execution/slices/absolute.md"
run_rejected absolute "$absolute" 'legacy Python input forbidden'

disguised="$(make_fixture disguised)"
printf '%s\n' '# Slice disguised' '- slice_kind: implementation' '- task_inputs:' \
  '  - `docs/rules/contact%2epy`' >"$disguised/docs/execution/slices/disguised.md"
run_rejected disguised "$disguised" 'legacy Python input forbidden'

printf '%s\n' 'slice-input-contract-tests: PASS'
