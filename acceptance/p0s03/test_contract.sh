#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
test_root="$(mktemp -d -t aicrm-v2-p0-s03-contract.XXXXXX)"
cleanup() { [[ -n "${test_root:-}" && -d "$test_root" ]] && rm -rf -- "$test_root"; }
trap cleanup EXIT
fail() { printf 'p0-s03-contract-tests: %s\n' "$*" >&2; exit 1; }

make_fixture() {
	local fixture="$test_root/$1"
	mkdir -p "$fixture/acceptance/p0s03" "$fixture/internal/platform/store/generated" "$fixture/internal/platform/store/queries"
	cp "$repo_root/acceptance/p0s03/static_contract.sh" "$fixture/acceptance/p0s03/static_contract.sh"
	cp "$repo_root/acceptance/p0s03/source_contract.go" "$fixture/acceptance/p0s03/source_contract.go"
	cp "$repo_root/internal/platform/store/queries/health.sql" "$fixture/internal/platform/store/queries/health.sql"
	for generated_file in db.go health.sql.go models.go querier.go; do cp "$repo_root/internal/platform/store/generated/$generated_file" "$fixture/internal/platform/store/generated/$generated_file"; done
	chmod 755 "$fixture/acceptance/p0s03/static_contract.sh"
	cp "$repo_root/go.mod" "$repo_root/go.sum" "$fixture/"
	printf '%s\n' 'package platformstore' >"$fixture/internal/platform/store/contract.go"
	printf '%b' 'package platformstore\n\nimport (\n\t"context"\n\t"fmt"\n\n\tdbgen "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store/generated"\n)\n\n' \
		'type PingStore struct {\n\tquerier dbgen.Querier\n}\n\nfunc NewPingStore(db dbgen.DBTX) *PingStore {\n\treturn &PingStore{querier: dbgen.New(db)}\n}\n\n' \
		'func (store *PingStore) Ping(ctx context.Context) error {\n\tvalue, err := store.querier.Ping(ctx)\n\tif err != nil {\n\t\treturn err\n\t}\n\tif value == 1 {\n\t\treturn nil\n\t}\n\treturn fmt.Errorf("platform store ping: unexpected value %d", value)\n}\n' >"$fixture/internal/platform/store/ping.go"
	printf '%b' 'package platformstore\n\nimport (\n\t"context"\n\t"errors"\n\t"testing"\n)\n\ntype fixtureQuerier struct { value int64; err error }\nfunc (querier fixtureQuerier) Ping(context.Context) (int64, error) { return querier.value, querier.err }\n\nfunc TestPing(t *testing.T) {\n\tif NewPingStore(nil) == nil { t.Fatal("nil store") }\n\tif err := (&PingStore{querier: fixtureQuerier{value: 1}}).Ping(context.Background()); err != nil { t.Fatal(err) }\n\tsentinel := errors.New("sentinel")\n\tif err := (&PingStore{querier: fixtureQuerier{err: sentinel}}).Ping(context.Background()); !errors.Is(err, sentinel) { t.Fatal(err) }\n\tif err := (&PingStore{querier: fixtureQuerier{}}).Ping(context.Background()); err == nil || err.Error() != "platform store ping: unexpected value 0" { t.Fatalf("unexpected error: %v", err) }\n}\n' >"$fixture/internal/platform/store/ping_test.go"
	gofmt -w "$fixture/internal/platform/store/ping.go" "$fixture/internal/platform/store/ping_test.go"
	chmod 755 "$fixture/internal" "$fixture/internal/platform" "$fixture/internal/platform/store" "$fixture/internal/platform/store/generated" "$fixture/internal/platform/store/queries"; chmod 644 "$fixture/internal/platform/store/contract.go" "$fixture/internal/platform/store/queries/health.sql" "$fixture/internal/platform/store/ping.go" "$fixture/internal/platform/store/ping_test.go" "$fixture/internal/platform/store/generated/"*.go
	[[ ! -e "$fixture/.git" && ! -L "$fixture/.git" ]] || fail "fixture unexpectedly has .git: $1"
	printf '%s\n' "$fixture"
}

run_completion() {
	local fixture="$1" output
	if [[ ! -e "$fixture/internal/platform/store/ping.go" && ! -L "$fixture/internal/platform/store/ping.go" && ! -e "$fixture/internal/platform/store/ping_test.go" && ! -L "$fixture/internal/platform/store/ping_test.go" ]]; then echo PENDING; return 0; fi
	"$fixture/acceptance/p0s03/static_contract.sh" || return
	output="$(cd "$fixture" && GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off go test -cover ./internal/platform/store 2>&1)" || { printf '%s\n' "$output"; return 1; }
	printf '%s\n' "$output"
	grep -Eq 'coverage: 100(\.0)?% of statements' <<<"$output" || fail "package coverage must be 100%"
	echo "p0-s03-completion: PASS"
}

expect_rejected() {
	local name="$1" want="$2" fixture output status
	fixture="$(make_fixture "$name")"; shift 2; "$@" "$fixture"
	set +e; output="$(run_completion "$fixture" 2>&1)"; status=$?; set -e
	[[ "$status" -ne 0 ]] || fail "$name was accepted"
	grep -Fq "$want" <<<"$output" || fail "$name rejected for the wrong reason: $output"
	! grep -Eq '^PENDING$|p0-s03-completion: PASS' <<<"$output" || fail "$name reached a false completion state"
}

positive="$(make_fixture positive)"; run_completion "$positive"
gnu_stat_fixture="$(make_fixture gnu-stat)"; mkdir "$gnu_stat_fixture/bin"; printf '%b' '#!/usr/bin/env bash\nif [[ "$1" = -f && "$2" = %p ]]; then echo "filesystem metadata"; exit 0; fi\nif [[ "$1" = -c && "$2" = %a ]]; then if [[ -d "$3" ]]; then echo 755; else echo 644; fi; exit 0; fi\nexit 2\n' >"$gnu_stat_fixture/bin/stat"
chmod 755 "$gnu_stat_fixture/bin/stat"; PATH="$gnu_stat_fixture/bin:$PATH" run_completion "$gnu_stat_fixture"
remove_ping() { rm -f "$1/internal/platform/store/ping.go"; }; make_symlink() { rm -f "$1/internal/platform/store/ping.go"; ln -s ping_test.go "$1/internal/platform/store/ping.go"; }
make_owner_executable() { chmod u+x "$1/internal/platform/store/ping.go"; }; make_group_executable() { chmod g+x "$1/internal/platform/store/ping.go"; }; make_other_executable() { chmod o+x "$1/internal/platform/store/ping.go"; }; make_setuid() { chmod u+s "$1/internal/platform/store/ping.go"; }
make_internal_symlink() { mv "$1/internal" "$1/internal-real"; ln -s internal-real "$1/internal"; }; make_platform_symlink() { mv "$1/internal/platform" "$1/internal/platform-real"; ln -s platform-real "$1/internal/platform"; }
make_store_symlink() { mv "$1/internal/platform/store" "$1/internal/platform/store-real"; ln -s store-real "$1/internal/platform/store"; }; make_generated_symlink() { mv "$1/internal/platform/store/generated" "$1/internal/platform/store/generated-real"; ln -s generated-real "$1/internal/platform/store/generated"; }; make_queries_symlink() { mv "$1/internal/platform/store/queries" "$1/internal/platform/store/queries-real"; ln -s queries-real "$1/internal/platform/store/queries"; }
add_generated_entry() { printf x >"$1/internal/platform/store/generated/extra.txt"; }; make_generated_child_symlink() { rm "$1/internal/platform/store/generated/db.go"; ln -s querier.go "$1/internal/platform/store/generated/db.go"; }; make_generated_fifo() { rm "$1/internal/platform/store/generated/health.sql.go"; mkfifo "$1/internal/platform/store/generated/health.sql.go"; }; make_generated_special() { chmod u+s "$1/internal/platform/store/generated/db.go"; }; make_generated_dir_writable() { chmod 777 "$1/internal/platform/store/generated"; }; make_health_private() { chmod 600 "$1/internal/platform/store/queries/health.sql"; }
make_query_symlink() { rm "$1/internal/platform/store/queries/health.sql"; ln -s ../contract.go "$1/internal/platform/store/queries/health.sql"; }; make_query_special() { rm "$1/internal/platform/store/queries/health.sql"; mkfifo "$1/internal/platform/store/queries/health.sql"; }; add_query_entry() { printf '%s\n' 'SELECT 2;' >"$1/internal/platform/store/queries/extra.sql"; }
add_extra_file() { printf '%s\n' 'package platformstore' >"$1/internal/platform/store/extra.go"; }; exceed_line_limit() { for _ in $(seq 1 200); do printf '%s\n' '// padding' >>"$1/internal/platform/store/ping_test.go"; done; }
append_sql() { printf '\n%s\n' 'const unsafeSQL = "SELECT 1"' >>"$1/internal/platform/store/ping.go"; }; append_split_sql() { printf '\n%s\n' 'const unsafeSQL = "SEL" + "ECT 1"' >>"$1/internal/platform/store/ping.go"; }
inject_local_strings() { local file="$1/internal/platform/store/ping.go"; awk '{ print; if ($0 == "func (store *PingStore) Ping(ctx context.Context) error {") print "\tif false {\n\t\tconst prefix = \"SEL\"\n\t\tvar suffix = \"ECT\"\n\t\t_ = prefix + suffix\n\t}" }' "$file" >"$file.tmp"; mv "$file.tmp" "$file"; gofmt -w "$file"; }
append_query_row() { printf '%b' '\nvar _ = db.QueryRow(\n\tctx,\n\t"value",\n)\n' >>"$1/internal/platform/store/ping.go"; }; append_query_exec() { printf '%b' '\nvar _, _ = db.Query(ctx, "value")\nvar _, _ = db.Exec(ctx, "value")\n' >>"$1/internal/platform/store/ping.go"; }
append_environment() { printf '\n%s\n' 'var _ = os.Getenv("P0S03")' >>"$1/internal/platform/store/ping.go"; }; append_connection() { printf '\n%s\n' 'var _ = pgx.Connect(ctx, "")' >>"$1/internal/platform/store/ping.go"; }
inject_paren_new() { local file="$1/internal/platform/store/ping.go"; awk '{ print; if ($0 == "func NewPingStore(db dbgen.DBTX) *PingStore {") print "\tif false { _ = dbgen.New((db)) }" }' "$file" >"$file.tmp"; mv "$file.tmp" "$file"; gofmt -w "$file"; }; inject_paren_ping() { local file="$1/internal/platform/store/ping.go"; awk '{ print; if ($0 == "func (store *PingStore) Ping(ctx context.Context) error {") print "\tif false { _, _ = store.querier.Ping((ctx)) }" }' "$file" >"$file.tmp"; mv "$file.tmp" "$file"; gofmt -w "$file"; }
inject_ping_method_value() { local file="$1/internal/platform/store/ping.go"; awk '{ print; if ($0 == "func (store *PingStore) Ping(ctx context.Context) error {") print "\t_ = store.querier.Ping" }' "$file" >"$file.tmp"; mv "$file.tmp" "$file"; gofmt -w "$file"; }; inject_func_lit_value() { local file="$1/internal/platform/store/ping.go"; awk '{ print; if ($0 == "func (store *PingStore) Ping(ctx context.Context) error {") print "\t_ = func() {}" }' "$file" >"$file.tmp"; mv "$file.tmp" "$file"; gofmt -w "$file"; }
append_export() { printf '\n%s\n' 'func Escape() {}' >>"$1/internal/platform/store/ping.go"; }; append_goroutine() { printf '\n%s\n' 'func unsafeGo() { go func() {}() }' >>"$1/internal/platform/store/ping.go"; gofmt -w "$1/internal/platform/store/ping.go"; }; append_defer() { printf '\n%s\n' 'func unsafeDefer() { defer fmt.Errorf("x") }' >>"$1/internal/platform/store/ping.go"; gofmt -w "$1/internal/platform/store/ping.go"; }
append_ident_call() { printf '\n%s\n' 'var _ = len("x")' >>"$1/internal/platform/store/ping.go"; }; append_func_lit_call() { printf '\n%s\n' 'var _ = func() int { return 0 }()' >>"$1/internal/platform/store/ping.go"; gofmt -w "$1/internal/platform/store/ping.go"; }; append_other_selector() { printf '\n%s\n' 'var _ = fmt.Println("dead")' >>"$1/internal/platform/store/ping.go"; }
inject_covered_non_one_bypass() {
	local implementation="$1/internal/platform/store/ping.go" tests="$1/internal/platform/store/ping_test.go"
	awk '
		$0 == "\treturn fmt.Errorf(\"platform store ping: unexpected value %d\", value)" {
			matches++
			print "\tif value == 42 {"
			print "\t\treturn nil"
			print "\t}"
		}
		{ print }
		END { exit !(matches == 1) }
	' "$implementation" >"$implementation.tmp" || fail "cannot construct covered non-one bypass"
	mv "$implementation.tmp" "$implementation"
	cat >>"$tests" <<'EOF'

func TestCoveredNonOneBypass(t *testing.T) {
	if err := (&PingStore{querier: fixtureQuerier{value: 42}}).Ping(context.Background()); err != nil {
		t.Fatalf("Ping(42) error = %v, want nil", err)
	}
}
EOF
	gofmt -w "$implementation" "$tests"
}
add_unsafe_test_import() { printf '%b' 'package platformstore\n\nimport "os"\n\nfunc TestUnsafe(t *testing.T) { _ = os.Getenv("x") }\n' >"$1/internal/platform/store/ping_test.go"; gofmt -w "$1/internal/platform/store/ping_test.go"; }; append_test_after_func() { printf '\n%s\n' 'var _ = context.AfterFunc(context.Background(), func() {})' >>"$1/internal/platform/store/ping_test.go"; gofmt -w "$1/internal/platform/store/ping_test.go"; }
append_test_go() { printf '\n%s\n' 'func TestUnsafeGo(t *testing.T) { go func() {}() }' >>"$1/internal/platform/store/ping_test.go"; gofmt -w "$1/internal/platform/store/ping_test.go"; }; append_test_defer() { printf '\n%s\n' 'func TestUnsafeDefer(t *testing.T) { defer context.AfterFunc(context.Background(), func() {})() }' >>"$1/internal/platform/store/ping_test.go"; gofmt -w "$1/internal/platform/store/ping_test.go"; }
append_test_method() { printf '\nfunc TestUnsafeMethod(t *testing.T) { %s }\n' "$2" >>"$1/internal/platform/store/ping_test.go"; gofmt -w "$1/internal/platform/store/ping_test.go"; }; test_setenv() { append_test_method "$1" 't.Setenv("K", "V")'; }; test_tempdir() { append_test_method "$1" 't.TempDir()'; }; test_chdir() { append_test_method "$1" 't.Chdir(".")'; }; test_parallel() { append_test_method "$1" 't.Parallel()'; }
empty_test() { : >"$1/internal/platform/store/ping_test.go"; }; remove_test_function() { printf '%s\n' 'package platformstore' >"$1/internal/platform/store/ping_test.go"; }; reduce_coverage() { printf '%b' 'package platformstore\n\nimport "testing"\nfunc TestPing(t *testing.T) { if NewPingStore(nil) == nil { t.Fatal("nil store") } }\n' >"$1/internal/platform/store/ping_test.go"; gofmt -w "$1/internal/platform/store/ping_test.go"; }
write_hostile_calls() { local path="$1/internal/platform/store/ping.go"; sed 's/\t"fmt"/\t"fmt"\n\t"log"\n\t"net"\n\t"net\/http"\n\t"os"\n\t"os\/exec"/' "$path" >"$path.tmp"; mv "$path.tmp" "$path"; awk '{ print; if ($0 == "func (store *PingStore) Ping(ctx context.Context) error {") print "\tif false { _, _ = net.Dial(\"\", \"\"); _ = http.ListenAndServe(\"\", nil); _ = os.WriteFile(\"\", nil, 0); _ = exec.Command(\"\"); log.Print(\"\"); fmt.Println(\"\") }" }' "$path" >"$path.tmp"; mv "$path.tmp" "$path"; gofmt -w "$path"; }

expect_rejected missing-file 'required regular file is missing' remove_ping; expect_rejected symlink 'required regular file is missing' make_symlink
expect_rejected owner-executable 'mode must be exactly 0644' make_owner_executable; expect_rejected group-executable 'mode must be exactly 0644' make_group_executable; expect_rejected other-executable 'mode must be exactly 0644' make_other_executable; expect_rejected setuid 'mode must be exactly 0644' make_setuid
expect_rejected internal-symlink 'required real directory is missing: internal' make_internal_symlink; expect_rejected platform-symlink 'required real directory is missing: internal/platform' make_platform_symlink; expect_rejected store-symlink 'required real directory is missing: internal/platform/store' make_store_symlink
expect_rejected generated-symlink 'required real directory is missing: internal/platform/store/generated' make_generated_symlink; expect_rejected queries-symlink 'required real directory is missing: internal/platform/store/queries' make_queries_symlink
expect_rejected generated-extra 'generated must contain exactly four frozen paths' add_generated_entry; expect_rejected generated-child-symlink 'required regular file is missing' make_generated_child_symlink; expect_rejected generated-fifo 'required regular file is missing' make_generated_fifo; expect_rejected generated-special 'mode must be exactly 0644' make_generated_special; expect_rejected generated-dir-mode 'directory mode must be exactly 0755' make_generated_dir_writable; expect_rejected health-mode 'mode must be exactly 0644' make_health_private
expect_rejected query-symlink 'required regular file is missing' make_query_symlink; expect_rejected query-special 'required regular file is missing' make_query_special; expect_rejected query-extra 'queries must contain only health.sql' add_query_entry; expect_rejected extra-file 'unexpected store top-level entry' add_extra_file
expect_rejected line-limit 'exceed 220 lines' exceed_line_limit; expect_rejected handwritten-sql 'implementation string literal is not allowed' append_sql; expect_rejected split-sql 'implementation string construction is forbidden' append_split_sql; expect_rejected local-split-sql 'implementation string literal is not allowed' inject_local_strings
expect_rejected direct-query-row 'implementation call is not allowed' append_query_row; expect_rejected direct-query-exec 'implementation call is not allowed' append_query_exec; expect_rejected hostile-calls 'import is not allowed' write_hostile_calls; expect_rejected environment 'implementation call is not allowed' append_environment; expect_rejected connection 'implementation call is not allowed' append_connection
expect_rejected paren-new 'implementation call is not allowed' inject_paren_new; expect_rejected paren-ping 'implementation call is not allowed' inject_paren_ping; expect_rejected ident-call 'implementation call is not allowed' append_ident_call; expect_rejected func-lit-call 'implementation call is not allowed' append_func_lit_call; expect_rejected selector-call 'implementation call is not allowed' append_other_selector
expect_rejected ping-method-value 'allowed implementation selectors must each appear exactly once' inject_ping_method_value; expect_rejected func-lit-value 'function literals are forbidden' inject_func_lit_value
expect_rejected covered-non-one-bypass 'Ping must contain exactly the canonical query/error/success/failure control flow' inject_covered_non_one_bypass
expect_rejected test-import 'import is not allowed' add_unsafe_test_import; expect_rejected extra-export 'unexpected implementation declaration' append_export; expect_rejected go-statement 'go and defer statements are forbidden' append_goroutine; expect_rejected defer-statement 'go and defer statements are forbidden' append_defer
expect_rejected test-after-func 'test package call is not allowed' append_test_after_func; expect_rejected test-go 'go and defer statements are forbidden' append_test_go; expect_rejected test-defer 'go and defer statements are forbidden' append_test_defer
expect_rejected test-setenv 'test method is not allowed: Setenv' test_setenv; expect_rejected test-tempdir 'test method is not allowed: TempDir' test_tempdir; expect_rejected test-chdir 'test method is not allowed: Chdir' test_chdir; expect_rejected test-parallel 'test method is not allowed: Parallel' test_parallel
expect_rejected empty-test 'ping_test.go must not be empty' empty_test; expect_rejected no-test-function 'at least one Test function is required' remove_test_function; expect_rejected partial-coverage 'package coverage must be 100%' reduce_coverage
echo "p0-s03-contract-tests: PASS"
