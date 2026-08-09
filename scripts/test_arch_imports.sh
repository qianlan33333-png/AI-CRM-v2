#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/check_arch_imports.go"
go_bin="${GO:-go}"
if [[ "$go_bin" != /* ]]; then
  go_bin="$(type -P "$go_bin" || true)"
fi
[[ -n "$go_bin" && -x "$go_bin" && -f "$checker" && ! -L "$checker" ]] || {
  echo "arch-import-lint-tests: trusted checker or Go binary unavailable" >&2
  exit 1
}

test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-arch-imports.XXXXXX")"
trap 'rm -rf "$test_root"' EXIT

seed() {
  local root="$1"
  mkdir -p "$root/cmd/aicrm" "$root/internal/contact/app" \
    "$root/internal/identity/port" "$root/internal/identity/store" \
    "$root/internal/platform/store" "$root/internal/api/generated"
  printf '%s\n' 'package app' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"' \
    >"$root/internal/contact/app/use.go"
  printf '%s\n' 'package main' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"' \
    >"$root/cmd/aicrm/main.go"
  printf '%s\n' 'package store' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"' \
    >"$root/internal/identity/store/use.go"
}

run_checker() {
  local root="$1"
  (cd / && env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE \
    GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
    "$go_bin" run "$checker" -root "$root")
}

positive="$test_root/positive"
seed "$positive"
[[ "$(run_checker "$positive")" == "arch-import-lint: PASS" ]] || {
  echo "arch-import-lint-tests: positive fixture failed" >&2
  exit 1
}

reject() {
  local name="$1" expected="$2" root output status
  root="$test_root/$name"
  seed "$root"
  mutate "$name" "$root"
  set +e
  output="$(run_checker "$root" 2>&1)"
  status=$?
  set -e
  [[ "$status" -ne 0 && "$output" == *"$expected"* ]] || {
    echo "arch-import-lint-tests: accepted or misdiagnosed $name: $output" >&2
    exit 1
  }
}

mutate() {
  local name="$1" root="$2"
  case "$name" in
    concrete)
      printf '%s\n' 'package app' 'import alias "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"' >"$root/internal/contact/app/use.go" ;;
    nested-port)
      printf '%s\n' 'package app' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port/internal"' >"$root/internal/contact/app/use.go" ;;
    platform-domain)
      printf '%s\n' 'package store' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"' >"$root/internal/platform/store/use.go" ;;
    api-domain)
      printf '%s\n' 'package generated' 'import . "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"' >"$root/internal/api/generated/use.go" ;;
    unknown)
      printf '%s\n' 'package app' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/shared/port"' >"$root/internal/contact/app/use.go" ;;
    malformed) printf '%s\n' 'package app' 'import (' >"$root/internal/contact/app/use.go" ;;
    symlink) ln -s ../identity "$root/internal/contact/link" ;;
    fifo) mkfifo "$root/internal/contact/unexpected" ;;
  esac
}

reject concrete 'forbidden cross-module import'
reject nested-port 'forbidden cross-module import'
reject platform-domain 'forbidden cross-module import'
reject api-domain 'forbidden cross-module import'
reject unknown 'unknown internal module'
reject malformed 'parse internal/contact/app/use.go'
reject symlink 'symlink or special path forbidden'
reject fifo 'symlink or special path forbidden'

echo "arch-import-lint-tests: PASS"
