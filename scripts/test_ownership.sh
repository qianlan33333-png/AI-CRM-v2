#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/ownership/main.go"
go_bin="${GO:-go}"
[[ "$go_bin" == /* ]] || go_bin="$(type -P "$go_bin" || true)"
[[ -x "$go_bin" && -f "$checker" && ! -L "$checker" ]] || exit 1
test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-ownership.XXXXXX")"
trap 'rm -rf "$test_root"' EXIT
seed() {
  local root="$1"
  mkdir -p "$root/docs/architecture" "$root/internal/contact/store/queries" \
    "$root/internal/segment/store/queries" "$root/internal/outbound/worker" \
    "$root/internal/wecom/store" "$root/internal/platform/store" \
    "$root/internal/events/store/queries" \
    "$root/acceptance/fixtures"
  cp "$script_dir/../docs/architecture/table-ownership.yml" "$root/docs/architecture/"
  printf '%s\n' 'INSERT INTO customers (id) VALUES (1);' >"$root/internal/contact/store/queries/write.sql"
  printf '%s\n' "SELECT 'UPDATE identities'; -- DELETE FROM tags" 'SELECT * FROM customers;' >"$root/internal/segment/store/queries/read.sql"
  printf '%s\n' 'package worker' 'const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/message/send"' >"$root/internal/outbound/worker/client.go"
  printf '%s\n' 'package store' 'const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/externalcontact/get"' >"$root/internal/wecom/store/client.go"
  printf '%s\n' 'package fixtures' \
    'const ddl = "CREATE TABLE acceptance_fixtures.fixture_probe (id bigint PRIMARY KEY)"' \
    'const dml = "INSERT INTO acceptance_fixtures.fixture_probe (id) VALUES (1)"' \
    >"$root/acceptance/fixtures/probe.go"
  printf '%s\n' \
    'INSERT INTO event_log (event_type) VALUES ($1)' \
    'ON CONFLICT (idempotency_key) DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key;' \
    'SELECT id FROM event_log FOR UPDATE SKIP LOCKED;' \
    >"$root/internal/events/store/queries/upsert.sql"
}
run_checker() {
  (cd / && env -u BASH_ENV -u ENV -u GOFLAGS -u GIT_DIR -u GIT_WORK_TREE \
    GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
    "$go_bin" run "$checker" -root "$1")
}
positive="$test_root/positive"
seed "$positive"
[[ "$(run_checker "$positive")" == "ownership-lint: PASS" ]] || exit 1
reject() {
  local name="$1" expected="$2" root="$test_root/$1" output status
  seed "$root"
  mutate "$name" "$root"
  set +e; output="$(run_checker "$root" 2>&1)"; status=$?; set -e
  [[ "$status" -ne 0 && "$output" == *"$expected"* ]] || {
    echo "ownership-lint-tests: accepted or misdiagnosed $name: $output" >&2
    exit 1
  }
}
mutate() {
  local name="$1" root="$2"
  case "$name" in
    contact-identity) echo 'UPDATE identities SET customer_id = 1;' >"$root/internal/contact/store/queries/write.sql" ;;
    contact-identity-receipt) echo 'INSERT INTO identity_operation_receipts (operation) VALUES ('\''bind'\'');' >"$root/internal/contact/store/queries/write.sql" ;;
    segment-write) echo 'DELETE FROM customers;' >"$root/internal/segment/store/queries/read.sql" ;;
    platform-write) echo 'INSERT INTO event_log DEFAULT VALUES;' >"$root/internal/platform/store/write.sql" ;;
    contact-event-update) echo 'UPDATE event_log SET dispatched = true;' >"$root/internal/contact/store/queries/write.sql" ;;
    contact-auth-session) echo 'UPDATE admin_sessions SET revoked_reason = '\''bypass'\'';' >"$root/internal/contact/store/queries/write.sql" ;;
    unknown-table) echo 'TRUNCATE TABLE ONLY mystery_table;' >"$root/internal/contact/store/queries/write.sql" ;;
    update-unknown-table) echo 'UPDATE mystery_table AS target SET id = 2;' >"$root/internal/contact/store/queries/write.sql" ;;
    public-fixture) printf '%s\n' 'package fixtures' 'const ddl = "CREATE TABLE public.mystery_table (id bigint PRIMARY KEY)"' >"$root/acceptance/fixtures/probe.go" ;;
    outbound-read) echo 'package worker; const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/externalcontact/get"' >"$root/internal/outbound/worker/client.go" ;;
    wecom-write) echo 'package store; const endpoint = "/cgi-bin/message/send"' >"$root/internal/wecom/store/client.go" ;;
    contact-endpoint) mkdir -p "$root/internal/contact/app"; echo 'package app; const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/message/send"' >"$root/internal/contact/app/client.go" ;;
    contact-sdk) mkdir -p "$root/internal/contact/app"; echo 'package app; import _ "example.com/wecomsdk"' >"$root/internal/contact/app/client.go" ;;
    unknown-operation) echo 'package worker; const endpoint = "https://qyapi.weixin.qq.com/cgi-bin/unknown/write"' >"$root/internal/outbound/worker/client.go" ;;
    fifo) mkfifo "$root/internal/contact/unexpected" ;;
  esac
}
reject contact-identity 'table write ownership violation'; reject segment-write 'table write ownership violation'
reject contact-identity-receipt 'table write ownership violation'
reject platform-write 'table write ownership violation'; reject contact-event-update 'table write ownership violation'
reject contact-auth-session 'table write ownership violation'
reject unknown-table 'write to unknown table'
reject update-unknown-table 'write to unknown table'
reject public-fixture 'write to unknown table'
reject outbound-read 'WeCom operation ownership violation'; reject wecom-write 'WeCom operation ownership violation'
reject contact-endpoint 'WeCom operation ownership violation'; reject contact-sdk 'external WeCom client import forbidden'
reject unknown-operation 'unknown WeCom operation'
reject fifo 'symlink or special path forbidden'
echo "ownership-lint-tests: PASS"
