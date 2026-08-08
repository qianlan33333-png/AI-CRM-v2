#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'p0-s04-static-contract-tests: %s\n' "$*" >&2; exit 1; }
script_path="${BASH_SOURCE[0]:-}"
case "$script_path" in */*) script_parent="${script_path%/*}" ;; *) script_parent='.' ;; esac
script_dir="$(CDPATH= cd -- "$script_parent" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
static_source="$script_dir/static_contract.sh"
checker_source="$script_dir/source_contract.go"
contract_source="$repo_root/internal/platform/river/contract.go"
module_source="$repo_root/go.mod"
for source in "$static_source" "$checker_source" "$contract_source" "$module_source"; do
	[[ -f "$source" && ! -L "$source" ]] || fail "missing fixture source: $source"
done

absolute_tool() {
	local path
	path="$(type -P "$1" || true)"
	[[ "$path" == /* && -x "$path" ]] || fail "trusted tool is unavailable: $1"
	printf '%s\n' "$path"
}
gofmt_bin="$(absolute_tool gofmt)"
stat_real="$(absolute_tool stat)"
rm_bin="$(absolute_tool rm)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/aicrm-v2-p0-s04-static.XXXXXX")" || fail 'mktemp failed'
cleanup() { [[ -n "${test_root:-}" && -d "$test_root" ]] && "$rm_bin" -rf "$test_root"; }
trap cleanup EXIT

write_runtime() {
	printf '%s\n' \
		'package platformriver' '' 'import (' '"context"' \
		'"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"' ')' '' \
		'type Runtime struct {' 'lifecycle Lifecycle' '}' '' \
		'func NewRuntime(lifecycle Lifecycle) *Runtime {' 'return &Runtime{lifecycle: lifecycle}' '}' '' \
		'func (r *Runtime) Run(parent context.Context) error {' \
		'if err := r.lifecycle.Start(context.WithoutCancel(parent)); err != nil {' 'return err' '}' \
		'select {' 'case <-parent.Done():' 'return r.stop(parent)' 'case <-r.lifecycle.Stopped():' \
		'select {' 'case <-parent.Done():' 'return r.stop(parent)' 'default:' 'return runtime.ErrUnexpectedStop' '}' '}' '}' '' \
		'func (r *Runtime) stop(parent context.Context) error {' \
		'shutdown, cancel := context.WithTimeout(context.WithoutCancel(parent), runtime.ShutdownGrace)' \
		'defer cancel()' 'return r.lifecycle.Stop(shutdown)' '}' >"$1"
}
write_migrate() {
	printf '%s\n' \
		'package platformriver' '' 'import (' '"context"' '"github.com/jackc/pgx/v5/pgxpool"' \
		'"github.com/riverqueue/river/riverdriver/riverpgxv5"' '"github.com/riverqueue/river/rivermigrate"' ')' '' \
		'type invalidDirectionError Direction' '' \
		'func (direction invalidDirectionError) Error() string {' 'return `platform river migration: invalid direction "` + string(direction) + `"`' '}' '' \
		'func (direction invalidDirectionError) Unwrap() error { return ErrInvalidDirection }' '' \
		'func Migrate(ctx context.Context, pool *pgxpool.Pool, direction Direction, options *MigrateOptions) error {' \
		'if direction != DirectionUp && direction != DirectionDown {' 'return invalidDirectionError(direction)' '}' \
		'migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)' 'if err != nil {' 'return err' '}' \
		'riverDirection := rivermigrate.DirectionUp' 'if direction == DirectionDown {' 'riverDirection = rivermigrate.DirectionDown' '}' \
		'var riverOptions *rivermigrate.MigrateOpts' 'if options != nil {' 'riverOptions = &rivermigrate.MigrateOpts{TargetVersion: options.TargetVersion}' '}' \
		'_, err = migrator.Migrate(ctx, riverDirection, riverOptions)' 'return err' '}' >"$1"
}
write_test() { printf '%s\n' 'package platformriver' '' 'import "testing"' '' 'func TestRuntime(t *testing.T) {}' >"$1"; }
candidate_path() { printf '%s/internal/platform/river/%s\n' "$1" "$2"; }
format_candidates() { "$gofmt_bin" -w "$(candidate_path "$1" runtime.go)" "$(candidate_path "$1" migrate.go)" "$(candidate_path "$1" runtime_test.go)"; }
parse_candidates() {
	local status
	set +e; "$gofmt_bin" -d "$(candidate_path "$1" runtime.go)" "$(candidate_path "$1" migrate.go)" "$(candidate_path "$1" runtime_test.go)" >/dev/null 2>&1; status=$?; set -e
	(( status == 0 ))
}
make_fixture() {
	local fixture="$test_root/$1"
	mkdir -p "$fixture/acceptance/p0s04" "$fixture/internal/platform/river" "$fixture/targets"
	cp "$static_source" "$fixture/acceptance/p0s04/static_contract.sh"
	cp "$checker_source" "$fixture/acceptance/p0s04/source_contract.go"
	cp "$contract_source" "$fixture/internal/platform/river/contract.go"
	cp "$module_source" "$fixture/go.mod"
	chmod 755 "$fixture/acceptance/p0s04/static_contract.sh"
	chmod 644 "$fixture/acceptance/p0s04/source_contract.go" "$fixture/internal/platform/river/contract.go" "$fixture/go.mod"
	cmp -s "$static_source" "$fixture/acceptance/p0s04/static_contract.sh" || fail 'static copy changed bytes'
	cmp -s "$checker_source" "$fixture/acceptance/p0s04/source_contract.go" || fail 'checker copy changed bytes'
	cmp -s "$contract_source" "$fixture/internal/platform/river/contract.go" || fail 'contract copy changed bytes'
	cmp -s "$module_source" "$fixture/go.mod" || fail 'go.mod copy changed bytes'
	write_runtime "$(candidate_path "$fixture" runtime.go)"
	write_migrate "$(candidate_path "$fixture" migrate.go)"
	write_test "$(candidate_path "$fixture" runtime_test.go)"
	format_candidates "$fixture" || fail 'positive fixture cannot format'
	[[ ! -e "$fixture/.git" && ! -L "$fixture/.git" ]] || fail 'fixture contains .git'
	printf '%s\n' "$fixture"
}
run_static() { ( cd / && /bin/bash "$1/acceptance/p0s04/static_contract.sh" ); }
writer_pid=''
link_path() { local fixture="$1" target="$2" stash="$fixture/targets/${2##*/}.real"; mv "$target" "$stash"; ln -s "$stash" "$target"; }
path_bad() {
	local fixture="$1" kind="$2" target="$fixture/$3"
	case "$kind" in
		missing) "$rm_bin" -f "$target" ;;
		link) link_path "$fixture" "$target" ;;
		fifo) "$rm_bin" -f "$target"; mkfifo "$target" ;;
		directory) "$rm_bin" -f "$target"; mkdir "$target" ;;
		mode600) chmod 600 "$target" ;;
		mode755) chmod 755 "$target" ;;
		setuid) chmod 4644 "$target" ;;
		*) fail "unknown path mutation: $kind" ;;
	esac
}
ancestor_bad() {
	local fixture="$1" kind="$2" name="$3" target
	case "$name" in internal) target="$fixture/internal" ;; platform) target="$fixture/internal/platform" ;; river) target="$fixture/internal/platform/river" ;; *) fail 'unknown ancestor' ;; esac
	case "$kind" in link) link_path "$fixture" "$target" ;; mode) chmod 700 "$target" ;; *) fail 'unknown ancestor mutation' ;; esac
}
implementation_bad() {
	local fixture="$1" kind="$2" file="$3" target
	target="$(candidate_path "$fixture" "$file")"
	case "$kind" in none) "$rm_bin" -f "$(candidate_path "$fixture" runtime.go)" "$(candidate_path "$fixture" migrate.go)" "$(candidate_path "$fixture" runtime_test.go)" ;;
		one) "$rm_bin" -f "$(candidate_path "$fixture" migrate.go)" "$(candidate_path "$fixture" runtime_test.go)" ;;
		two) "$rm_bin" -f "$(candidate_path "$fixture" runtime_test.go)" ;;
		*) path_bad "$fixture" "$kind" "internal/platform/river/$file" ;;
	esac
}
extra_bad() {
	local river="$1/internal/platform/river"
	case "$2" in plain) : >"$river/extra.go" ;; hidden) : >"$river/.hidden" ;; subdir) mkdir "$river/nested" ;; fifo) mkfifo "$river/extra-fifo" ;; *) fail 'unknown inventory mutation' ;; esac
}
content_bad() {
	local fixture="$1" kind="$2" target current padding i
	target="$(candidate_path "$fixture" runtime_test.go)"
	case "$kind" in
		package) awk 'NR == 1 { print "package other"; next } { print }' "$target" >"$target.new"; mv "$target.new" "$target" ;;
		multiple) printf '\npackage platformriver\n' >>"$target" ;;
		gofmt) awk '{ if ($0 == "func TestRuntime(t *testing.T) {}") print "func TestRuntime(t *testing.T){}"; else print }' "$target" >"$target.new"; mv "$target.new" "$target" ;;
		lines)
			current=$(( $(awk 'END { print NR + 0 }' "$(candidate_path "$fixture" runtime.go)") + $(awk 'END { print NR + 0 }' "$(candidate_path "$fixture" migrate.go)") ))
			padding=$((321 - current - 6)); (( padding >= 0 )) || fail 'line fixture is unexpectedly large'
			{
				printf '%s\n' 'package platformriver' '' 'import "testing"' '' 'func TestRuntime(t *testing.T) {'
				for ((i = 0; i < padding; i++)); do printf '\t// padding\n'; done
				printf '}\n'
			} >"$target"
			"$gofmt_bin" -w "$target"
			;;
		*) fail 'unknown content mutation' ;;
	esac
}
source_only_bad() {
	local target
	target="$(candidate_path "$1" runtime.go)"
	awk '
		{
			marker = "r.lifecycle.Start(context.WithoutCancel(parent))"
			position = index($0, marker)
			if (position) { count++; print substr($0, 1, position - 1) "r.lifecycle.Start(parent)" substr($0, position + length(marker)); next }
			print
		}
		END { if (count != 1) exit 1 }
	' "$target" >"$target.new"
	mv "$target.new" "$target"
}
checker_reject_all() {
	local target="$1/acceptance/p0s04/source_contract.go"
	printf '%s\n' 'package main' '' 'func main() { panic("reject") }' >"$target"
	chmod 644 "$target"
}
static_bad() {
	local fixture="$1" kind="$2" target="$fixture/acceptance/p0s04/static_contract.sh"
	case "$kind" in link) mv "$target" "$target.real"; ln -s "$target.real" "$target" ;;
		fifo) mv "$target" "$target.real"; mkfifo "$target"; (cp "$target.real" "$target") & writer_pid=$! ;;
		mode) chmod 644 "$target" ;;
		*) fail 'unknown static mutation' ;;
	esac
}
assert_failure() {
	local name="$1" expected="$2" fixture="$3" output code
	set +e; output="$(run_static "$fixture" 2>&1)"; code=$?; set -e
	[[ -z "$writer_pid" ]] || wait "$writer_pid" || true
	[[ "$code" -ne 0 ]] || fail "accepted negative: $name"
	[[ "$output" == "p0-s04-static: $expected"* && "$output" != *$'\n'* ]] || fail "wrong diagnostic for $name: $output"
	case "$output" in *PASS*|*PENDING*) fail "non-failure marker for $name: $output" ;; esac
}
negative() {
	local name="$1" expected="$2" parse="$3" action="$4" fixture
	shift 4; writer_pid=''; fixture="$(make_fixture "$name")" || fail "cannot create fixture: $name"
	"$action" "$fixture" "$@"
	[[ "$parse" != yes ]] || parse_candidates "$fixture" || fail "negative cannot parse: $name"
	assert_failure "$name" "$expected" "$fixture"
}
positive() {
	local fixture output
	fixture="$(make_fixture positive)" || fail 'cannot create positive fixture'
	output="$(run_static "$fixture" 2>&1)" || fail "positive rejected: $output"
	[[ "$output" == 'p0-s04-static: PASS' ]] || fail "positive output: $output"
}
make_safe_path() {
	local safe="$test_root/no-git-bin" tool path
	mkdir -p "$safe"
	for tool in awk find go gofmt readlink; do path="$(absolute_tool "$tool")"; ln -s "$path" "$safe/$tool"; done
	if "$stat_real" -c '%a' "$static_source" >/dev/null 2>&1; then
		printf '%s\n' '#!/bin/sh' 'if [ "$1" = "-f" ]; then exit 64; fi' 'if [ "$1" = "-c" ] && [ "$2" = "%a" ]; then' '  shift 2' "  exec \"$stat_real\" -c '%a' \"\$@\"" 'fi' 'exit 64' >"$safe/stat"
	else
		printf '%s\n' '#!/bin/sh' 'if [ "$1" = "-f" ]; then exit 64; fi' 'if [ "$1" = "-c" ] && [ "$2" = "%a" ]; then' '  shift 2' "  exec \"$stat_real\" -f '%Lp' \"\$@\"" 'fi' 'exit 64' >"$safe/stat"
	fi
	chmod 755 "$safe/stat"; printf '%s\n' "$safe"
}

positive
for ancestor in internal platform river; do
	negative "ancestor-$ancestor-link" 'directory required' yes ancestor_bad link "$ancestor"
	negative "ancestor-$ancestor-mode" 'mode must be exactly 0755' yes ancestor_bad mode "$ancestor"
done
for file in runtime.go migrate.go runtime_test.go; do
	negative "missing-$file" 'regular file required' no implementation_bad missing "$file"
	for kind in link fifo directory mode600 mode755 setuid; do
		parse_case=no; case "$kind" in link|mode600|mode755|setuid) parse_case=yes ;; esac
		expected='regular file required'; case "$kind" in mode*) expected='mode must be exactly 0644' ;; setuid) expected='mode must be exactly 0644' ;; esac
		negative "$file-$kind" "$expected" "$parse_case" implementation_bad "$kind" "$file"
	done
done
negative only-one 'regular file required' no implementation_bad one runtime.go
negative only-two 'regular file required' no implementation_bad two runtime.go
negative all-missing 'regular file required' no implementation_bad none runtime.go
for kind in missing link fifo mode600; do
	expected='regular file required'; [[ "$kind" == mode600 ]] && expected='mode must be exactly 0644'
	negative "contract-$kind" "$expected" yes path_bad "$kind" internal/platform/river/contract.go
	negative "checker-$kind" "$expected" yes path_bad "$kind" acceptance/p0s04/source_contract.go
done
for kind in plain hidden subdir fifo; do negative "inventory-$kind" 'river inventory contains unexpected entry' yes extra_bad "$kind"; done
negative lines-321 'river implementation exceeds 320 lines' yes content_bad lines
negative wrong-package 'package must be exactly platformriver' yes content_bad package
negative multiple-package 'package must be exactly platformriver' no content_bad multiple
negative gofmt-diff 'gofmt failed' no content_bad gofmt
negative source-contract-start-parent 'source contract rejected' yes source_only_bad
negative checker-reject-all 'source contract rejected' yes checker_reject_all
for kind in link fifo mode; do
	expected='regular file required'; [[ "$kind" == mode ]] && expected='mode must be exactly 0755'
	negative "static-$kind" "$expected" yes static_bad "$kind"
done
safe_path="$(make_safe_path)"
hostile_fixture="$(make_fixture hostile)" || fail 'cannot create hostile fixture'
set +e
hostile_output="$(PATH="$safe_path"; export PATH; [[ ! -e "$safe_path/git" && ! -L "$safe_path/git" ]] || exit 71; GIT_DIR=/no-git GIT_WORK_TREE=/no-worktree GIT_INDEX_FILE=/no-index GIT_CONFIG_GLOBAL=/no-config GIT_CONFIG_NOSYSTEM=1 GOWORK=/invalid GOTOOLCHAIN=auto GOFLAGS='-mod=mod -x' GOPROXY=https://invalid.invalid GOSUMDB=sum.golang.org run_static "$hostile_fixture")"
hostile_code=$?
set -e
[[ "$hostile_code" -eq 0 && "$hostile_output" == 'p0-s04-static: PASS' ]] || fail "hostile GNU-stat no-Git positive failed: $hostile_output"
printf '%s\n' 'p0-s04-static-contract-tests: PASS'
