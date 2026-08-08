#!/usr/bin/env bash
set -euo pipefail

script_path="${BASH_SOURCE[0]}"
case "$script_path" in */*) script_parent="${script_path%/*}" ;; *) script_parent='.' ;; esac
script_dir="$(CDPATH= cd -- "$script_parent" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
source_checker="$script_dir/source_contract.go"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-v2-p0-s04-source.XXXXXX")"
cleanup() { [[ -n "${test_root:-}" && -d "$test_root" ]] && rm -rf -- "$test_root"; }
trap cleanup EXIT
fail() { printf 'p0-s04-source-contract: %s\n' "$*" >&2; exit 1; }
[[ "$#" -eq 0 ]] || fail "unexpected argument"
readlink_bin="$(type -P readlink || true)"
[[ "$readlink_bin" == /* && -x "$readlink_bin" ]] || fail "trusted readlink is unavailable"
canonical_tool() {
	local tool="$1" target link target_dir link_dir
	target="$(type -P "$tool" || true)"
	[[ "$target" == /* && -x "$target" ]] || fail "trusted tool is unavailable: $tool"
	while [[ -L "$target" ]]; do
		link="$("$readlink_bin" "$target")" || fail "cannot resolve Go"
		target_dir="${target%/*}"
		case "$link" in
		/*) target="$link" ;;
		*/*) link_dir="${link%/*}"; target="$(CDPATH= cd -- "$target_dir/$link_dir" && pwd -P)/${link##*/}" ;;
		*) target="$target_dir/$link" ;;
		esac
	done
	[[ "$target" == /* && -f "$target" && ! -L "$target" && -x "$target" ]] || fail "trusted tool is invalid: $tool"
	printf '%s\n' "$target"
}
go_bin="$(canonical_tool go)"
gofmt_bin="$(canonical_tool gofmt)"
make_safe_path() {
	local safe="$test_root/no-git-bin" tool tool_path
	mkdir -p "$safe"
	for tool in awk chmod cp go gofmt grep ln mkdir mkfifo mktemp mv readlink rm stat; do
		tool_path="$(type -P "$tool" || true)"
		[[ "$tool_path" == /* && -x "$tool_path" ]] || fail "trusted tool is unavailable: $tool"
		ln -s "$tool_path" "$safe/$tool"
	done
	printf '%s\n' "$safe"
}
safe_path="$(make_safe_path)"
mode_of() {
	local value
	value="$(stat -f '%p' "$1" 2>/dev/null || true)"
	if [[ "$value" =~ ^[0-7]+$ ]]; then printf '%s\n' "$value"; return; fi
	value="$(stat -c '%a' "$1" 2>/dev/null || true)"
	[[ "$value" =~ ^[0-7]+$ ]] || return 1
	printf '%s\n' "$value"
}
check_regular() {
	local target="$1" expected="$2" actual
	if [[ ! -f "$target" || -L "$target" ]]; then
		printf 'p0-s04-source-contract: regular path required: %s\n' "$target" >&2
		return 1
	fi
	actual="$(mode_of "$target")" || { printf 'p0-s04-source-contract: cannot read mode: %s\n' "$target" >&2; return 1; }
	if (( (8#$actual & 07777) != 8#$expected )); then
		printf 'p0-s04-source-contract: mode must be exactly %s: %s\n' "$expected" "$target" >&2
		return 1
	fi
}

check_regular "$script_path" 0755 || exit 1
check_regular "$source_checker" 0644 || exit 1
[[ -f "$repo_root/go.mod" && ! -L "$repo_root/go.mod" ]] || fail "invalid repository root"

write_runtime() {
	printf '%s\n' \
		'package platformriver' '' 'import (' '"context"' '// runtime-import-marker' '"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"' ')' '' \
		'type Runtime struct {' 'lifecycle Lifecycle' '}' '// runtime-declaration-marker' '' \
		'func NewRuntime(lifecycle Lifecycle) *Runtime {' '// constructor-extra-marker' 'return &Runtime{lifecycle: lifecycle}' '}' '' \
		'func (r *Runtime) Run(parent context.Context) error {' 'if err := r.lifecycle.Start(context.WithoutCancel(parent)); err != nil {' 'return err' '}' '// run-extra-marker' 'select {' 'case <-parent.Done():' 'return r.stop(parent)' 'case <-r.lifecycle.Stopped():' 'select {' 'case <-parent.Done(): // nested-parent-marker' 'return r.stop(parent)' '// nested-extra-marker' 'default:' 'return runtime.ErrUnexpectedStop' '}' '// outer-extra-marker' '}' '}' '' \
		'func (r *Runtime) stop(parent context.Context) error {' 'shutdown, cancel := context.WithTimeout(context.WithoutCancel(parent), runtime.ShutdownGrace)' 'defer cancel()' '// stop-extra-marker' 'return r.lifecycle.Stop(shutdown)' '}' >"$1"
}
write_migrate() {
	printf '%s\n' \
		'package platformriver' '' 'import (' '"context"' '// migrate-import-marker' '"github.com/jackc/pgx/v5/pgxpool"' '"github.com/riverqueue/river/riverdriver/riverpgxv5"' '"github.com/riverqueue/river/rivermigrate"' ')' '' \
		'type invalidDirectionError Direction' '// migrate-declaration-marker' '' \
		'func (direction invalidDirectionError) Error() string {' 'return `platform river migration: invalid direction "` + string(direction) + `"`' '}' '' \
		'func (direction invalidDirectionError) Unwrap() error { return ErrInvalidDirection }' '' \
		'func Migrate(ctx context.Context, pool *pgxpool.Pool, direction Direction, options *MigrateOptions) error {' '// guard-marker' 'if direction != DirectionUp && direction != DirectionDown {' 'return invalidDirectionError(direction)' '}' '// body-marker' 'migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil) // driver-dataflow-marker' 'if err != nil {' 'return err' '}' 'riverDirection := rivermigrate.DirectionUp' 'if direction == DirectionDown {' 'riverDirection = rivermigrate.DirectionDown' '}' 'var riverOptions *rivermigrate.MigrateOpts' 'if options != nil {' 'riverOptions = &rivermigrate.MigrateOpts{TargetVersion: options.TargetVersion}' '}' '_, err = migrator.Migrate(ctx, riverDirection, riverOptions) // migrator-dataflow-marker' 'return err' '}' >"$1"
}
write_test() { printf '%s\n' 'package platformriver' '' 'import "testing"' '' 'func TestRuntime(t *testing.T) {}' >"$1"; }
make_fixture() {
	local fixture="$test_root/$1"
	mkdir -p "$fixture/acceptance/p0s04" "$fixture/internal/platform/river"
	printf '%s\n' 'module fixture' '' 'go 1.26.5' >"$fixture/go.mod"
	cp "$source_checker" "$fixture/acceptance/p0s04/source_contract.go"
	cp "$script_path" "$fixture/acceptance/p0s04/test_source_contract.sh"
	chmod 644 "$fixture/acceptance/p0s04/source_contract.go"
	chmod 755 "$fixture/acceptance/p0s04/test_source_contract.sh"
	write_runtime "$fixture/internal/platform/river/runtime.go"
	write_migrate "$fixture/internal/platform/river/migrate.go"
	write_test "$fixture/internal/platform/river/runtime_test.go"
	[[ ! -e "$fixture/.git" && ! -L "$fixture/.git" ]] || fail "fixture contains .git: $1"
	printf '%s\n' "$fixture"
}
replace_once() {
	local target="$1" marker="$2" replacement="$3"
	awk -v marker="$marker" -v replacement="$replacement" '
		index($0, marker) { position = index($0, marker); if (++count > 1) exit 3; print substr($0, 1, position - 1) replacement substr($0, position + length(marker)); next }
		{ print }
		END { if (count != 1) exit 4 }
	' "$target" >"$target.new" || fail "replacement did not uniquely match: $marker"
	mv "$target.new" "$target"
}
run_checker() {
	local fixture="$1"
	(
		cd /
		unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES
		GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off \
			"$go_bin" run "$fixture/acceptance/p0s04/source_contract.go" -- \
			"$fixture/internal/platform/river/runtime.go" \
			"$fixture/internal/platform/river/migrate.go" \
			"$fixture/internal/platform/river/runtime_test.go" 2>&1
	)
}
parse_candidate() {
	"$gofmt_bin" -w "$1/internal/platform/river/runtime.go" "$1/internal/platform/river/migrate.go" "$1/internal/platform/river/runtime_test.go"
}
mutate() {
	local name="$1" fixture="$2" runtime="$2/internal/platform/river/runtime.go" migrate="$2/internal/platform/river/migrate.go" tests="$2/internal/platform/river/runtime_test.go"
	case "$name" in
	constructor_nil) replace_once "$runtime" 'return &Runtime{lifecycle: lifecycle}' 'return nil' ;;
	constructor_go) replace_once "$runtime" '// constructor-extra-marker' 'go func() {}()' ;;
	constructor_extra) replace_once "$runtime" '// constructor-extra-marker' '_ = lifecycle' ;;
	run_extra_if) replace_once "$runtime" '// run-extra-marker' 'if parent == nil { return nil }' ;;
	run_extra_ident) replace_once "$runtime" '// run-extra-marker' '_ = parent' ;;
	start_parent) replace_once "$runtime" 'r.lifecycle.Start(context.WithoutCancel(parent))' 'r.lifecycle.Start(parent)' ;;
	start_background) replace_once "$runtime" 'r.lifecycle.Start(context.WithoutCancel(parent))' 'r.lifecycle.Start(context.Background())' ;;
	outer_default) replace_once "$runtime" '// outer-extra-marker' 'default: return nil' ;;
	outer_extra) replace_once "$runtime" '// outer-extra-marker' 'case <-parent.Done(): return r.stop(parent)' ;;
	nested_extra) replace_once "$runtime" '// nested-extra-marker' 'case <-r.lifecycle.Stopped(): return runtime.ErrUnexpectedStop' ;;
	nested_missing) replace_once "$runtime" 'parent.Done(): // nested-parent-marker' 'r.lifecycle.Stopped():' ;;
	stop_parent) replace_once "$runtime" 'shutdown, cancel := context.WithTimeout(context.WithoutCancel(parent), runtime.ShutdownGrace)' 'shutdown, cancel := context.WithTimeout(parent, runtime.ShutdownGrace)' ;;
	stop_alias) replace_once "$runtime" 'shutdown, cancel := context.WithTimeout(context.WithoutCancel(parent), runtime.ShutdownGrace)' 'shutdown, alias := context.WithTimeout(context.WithoutCancel(parent), runtime.ShutdownGrace)' ;;
	stop_extra) replace_once "$runtime" '// stop-extra-marker' '_ = shutdown' ;;
	error_wrong) replace_once "$migrate" 'return `platform river migration: invalid direction "` + string(direction) + `"`' 'return "wrong"' ;;
	error_extra) replace_once "$migrate" 'return `platform river migration: invalid direction "` + string(direction) + `"`' '_ = direction; return `platform river migration: invalid direction "` + string(direction) + `"`' ;;
	guard_late) replace_once "$migrate" '// guard-marker' '_ = riverpgxv5.New(pool)' ;;
	global_sql) replace_once "$migrate" '// migrate-declaration-marker' 'const sql = "SELECT 1"' ;;
	global_split_sql) replace_once "$migrate" '// migrate-declaration-marker' 'const sql = "SEL" + "ECT 1"' ;;
	body_sql) replace_once "$migrate" '// body-marker' 'const sql = "SELECT 1"' ;;
	body_split_sql) replace_once "$migrate" '// body-marker' 'const sql = "SEL" + "ECT 1"' ;;
	pool_exec) replace_once "$migrate" '// body-marker' '_, _ = pool.Exec(nil, "")' ;;
	pool_query) replace_once "$migrate" '// body-marker' '_, _ = pool.Query(nil, "")' ;;
	river_internal) replace_once "$migrate" '// migrate-import-marker' '"github.com/riverqueue/river/internal/unsafe"' ;;
	alias_import) replace_once "$runtime" '"context"' 'ctx "context"' ;;
	dot_import) replace_once "$runtime" '"context"' '. "context"' ;;
	extra_export) replace_once "$runtime" '// runtime-declaration-marker' 'func Escape() {}' ;;
	private_var) replace_once "$runtime" '// runtime-declaration-marker' 'var hidden int' ;;
	private_type) replace_once "$runtime" '// runtime-declaration-marker' 'type hidden struct{}' ;;
	private_func) replace_once "$runtime" '// runtime-declaration-marker' 'func hidden() {}' ;;
	private_method) replace_once "$runtime" '// runtime-declaration-marker' 'func (*Runtime) hidden() {}' ;;
	go_stmt) replace_once "$migrate" '// body-marker' 'go func() {}()' ;;
	defer_stmt) replace_once "$migrate" '// body-marker' 'defer func() {}()' ;;
	run_migrate) replace_once "$runtime" '// run-extra-marker' 'Migrate(nil, nil, "", nil)' ;;
	extra_option) replace_once "$migrate" 'TargetVersion: options.TargetVersion' 'TargetVersion: options.TargetVersion, MaxSteps: 1' ;;
	driver_unpassed) replace_once "$migrate" 'migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil) // driver-dataflow-marker' 'driver := riverpgxv5.New(pool); _ = driver; migrator, err := rivermigrate.New(nil, nil)' ;;
	migrator_replaced) replace_once "$migrate" '_, err = migrator.Migrate(ctx, riverDirection, riverOptions) // migrator-dataflow-marker' 'migrator = nil; _, err = migrator.Migrate(ctx, riverDirection, riverOptions)' ;;
	wrong_down) replace_once "$migrate" 'riverDirection = rivermigrate.DirectionDown' 'riverDirection = rivermigrate.DirectionUp' ;;
	wrong_target) replace_once "$migrate" 'TargetVersion: options.TargetVersion' 'TargetVersion: 0' ;;
	empty_test) printf '%s\n' 'package platformriver' >"$tests" ;;
	*) fail "unknown mutation: $name" ;;
	esac
}
expected_bad() {
	case "$1" in
	constructor_nil) printf '%s\n' 'NewRuntime must allocate Runtime' ;;
	constructor_go|constructor_extra) printf '%s\n' 'NewRuntime must return one Runtime' ;;
	run_extra_if|run_extra_ident|run_migrate) printf '%s\n' 'Run must contain only Start guard and select' ;;
	start_parent|start_background) printf '%s\n' 'Start must use context.WithoutCancel(parent)' ;;
	outer_default|outer_extra) printf '%s\n' 'Run requires exact cancellation and Stopped select' ;;
	nested_extra|nested_missing) printf '%s\n' 'Stopped branch must prefer concurrent cancellation' ;;
	stop_parent) printf '%s\n' 'stop must use bounded live shutdown context' ;;
	stop_alias) printf '%s\n' 'stop must bind shutdown and cancel' ;;
	stop_extra) printf '%s\n' 'stop must have exactly three statements' ;;
	error_wrong) printf '%s\n' 'invalid direction Error text is not exact' ;;
	error_extra) printf '%s\n' 'invalid direction Error must have one exact return' ;;
	guard_late) printf '%s\n' 'invalid direction validation must precede driver or pool access' ;;
	global_sql|global_split_sql) printf '%s\n' 'unexpected package declaration' ;;
	body_sql|body_split_sql) printf '%s\n' 'Migrate string literals are forbidden' ;;
	pool_exec|pool_query) printf '%s\n' 'pool Exec/Query is forbidden' ;;
	river_internal) printf '%s\n' 'import is not allowed: "github.com/riverqueue/river/internal/unsafe"' ;;
	alias_import|dot_import) printf '%s\n' 'import aliases, dot imports, and blank imports are forbidden' ;;
	extra_export|private_var|private_type|private_func|private_method) printf '%s\n' 'unexpected package declaration' ;;
	go_stmt|defer_stmt) printf '%s\n' 'Migrate side effects are forbidden' ;;
	extra_option) printf '%s\n' 'only TargetVersion may be forwarded' ;;
	*) fail "missing expected diagnostic: $1" ;;
	esac
}
bad() {
	local name="$1" fixture output result_code expected
	fixture="$(make_fixture "$name")"; mutate "$name" "$fixture"
	parse_candidate "$fixture" || fail "negative mutation cannot parse: $name"
	expected="$(expected_bad "$name")"
	set +e; output="$(run_checker "$fixture" 2>&1)"; result_code=$?; set -e
	[[ "$result_code" -ne 0 ]] || fail "accepted negative case: $name"
	grep -Fq "p0-s04-source: $expected" <<<"$output" || fail "wrong rejection for $name (wanted $expected): $output"
}
bad_import() {
	local package="$1" fixture output result_code expected
	fixture="$(make_fixture "import-${package//\//_}")"
	replace_once "$fixture/internal/platform/river/runtime.go" '// runtime-import-marker' "\"$package\""
	parse_candidate "$fixture" || fail "forbidden-import mutation cannot parse: $package"
	expected="import is not allowed: \"$package\""
	set +e; output="$(run_checker "$fixture" 2>&1)"; result_code=$?; set -e
	[[ "$result_code" -ne 0 ]] || fail "accepted forbidden import: $package"
	grep -Fq "p0-s04-source: $expected" <<<"$output" || fail "wrong rejection for import $package: $output"
}
accepted() {
	local name="$1" fixture output
	fixture="$(make_fixture "delegated-$name")"; mutate "$name" "$fixture"
	parse_candidate "$fixture" || fail "delegated mutation cannot parse: $name"
	output="$(run_checker "$fixture")" || fail "delegated case rejected: $name"
	grep -Fqx 'p0-s04-source: PASS' <<<"$output" || fail "delegated case did not pass: $name"
}
self_bad() {
	local name="$1" fixture source test target output result_code expected writer_pid=''
	fixture="$(make_fixture "self-$name")"; source="$fixture/acceptance/p0s04/source_contract.go"; test="$fixture/acceptance/p0s04/test_source_contract.sh"; target="$test"
	case "$name" in
	source_symlink) mv "$source" "$source.real"; ln -s source_contract.go.real "$source" ;;
	source_fifo) rm -f "$source"; mkfifo "$source" ;;
	source_mode) chmod 600 "$source" ;;
	test_symlink) mv "$test" "$test.real"; ln -s test_source_contract.sh.real "$test" ;;
	test_fifo) mv "$test" "$test.real"; mkfifo "$test"; (cp "$test.real" "$test") & writer_pid=$! ;;
	test_mode) chmod 644 "$test" ;;
	*) fail "unknown self case: $name" ;;
	esac
	case "$name" in source_mode|test_mode) expected='mode must be exactly' ;; *) expected='regular path required' ;; esac
	set +e; output="$(/bin/bash "$target" 2>&1)"; result_code=$?; set -e
	[[ -z "$writer_pid" ]] || wait "$writer_pid" || true
	[[ "$result_code" -ne 0 ]] || fail "accepted invalid runner input: $name"
	grep -Fq "p0-s04-source-contract: $expected" <<<"$output" || fail "self-check did not reject $name: $output"
}

positive="$(make_fixture positive)"
parse_candidate "$positive" || fail 'positive fixture cannot parse'
run_checker "$positive" | grep -Fqx 'p0-s04-source: PASS'
hostile_fixture="$(make_fixture hostile-env)"
parse_candidate "$hostile_fixture" || fail 'hostile fixture cannot parse'
hostile_output="$(
	PATH="$safe_path"
	export PATH
	type -P git >/dev/null 2>&1 && exit 71
	GIT_DIR=/not-a-repository GIT_WORK_TREE=/not-a-worktree GIT_INDEX_FILE=/not-an-index GIT_CONFIG_GLOBAL=/not-a-config GIT_CONFIG_NOSYSTEM=1 GOFLAGS='-mod=mod -x' GOPROXY=https://invalid.invalid GOSUMDB=off run_checker "$hostile_fixture"
)" || fail 'hostile no-Git probe failed'
grep -Fqx 'p0-s04-source: PASS' <<<"$hostile_output" || fail 'hostile no-Git probe changed checker result'
for name in source_symlink source_fifo source_mode test_symlink test_fifo test_mode; do self_bad "$name"; done
for name in constructor_nil constructor_go constructor_extra run_extra_if run_extra_ident start_parent start_background outer_default outer_extra nested_extra nested_missing stop_parent stop_alias stop_extra error_wrong error_extra guard_late global_sql global_split_sql body_sql body_split_sql pool_exec pool_query river_internal alias_import dot_import extra_export private_var private_type private_func private_method go_stmt defer_stmt run_migrate extra_option; do bad "$name"; done
for package in os net os/exec log time fmt; do bad_import "$package"; done
for name in driver_unpassed migrator_replaced wrong_down wrong_target empty_test; do accepted "$name"; done
printf '%s\n' 'p0-s04-source-contract-tests: PASS' 'DELEGATED_C06B2_PG_BEHAVIOR: driver transfer, migrator replacement, Down, TargetVersion' 'DELEGATED_C06B2_COVERAGE: runtime_test coverage'
