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
    "$root/internal/product/membergrid" \
    "$root/internal/events/dispatcher" "$root/internal/stats/app" \
    "$root/internal/api/candidate/generated" "$root/scripts/sourcepolicy"
  printf '%s\n' '# source-policy-baseline-v1: path<TAB>sorted_rule_counts<TAB>ordered_syntax_sha256' >"$root/scripts/sourcepolicy/baseline.tsv"
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
  printf '%s\n' 'package main' \
    'const page = `<label>State <select name="state"><option>all</option></select></label>`' \
    'const updateMessage = "invalid image update request"' \
    'const copyMessage = "copy replay remained stable"' \
    >"$root/cmd/aicrm/non_sql_text.go"
  printf '%s\n' 'package membergrid' \
    'type Application interface { Query() }' \
    'type Handler struct { application Application }' \
    'func (handler *Handler) Query(){ handler.application.Query() }' \
    'type routeFragment struct { handler *Handler }' \
    'func (fragment *routeFragment) ServeHTTP(){ fragment.handler.Query() }' \
    >"$root/internal/product/membergrid/http.go"
  printf '%s\n' 'package membergrid' \
    'type Service struct{}' \
    'func (*Service) Query(){}' \
    'func newTestService() *Service { return &Service{} }' \
    'func testQuery(){ service := newTestService(); service.Query() }' \
    >"$root/internal/product/membergrid/service_test.go"
  printf '%s\n' 'package store' 'type customerQueryDBTX struct{ Tx database }' 'func (db customerQueryDBTX) Query(){ db.Tx.Query() }' 'func (db customerQueryDBTX) QueryRow(){ db.Tx.QueryRow() }' >"$root/internal/contact/store/customer_query_repository.go"
}
run_checker() {
  (cd / && env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off "$go_bin" run "$checker" -root "$1")
}
print_baseline() {
  (cd / && env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off "$go_bin" run "$checker" -root "$1" -print-baseline)
}
positive="$test_root/positive"; seed "$positive"
[[ "$(run_checker "$positive")" == "source-policy-lint: PASS_WITH_BASELINE(0)" ]] || exit 1
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
    sql-leading-block-comment) echo 'package app; const q = "/* audit */ SELECT id FROM customers"' >"$root/internal/contact/app/app.go" ;;
    sql-leading-line-comment) echo 'package app; const q = "-- audit SELECT id FROM customers"' >"$root/internal/contact/app/app.go" ;;
    sql-with-select) echo 'package app; const q = "WITH visible AS(SELECT id FROM customers) SELECT id FROM visible"' >"$root/internal/contact/app/app.go" ;;
    sql-with-update) echo 'package app; const q = "WITH visible AS MATERIALIZED (SELECT id FROM customers) UPDATE customers SET name=$1 FROM visible WHERE customers.id=visible.id"' >"$root/internal/contact/app/app.go" ;;
    sql-insert) echo 'package app; const q = "INSERT INTO customers DEFAULT VALUES"' >"$root/internal/contact/app/app.go" ;;
    sql-delete) echo 'package app; const q = "DELETE FROM customers"' >"$root/internal/contact/app/app.go" ;;
    sql-merge) echo 'package app; const q = "MERGE INTO customers USING source ON false WHEN NOT MATCHED THEN DO NOTHING"' >"$root/internal/contact/app/app.go" ;;
    sql-copy) echo 'package app; const q = "COPY customers FROM STDIN"' >"$root/internal/contact/app/app.go" ;;
    sql-truncate) echo 'package app; const q = "TRUNCATE TABLE customers"' >"$root/internal/contact/app/app.go" ;;
    db-call) echo 'package app; func f(){ db.Query("SELECT * FROM customers") }' >"$root/internal/contact/app/app.go" ;;
    dispatcher-savepoint) echo 'package dispatcher; func f(){ db.Exec("SAVEPOINT automation_delivery") }' >"$root/internal/events/dispatcher/jobs.go" ;;
    stats-runtime-sql) echo 'package app; func f(){ db.Query("SELECT value FROM stats_daily") }' >"$root/internal/stats/app/app.go" ;;
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
reject sql-leading-block-comment 'handwritten SQL forbidden'; reject sql-leading-line-comment 'handwritten SQL forbidden'
reject sql-with-select 'handwritten SQL forbidden'; reject sql-with-update 'handwritten SQL forbidden'
reject sql-insert 'handwritten SQL forbidden'; reject sql-delete 'handwritten SQL forbidden'
reject sql-merge 'handwritten SQL forbidden'; reject sql-copy 'handwritten SQL forbidden'; reject sql-truncate 'handwritten SQL forbidden'
reject db-call 'direct database call forbidden'; reject candidate-manual 'direct database call forbidden'; reject orm 'dynamic SQL library forbidden'
reject dispatcher-savepoint 'direct database call forbidden'
reject stats-runtime-sql 'direct database call forbidden'
reject performance-command-copy 'handwritten SQL forbidden'
reject performance-runner-copy 'handwritten SQL forbidden'
reject customer-plan-wrapper-copy 'direct database call forbidden'
reject customer-plan-wrapper-wrong-receiver 'direct database call forbidden'
reject customer-plan-wrapper-shadowed-receiver 'direct database call forbidden'
reject customer-plan-wrapper-exec 'direct database call forbidden'
reject ticker 'business timer forbidden'; reject after-func 'business timer forbidden'
reject cron 'third-party cron forbidden'; reject fifo 'symlink or special path forbidden'

baseline="$test_root/baseline"; seed "$baseline"
echo 'package app; func legacy(){ db.Query("SELECT id FROM customers") }' >"$baseline/internal/contact/app/debt.go"
print_baseline "$baseline" >"$baseline/scripts/sourcepolicy/baseline.tsv"
[[ "$(run_checker "$baseline")" == "source-policy-lint: PASS_WITH_BASELINE(1)" ]] || exit 1
printf '%s\n' 'package app' 'func legacy() {' ' db.Query("SELECT id FROM customers")' '}' >"$baseline/internal/contact/app/debt.go"
[[ "$(run_checker "$baseline")" == "source-policy-lint: PASS_WITH_BASELINE(1)" ]] || { echo "source-policy-tests: formatting changed a stable fingerprint" >&2; exit 1; }

echo 'package app; func added(){ db.Exec("DELETE FROM customers") }' >"$baseline/internal/contact/app/added.go"
set +e; output="$(run_checker "$baseline" 2>&1)"; status=$?; set -e
[[ "$status" -ne 0 && "$output" == *"unexpected baseline debt"* ]] || { echo "source-policy-tests: accepted new debt: $output" >&2; exit 1; }
rm "$baseline/internal/contact/app/added.go"

echo 'package app; func legacy(){ db.Exec("DELETE FROM customers") }' >"$baseline/internal/contact/app/debt.go"
set +e; output="$(run_checker "$baseline" 2>&1)"; status=$?; set -e
[[ "$status" -ne 0 && "$output" == *"unexpected baseline debt"* ]] || { echo "source-policy-tests: accepted changed debt: $output" >&2; exit 1; }
echo 'package app; func legacy(){ db.Query("SELECT id FROM customers") }' >"$baseline/internal/contact/app/debt.go"

mv "$baseline/internal/contact/app/debt.go" "$baseline/internal/contact/app/moved.go"
set +e; output="$(run_checker "$baseline" 2>&1)"; status=$?; set -e
[[ "$status" -ne 0 && "$output" == *"unexpected baseline debt"* ]] || { echo "source-policy-tests: accepted moved debt: $output" >&2; exit 1; }
mv "$baseline/internal/contact/app/moved.go" "$baseline/internal/contact/app/debt.go"

rm "$baseline/internal/contact/app/debt.go"
set +e; output="$(run_checker "$baseline" 2>&1)"; status=$?; set -e
[[ "$status" -ne 0 && "$output" == *"stale baseline debt"* ]] || { echo "source-policy-tests: accepted stale baseline: $output" >&2; exit 1; }

malformed="$test_root/malformed"; seed "$malformed"
printf '%s\n' '# source-policy-baseline-v1: path<TAB>sorted_rule_counts<TAB>ordered_syntax_sha256' 'not-a-valid-entry' >"$malformed/scripts/sourcepolicy/baseline.tsv"
set +e; output="$(run_checker "$malformed" 2>&1)"; status=$?; set -e
[[ "$status" -ne 0 && "$output" == *"malformed baseline entry"* ]] || { echo "source-policy-tests: accepted malformed baseline: $output" >&2; exit 1; }

duplicate="$test_root/duplicate"; seed "$duplicate"
echo 'package app; func legacy(){ db.Query("SELECT id FROM customers") }' >"$duplicate/internal/contact/app/debt.go"
print_baseline "$duplicate" >"$duplicate/scripts/sourcepolicy/baseline.tsv"
tail -n 1 "$duplicate/scripts/sourcepolicy/baseline.tsv" >>"$duplicate/scripts/sourcepolicy/baseline.tsv"
set +e; output="$(run_checker "$duplicate" 2>&1)"; status=$?; set -e
[[ "$status" -ne 0 && "$output" == *"duplicate baseline entry"* ]] || { echo "source-policy-tests: accepted duplicate baseline: $output" >&2; exit 1; }
echo "source-policy-tests: PASS"
