#!/usr/bin/env bash
set -euo pipefail

fail() { echo "query-plan-gate-tests: $*" >&2; exit 1; }
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$script_dir/.." && pwd -P)"
go_bin="${GO:-go}"
database_url="${QUERY_PLAN_TEST_DATABASE_URL:-}"
test_root="$(mktemp -d -t aicrm-v2-query-plan.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT
gate_bin="$test_root/query-plan-gate"
GOWORK=off GOFLAGS=-mod=readonly "$go_bin" -C "$repo_root/tools" build -o "$gate_bin" ./query-plan-gate

make_fixture() {
  local name="$1" query_sql="$2" fixture
  fixture="$test_root/$name"
  mkdir -p "$fixture/migrations" "$fixture/internal/contact/store/queries"
  printf '%s\n' \
    '-- +goose Up' \
    'CREATE TABLE customers (id BIGINT PRIMARY KEY, email TEXT NOT NULL);' \
    'CREATE INDEX idx_customers_email ON customers(email);' \
    'CREATE TABLE customer_events (id BIGINT PRIMARY KEY, customer_id BIGINT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL);' \
    'CREATE INDEX idx_customer_events_customer ON customer_events(customer_id, occurred_at DESC);' \
    "INSERT INTO customers SELECT n, n::text || '@example.test' FROM generate_series(1, 10000) AS n;" \
    "INSERT INTO customer_events SELECT n, (n % 10000) + 1, now() FROM generate_series(1, 10000) AS n;" \
    'ANALYZE customers;' 'ANALYZE customer_events;' \
    '-- +goose Down' 'DROP TABLE customer_events;' 'DROP TABLE customers;' \
    >"$fixture/migrations/00001_fixture.sql"
  printf '%s\n' '-- name: Baseline :one' 'SELECT 1;' >"$fixture/internal/contact/store/queries/contact.sql"
  git -C "$fixture" init -q
  git -C "$fixture" config user.name fixture
  git -C "$fixture" config user.email fixture@example.invalid
  git -C "$fixture" add -A
  git -C "$fixture" commit -qm baseline
  FIXTURE_BASE="$(git -C "$fixture" rev-parse HEAD)"
  printf '%s\n' "$query_sql" >"$fixture/internal/contact/store/queries/contact.sql"
  git -C "$fixture" add -A
  git -C "$fixture" commit -qm candidate
  FIXTURE_HEAD="$(git -C "$fixture" rev-parse HEAD)"
  FIXTURE_ROOT="$fixture"
}

run_gate() {
  "$gate_bin" -root "$FIXTURE_ROOT" -base "$FIXTURE_BASE" -head "$FIXTURE_HEAD" -database-url "$database_url"
}

make_fixture indexed $'-- name: CustomerByID :one\nSELECT * FROM customers WHERE id = $1;'
[[ "$(run_gate)" = 'query-plan-gate: PASS (checked=1)' ]] || fail "indexed query was rejected"

make_fixture unrelated $'-- name: Health :one\nSELECT 1;'
[[ "$(run_gate)" = 'query-plan-gate: PASS (checked=0)' ]] || fail "unrelated query was rejected"

make_fixture fullscan $'-- name: AllCustomers :many\nSELECT * FROM customers;'
fullscan_log="$test_root/fullscan.log"
set +e
/usr/bin/perl -e 'alarm 15; exec @ARGV' "$gate_bin" -root "$FIXTURE_ROOT" -base "$FIXTURE_BASE" -head "$FIXTURE_HEAD" -database-url "$database_url" >"$fullscan_log" 2>&1
status=$?
set -e
[[ "$status" -ne 0 && "$status" -ne 142 ]] || fail "full-table scan was accepted or timed out"
grep -Fq 'Seq Scan on customers' "$fullscan_log" || fail "full-table scan missed its diagnostic"

make_fixture malformed $'-- name: Broken :one\nSELECT * FROM customers WHERE id = $1 +;'
if run_gate >/dev/null 2>&1; then fail "unparseable query was accepted"; fi

make_fixture migration $'-- name: CustomerEvents :many\nSELECT * FROM customer_events WHERE customer_id = $1 ORDER BY occurred_at DESC;'
FIXTURE_BASE="$FIXTURE_HEAD"
printf '%s\n' '-- migration receipt change' >>"$FIXTURE_ROOT/migrations/00001_fixture.sql"
git -C "$FIXTURE_ROOT" add migrations/00001_fixture.sql
git -C "$FIXTURE_ROOT" commit -qm migration-change
FIXTURE_HEAD="$(git -C "$FIXTURE_ROOT" rev-parse HEAD)"
[[ "$(run_gate)" = 'query-plan-gate: PASS (checked=1)' ]] || fail "migration-triggered indexed query was rejected"

if "$gate_bin" -root "$FIXTURE_ROOT" -base "$FIXTURE_BASE" -head "$FIXTURE_HEAD" -database-url '' >/dev/null 2>&1; then
  fail "missing database URL was accepted"
fi

printf '%s\n' 'query-plan-gate-tests: PASS'
