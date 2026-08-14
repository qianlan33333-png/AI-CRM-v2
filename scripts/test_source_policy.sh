#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/sourcepolicy/main.go"
go_bin="${GO:-go}"; [[ "$go_bin" == /* ]] || go_bin="$(type -P "$go_bin" || true)"
[[ -x "$go_bin" && -f "$checker" && ! -L "$checker" ]] || exit 1
test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-source-policy.XXXXXX")"
trap 'rm -rf "$test_root"' EXIT
seed() {
  local root="$1"
  mkdir -p "$root/cmd/aicrm" "$root/internal/platform/runtime" "$root/internal/config/source" \
    "$root/internal/contact/store/queries" "$root/internal/contact/store/generated" "$root/internal/contact/app" \
    "$root/internal/events/dispatcher" \
    "$root/internal/api/candidate/generated"
  echo 'package main' >"$root/cmd/aicrm/main.go"
  mkdir -p "$root/cmd/aicrm-contact-perf-data" "$root/cmd/aicrm-contact-perf"
  echo 'package main; const reset = "TRUNCATE TABLE customers"; func run(){ db.Exec(reset); db.CopyFrom() }' >"$root/cmd/aicrm-contact-perf-data/main.go"
  echo 'package main; const inspect = "SELECT count(*) FROM customers"; func run(){ db.QueryRow(inspect); db.Query(inspect) }' >"$root/cmd/aicrm-contact-perf/main.go"
  printf '%s\n' 'package runtime' 'import "time"' 'func bounded(){ _ = time.NewTimer(time.Second) }' >"$root/internal/platform/runtime/timer.go"
  printf '%s\n' 'package source' 'import "os"' 'func load(){ _, _ = os.LookupEnv("KEY") }' >"$root/internal/config/source/env.go"
  echo 'SELECT 1;' >"$root/internal/contact/store/queries/list.sql"
  echo 'package generated; const query = "SELECT 1"; func call(){ db.QueryRow(ctx, query) }' >"$root/internal/contact/store/generated/query.go"
  echo 'package generated; const query = "SELECT 1"; func call(){ db.Query(query) }' >"$root/internal/api/candidate/generated/server.gen.go"
  printf '%s\n' 'package app' 'import "net/http"' 'func readQuery(r *http.Request){ _ = r.URL.Query() }' >"$root/internal/contact/app/app.go"
  printf '%s\n' 'package store' 'type customerQueryDBTX struct{ Tx database }' 'func (db customerQueryDBTX) Query(){ db.Tx.Query() }' 'func (db customerQueryDBTX) QueryRow(){ db.Tx.QueryRow() }' >"$root/internal/contact/store/customer_query_repository.go"
}
run_checker() {
  (cd / && env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off "$go_bin" run "$checker" -root "$1")
}
positive="$test_root/positive"; seed "$positive"
[[ "$(run_checker "$positive")" == "source-policy-lint: PASS" ]] || exit 1
reject() {
  local name="$1" expected="$2" root="$test_root/$1" output status
  seed "$root"; mutate "$name" "$root"
  set +e; output="$(run_checker "$root" 2>&1)"; status=$?; set -e
  [[ "$status" -ne 0 && "$output" == *"$expected"* ]] || { echo "source-policy-tests: accepted or misdiagnosed $name: $output" >&2; exit 1; }
}
mutate() {
  local name="$1" root="$2"
  case "$name" in
    env) echo 'package app; import e "os"; func f(){ _ = e.Getenv("X") }' >"$root/internal/contact/app/app.go" ;;
    env-loader) echo 'package app; import _ "example.com/godotenv"' >"$root/internal/contact/app/app.go" ;;
    sql-path) echo 'SELECT 1;' >"$root/internal/contact/store/direct.sql" ;;
    sql-literal) echo 'package app; const q = "UPDATE customers SET name=$1"' >"$root/internal/contact/app/app.go" ;;
    sql-split) echo 'package app; const q = "SEL"+("ECT 1")' >"$root/internal/contact/app/app.go" ;;
    db-call) echo 'package app; func f(){ db.Query("SELECT * FROM customers") }' >"$root/internal/contact/app/app.go" ;;
    dispatcher-savepoint) echo 'package dispatcher; func f(){ db.Exec("SAVEPOINT automation_delivery") }' >"$root/internal/events/dispatcher/jobs.go" ;;
    performance-command-copy) mkdir -p "$root/cmd/aicrm-contact-perf-data-copy"; echo 'package main; const q = "TRUNCATE TABLE customers"; func f(){ db.Exec(q) }' >"$root/cmd/aicrm-contact-perf-data-copy/main.go" ;;
    performance-runner-copy) mkdir -p "$root/cmd/aicrm-contact-perf-copy"; echo 'package main; const q = "SELECT count(*) FROM customers"; func f(){ db.Query(q) }' >"$root/cmd/aicrm-contact-perf-copy/main.go" ;;
    customer-plan-wrapper-copy) echo 'package store; type wrapper struct{ Tx database }; func (db wrapper) f(){ db.Tx.Query() }' >"$root/internal/contact/store/customer_query_plan_copy.go" ;;
    customer-plan-wrapper-wrong-receiver) echo 'package store; type otherWrapper struct{ Tx database }; func (db otherWrapper) Query(){ db.Tx.Query() }; func (db customerQueryDBTX) QueryRow(){ db.Tx.QueryRow() }' >"$root/internal/contact/store/customer_query_repository.go" ;;
    customer-plan-wrapper-shadowed-receiver) echo 'package store; type customerQueryDBTX struct{ Tx database }; type otherWrapper struct{ Tx database }; func (db customerQueryDBTX) Query(){ { db := otherWrapper{}; db.Tx.Query() } }; func (db customerQueryDBTX) QueryRow(){ db.Tx.QueryRow() }' >"$root/internal/contact/store/customer_query_repository.go" ;;
    customer-plan-wrapper-exec) echo 'package store; type customerQueryDBTX struct{ Tx database }; func (db customerQueryDBTX) Query(){ db.Tx.Exec() }; func (db customerQueryDBTX) QueryRow(){ db.Tx.QueryRow() }' >"$root/internal/contact/store/customer_query_repository.go" ;;
    candidate-manual) mkdir -p "$root/internal/api/candidate"; echo 'package candidate; func f(){ db.Query("SELECT * FROM customers") }' >"$root/internal/api/candidate/manual.go" ;;
    orm) echo 'package app; import _ "gorm.io/gorm"' >"$root/internal/contact/app/app.go" ;;
    ticker) echo 'package app; import clock "time"; func f(){ _ = clock.NewTicker(clock.Second) }' >"$root/internal/contact/app/app.go" ;;
    after-func) echo 'package app; import "time"; func f(){ time.AfterFunc(time.Second, f) }' >"$root/internal/contact/app/app.go" ;;
    cron) echo 'package app; import _ "github.com/robfig/cron/v3"' >"$root/internal/contact/app/app.go" ;;
    fifo) mkfifo "$root/internal/contact/unexpected" ;;
  esac
}
reject env 'environment read forbidden'; reject env-loader 'environment loader forbidden'
reject sql-path 'SQL source outside'; reject sql-literal 'handwritten SQL forbidden'; reject sql-split 'constructed SQL forbidden'
reject db-call 'direct database call forbidden'; reject candidate-manual 'direct database call forbidden'; reject orm 'dynamic SQL library forbidden'
reject dispatcher-savepoint 'direct database call forbidden'
reject performance-command-copy 'handwritten SQL forbidden'
reject performance-runner-copy 'handwritten SQL forbidden'
reject customer-plan-wrapper-copy 'direct database call forbidden'
reject customer-plan-wrapper-wrong-receiver 'direct database call forbidden'
reject customer-plan-wrapper-shadowed-receiver 'direct database call forbidden'
reject customer-plan-wrapper-exec 'direct database call forbidden'
reject ticker 'business timer forbidden'; reject after-func 'business timer forbidden'
reject cron 'third-party cron forbidden'; reject fifo 'symlink or special path forbidden'
echo "source-policy-tests: PASS"
