#!/usr/bin/env bash
set -euo pipefail

prefix='p0-s04-static: '
fail() { printf '%s%s\n' "$prefix" "$*" >&2; exit 1; }

[[ "$#" -eq 0 ]] || fail 'unexpected argument'
script_path="${BASH_SOURCE[0]:-}"
[[ -f "$script_path" && ! -L "$script_path" ]] || fail 'regular file required: static_contract.sh'

readlink_bin="$(type -P readlink || true)"
[[ "$readlink_bin" == /* && -x "$readlink_bin" ]] || fail 'trusted readlink is unavailable'
canonical_tool() {
	local tool="$1" target link target_dir link_dir
	target="$(type -P "$tool" || true)"
	[[ "$target" == /* && -x "$target" ]] || fail "trusted tool is unavailable: $tool"
	while [[ -L "$target" ]]; do
		link="$("$readlink_bin" "$target")" || fail "cannot resolve tool: $tool"
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

stat_bin="$(canonical_tool stat)"
gofmt_bin="$(canonical_tool gofmt)"
go_bin="$(canonical_tool go)"
find_bin="$(canonical_tool find)"
awk_bin="$(canonical_tool awk)"
mode_of() {
	local value
	value="$("$stat_bin" -f '%p' "$1" 2>/dev/null || true)"
	if [[ "$value" =~ ^[0-7]+$ ]]; then printf '%s\n' "$value"; return; fi
	value="$("$stat_bin" -c '%a' "$1" 2>/dev/null || true)"
	[[ "$value" =~ ^[0-7]+$ ]] || return 1
	printf '%s\n' "$value"
}
check_directory() {
	local path="$1" label="$2" actual
	[[ -d "$path" && ! -L "$path" ]] || fail "directory required: $label"
	actual="$(mode_of "$path")" || fail "cannot read mode: $label"
	(( (8#$actual & 07777) == 8#0755 )) || fail "mode must be exactly 0755: $label"
}
check_regular() {
	local path="$1" expected="$2" label="$3" actual
	[[ -f "$path" && ! -L "$path" ]] || fail "regular file required: $label"
	actual="$(mode_of "$path")" || fail "cannot read mode: $label"
	(( (8#$actual & 07777) == 8#$expected )) || fail "mode must be exactly $expected: $label"
}

check_regular "$script_path" 0755 static_contract.sh
case "$script_path" in */*) script_parent="${script_path%/*}" ;; *) script_parent='.' ;; esac
script_dir="$(CDPATH= cd -- "$script_parent" && pwd -P)" || fail 'cannot locate static contract'
source_checker="$script_dir/source_contract.go"
check_regular "$source_checker" 0644 source_contract.go

repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)" || fail 'cannot locate repository root'
internal_dir="$repo_root/internal"
platform_dir="$internal_dir/platform"
river_dir="$platform_dir/river"
check_directory "$internal_dir" internal
check_directory "$platform_dir" internal/platform
check_directory "$river_dir" internal/platform/river

extra_entries="$("$find_bin" "$river_dir" -mindepth 1 -maxdepth 1 ! -name contract.go ! -name runtime.go ! -name migrate.go ! -name runtime_test.go -print 2>/dev/null)" || fail 'cannot inventory river directory'
[[ -z "$extra_entries" ]] || fail 'river inventory contains unexpected entry'
check_regular "$river_dir/contract.go" 0644 contract.go

candidates=("$river_dir/runtime.go" "$river_dir/migrate.go" "$river_dir/runtime_test.go")
total_lines=0
for candidate in "${candidates[@]}"; do
	label="${candidate##*/}"
	check_regular "$candidate" 0644 "$label"
	lines="$("$awk_bin" 'END { print NR + 0 }' "$candidate")" || fail "cannot count lines: $label"
	[[ "$lines" =~ ^[0-9]+$ ]] || fail "invalid line count: $label"
	total_lines=$((total_lines + lines))
	packages="$("$awk_bin" '/^[[:space:]]*package[[:space:]]+platformriver([[:space:]]|$)/ { count++ } END { print count + 0 }' "$candidate")" || fail "cannot inspect package: $label"
	[[ "$packages" == 1 ]] || fail "package must be exactly platformriver: $label"
	set +e
	format_output="$("$gofmt_bin" -d "$candidate" 2>&1)"
	format_status=$?
	set -e
	(( format_status == 0 )) || fail "gofmt failed: $label"
	[[ -z "$format_output" ]] || fail "gofmt diff: $label"
done
(( total_lines <= 320 )) || fail 'river implementation exceeds 320 lines'

checker_output="$(
	cd / || exit 1
	GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off \
		"$go_bin" run "$source_checker" -- "${candidates[@]}" 2>&1
)" || fail 'source contract rejected'
[[ "$checker_output" == 'p0-s04-source: PASS' ]] || fail 'source contract did not report PASS'
printf '%s\n' 'p0-s04-static: PASS'
