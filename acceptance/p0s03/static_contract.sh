#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
fail() { printf 'p0-s03-static: %s\n' "$*" >&2; exit 1; }
mode_of() {
	local candidate
	candidate="$(stat -f '%p' "$1" 2>/dev/null || true)"
	if [[ "$candidate" =~ ^[0-7]+$ ]]; then printf '%s\n' "$candidate"; return; fi
	candidate="$(stat -c '%a' "$1" 2>/dev/null || true)"
	[[ "$candidate" =~ ^[0-7]+$ ]] || return 1
	printf '%s\n' "$candidate"
}

[[ -f "$repo_root/go.mod" ]] || fail "invalid repository root: $repo_root"
cd "$repo_root"
store_root='internal/platform/store'
for directory in internal internal/platform "$store_root" "$store_root/generated" "$store_root/queries"; do
	[[ -d "$directory" && ! -L "$directory" ]] || fail "required real directory is missing: $directory"
	mode="$(mode_of "$directory")" || fail "cannot read mode: $directory"
	[[ "$mode" =~ ^[0-7]+$ ]] && (( (8#$mode & 07777) == 0755 )) || fail "directory mode must be exactly 0755: $directory"
done
files=("$store_root/ping.go" "$store_root/ping_test.go")
generated_files=("$store_root/generated/db.go" "$store_root/generated/health.sql.go" "$store_root/generated/models.go" "$store_root/generated/querier.go")
ordinary=("$store_root/contract.go" "${files[@]}" "$store_root/queries/health.sql" "${generated_files[@]}")
shopt -s dotglob nullglob
store_entries=("$store_root"/*); query_entries=("$store_root/queries"/*); generated_entries=("$store_root/generated"/*)
shopt -u dotglob nullglob
for path in "${store_entries[@]}"; do
	case "$path" in
	"$store_root/contract.go"|"$store_root/ping.go"|"$store_root/ping_test.go"|"$store_root/generated"|"$store_root/queries") ;;
	*) fail "unexpected store top-level entry: $path" ;;
	esac
done
[[ "${#query_entries[@]}" = 1 && "${query_entries[0]}" = "$store_root/queries/health.sql" ]] || fail "queries must contain only health.sql"
[[ "${#generated_entries[@]}" = 4 ]] || fail "generated must contain exactly four frozen paths"
for path in "${generated_entries[@]}"; do
	case "$path" in "${generated_files[0]}"|"${generated_files[1]}"|"${generated_files[2]}"|"${generated_files[3]}") ;; *) fail "unexpected generated entry: $path" ;; esac
done
for path in "${ordinary[@]}"; do
	[[ -f "$path" && ! -L "$path" ]] || fail "required regular file is missing: $path"
	mode="$(mode_of "$path")" || fail "cannot read mode: $path"
	[[ "$mode" =~ ^[0-7]+$ ]] && (( (8#$mode & 07777) == 0644 )) || fail "mode must be exactly 0644: $path"
done
[[ -s "$store_root/ping_test.go" ]] || fail "ping_test.go must not be empty"
line_count="$(awk 'END { print NR }' "${files[@]}")"
(( line_count <= 220 )) || fail "ping.go and ping_test.go exceed 220 lines: $line_count"
formatted="$(gofmt -d "${files[@]}")" || fail "Go source is not parseable"
[[ -z "$formatted" ]] || fail "Go source is not gofmt-normalized"
for path in "${files[@]}"; do
	[[ "$(grep -Ec '^package[[:space:]]+' "$path" || true)" = 1 ]] && grep -Fqx 'package platformstore' "$path" || fail "only package platformstore is allowed: $path"
done
[[ -f "$script_dir/source_contract.go" && ! -L "$script_dir/source_contract.go" ]] || fail "source checker must be a regular file"
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly go run "$script_dir/source_contract.go" -- "${files[@]}"
implementation="$store_root/ping.go"
[[ "$(grep -Fxc 'type PingStore struct {' "$implementation" || true)" = 1 ]] || fail "PingStore declaration must be unique"
awk '$0 == "type PingStore struct {" { seen++; getline; valid = ($0 == "\tquerier dbgen.Querier"); getline; valid = valid && ($0 == "}") } END { exit !(seen == 1 && valid) }' "$implementation" || fail "PingStore must have only querier dbgen.Querier"
[[ "$(grep -Fxc 'func NewPingStore(db dbgen.DBTX) *PingStore {' "$implementation" || true)" = 1 ]] || fail "NewPingStore signature is required"
[[ "$(grep -Fxc 'func (store *PingStore) Ping(ctx context.Context) error {' "$implementation" || true)" = 1 ]] || fail "Ping signature is required"
grep -Fqx $'\treturn fmt.Errorf("platform store ping: unexpected value %d", value)' "$implementation" || fail "unexpected-value error must be exact"
grep -Fqx $'\tif value == 1 {' "$implementation" && grep -Fqx $'\t\treturn nil' "$implementation" || fail "generated value 1 must succeed"
grep -Fqx $'\tif err != nil {' "$implementation" && grep -Fqx $'\t\treturn err' "$implementation" || fail "generated error must return unchanged"
if grep -En '^(type|var|const|func)[[:space:]]' "$implementation" | grep -Ev ':(type PingStore struct \{|func NewPingStore\(db dbgen\.DBTX\) \*PingStore \{|func \(store \*PingStore\) Ping\(ctx context\.Context\) error \{)$' >/dev/null; then fail "unexpected implementation declaration"; fi
if grep -En '^(type|var|const)[[:space:]]+[[:upper:]]|^func[[:space:]]+([[:upper:]]|\([^)]*\)[[:space:]]+[[:upper:]])' "$store_root/ping_test.go" | grep -Ev ':func (Test[[:alnum:]_]*|\([^)]*\) Ping)\(' >/dev/null; then fail "unexpected exported test declaration"; fi
echo "p0-s03-static: PASS"
