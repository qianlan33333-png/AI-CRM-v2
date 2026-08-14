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
  mkdir -p "$root/cmd/aicrm" "$root/cmd/aicrm-river-migrate" "$root/cmd/aicrm-contact-perf" "$root/internal/contact/app" \
    "$root/internal/identity/port" "$root/internal/identity/store" \
    "$root/internal/automation/app" "$root/internal/events/store" \
    "$root/internal/outbound/app" \
    "$root/internal/platform/store" "$root/internal/api/generated" \
    "$root/internal/config"
  printf '%s\n' 'package app' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"' \
    >"$root/internal/contact/app/use.go"
  printf '%s\n' 'package main' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"' \
    >"$root/cmd/aicrm/main.go"
  printf '%s\n' 'package main' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/config"' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"' \
    >"$root/cmd/aicrm-river-migrate/main.go"
  printf '%s\n' 'package main' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"' \
    >"$root/cmd/aicrm-contact-perf/main.go"
  printf '%s\n' 'package store' \
    'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"' \
    >"$root/internal/identity/store/use.go"
  printf '%s\n' 'package app' \
    'import queue "github.com/riverqueue/river"' \
    'var _ queue.JobArgs' \
    >"$root/internal/outbound/app/use.go"
  printf '%s\n' 'package config' 'import "os"' \
    'func load() { _, _ = os.LookupEnv("AICRM_DATABASE_URL") }' \
    >"$root/internal/config/load.go"
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
    automation-events-store)
      printf '%s\n' 'package app' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"' >"$root/internal/automation/app/use.go" ;;
    nested-port)
      printf '%s\n' 'package app' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port/internal"' >"$root/internal/contact/app/use.go" ;;
    platform-domain)
      printf '%s\n' 'package store' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"' >"$root/internal/platform/store/use.go" ;;
    api-domain)
      printf '%s\n' 'package generated' 'import . "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"' >"$root/internal/api/generated/use.go" ;;
    unknown)
      printf '%s\n' 'package app' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/shared/port"' >"$root/internal/contact/app/use.go" ;;
    unapproved-composition-root)
      mkdir -p "$root/cmd/aicrm-copy"
      printf '%s\n' 'package main' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/config"' >"$root/cmd/aicrm-copy/main.go" ;;
    performance-composition-copy)
      mkdir -p "$root/cmd/aicrm-contact-perf-copy"
      printf '%s\n' 'package main' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"' >"$root/cmd/aicrm-contact-perf-copy/main.go" ;;
    scattered-env)
      printf '%s\n' 'package app' 'import "os"' 'var _ = os.Getenv("DATABASE_URL")' >"$root/internal/contact/app/use.go" ;;
    aliased-env)
      printf '%s\n' 'package main' 'import system "os"' 'var _ = system.LookupEnv("DATABASE_URL")' >"$root/cmd/aicrm/main.go" ;;
    raw-river-client)
      printf '%s\n' 'package app' 'import queue "github.com/riverqueue/river"' 'var _ *queue.Client[any]' >"$root/internal/outbound/app/use.go" ;;
    default-river-queue)
      printf '%s\n' 'package app' 'import queue "github.com/riverqueue/river"' 'var _ = queue.QueueDefault' >"$root/internal/outbound/app/use.go" ;;
    raw-river-driver)
      printf '%s\n' 'package main' 'import _ "github.com/riverqueue/river/riverdriver/riverpgxv5"' >"$root/cmd/aicrm/river.go" ;;
    raw-periodic-constructor)
      printf '%s\n' 'package app' 'import queue "github.com/riverqueue/river"' 'var _ = queue.NewPeriodicJob' >"$root/internal/outbound/app/use.go" ;;
    dynamic-periodic-registration)
      printf '%s\n' 'package app' 'func register(){ client.PeriodicJobs() }' >"$root/internal/outbound/app/use.go" ;;
    scattered-scheduler-import)
      printf '%s\n' 'package app' 'import _ "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/scheduler"' >"$root/internal/outbound/app/use.go" ;;
    malformed) printf '%s\n' 'package app' 'import (' >"$root/internal/contact/app/use.go" ;;
    symlink) ln -s ../identity "$root/internal/contact/link" ;;
    fifo) mkfifo "$root/internal/contact/unexpected" ;;
  esac
}

reject concrete 'forbidden cross-module import'
reject automation-events-store 'forbidden cross-module import'
reject nested-port 'forbidden cross-module import'
reject platform-domain 'forbidden cross-module import'
reject api-domain 'forbidden cross-module import'
reject unknown 'unknown internal module'
reject unapproved-composition-root 'forbidden cross-module import'
reject performance-composition-copy 'forbidden cross-module import'
reject scattered-env 'scattered environment read forbidden'
reject aliased-env 'scattered environment read forbidden'
reject raw-river-client 'raw or default River symbol forbidden'
reject default-river-queue 'raw or default River symbol forbidden'
reject raw-river-driver 'raw River driver forbidden'
reject raw-periodic-constructor 'raw or default River symbol forbidden'
reject dynamic-periodic-registration 'raw or default River symbol forbidden'
reject scattered-scheduler-import 'scheduler registration import forbidden outside the unique catalog'
reject malformed 'parse internal/contact/app/use.go'
reject symlink 'symlink or special path forbidden'
reject fifo 'symlink or special path forbidden'

echo "arch-import-lint-tests: PASS"
