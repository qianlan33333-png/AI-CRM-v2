#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
fail() { echo "p0-s02-static: $*" >&2; exit 1; }
[[ -f "$repo_root/go.mod" &&
  -f "$repo_root/internal/api/generated/server.gen.go" ]] || fail "invalid repository root: $repo_root"
cd "$repo_root"

implementation="internal/platform/http/health.go"
unit_test="internal/platform/http/health_test.go"
source_files=("$implementation" "$unit_test")
[[ -d internal/platform/http && ! -L internal/platform/http ]] || fail "HTTP package path must be a directory"
while IFS= read -r -d '' entry; do
  case "$entry" in
    internal/platform/http/contract.go|"$implementation"|"$unit_test") ;;
    *) fail "unexpected HTTP package entry: $entry" ;;
  esac
  [[ -f "$entry" && ! -L "$entry" ]] || fail "HTTP package entry must be a regular non-symlink file: $entry"
done < <(find internal/platform/http -mindepth 1 -maxdepth 1 -print0)
for source_file in "${source_files[@]}"; do
  [[ -f "$source_file" && ! -L "$source_file" ]] || fail "required Slice path must be a regular non-symlink file: $source_file"
  mode="$(stat -f '%Lp' "$source_file" 2>/dev/null || stat -c '%a' "$source_file")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || fail "cannot read file mode: $source_file"
  permissions=$((8#$mode))
  (( (permissions & 011) == 0 )) || fail "group/other execute bits are forbidden: $source_file"
done
line_count="$(awk 'END { print NR }' "$implementation")"
line_count=$((line_count + $(awk 'END { print NR }' "$unit_test")))
(( line_count <= 180 )) || fail "implementation files exceed 180 lines: $line_count"

export_checker_dir="$(mktemp -d -t p0s02-exports.XXXXXX)"
export_checker="$export_checker_dir/check.go"
trap 'rm -f "$export_checker"; rmdir "$export_checker_dir"' EXIT
cat >"$export_checker" <<'EOF'
package main
import("fmt";"go/ast";"go/parser";"go/token";"os";"strings")
func check(path,kind,name string){test:=strings.HasSuffix(path,"_test.go");if test&&kind=="func"&&strings.HasPrefix(name,"Test"){return};if !test&&((kind=="type"&&name=="HealthHandler")||(kind=="func"&&(name=="NewHealthHandler"||name=="GetHealthz"))){return};fmt.Fprintf(os.Stderr,"p0-s02-static: forbidden exported %s %s in %s\n",kind,name,path);os.Exit(1)}
func main(){for _,path:=range os.Args[2:]{file,err:=parser.ParseFile(token.NewFileSet(),path,nil,0);if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)};for _,decl:=range file.Decls{switch node:=decl.(type){case *ast.GenDecl:for _,spec:=range node.Specs{switch item:=spec.(type){case *ast.TypeSpec:if item.Name.IsExported(){check(path,"type",item.Name.Name)};case *ast.ValueSpec:for _,name:=range item.Names{if name.IsExported(){check(path,"value",name.Name)}}}};case *ast.FuncDecl:if node.Name.IsExported(){check(path,"func",node.Name.Name)}}}}}
EOF
GOFLAGS=-mod=readonly go run "$export_checker" -- "${source_files[@]}"

forbidden='"encoding/json"|"net(/[^"[:space:]]*)?"|"database/sql"|"os(/[^"[:space:]]*)?"|"io(/[^"[:space:]]*)?"|"path/filepath"|"syscall"|"time"|"log"|"embed"|golang\.org/x/sys|github\.com/jackc/pgx|github\.com/riverqueue/river|\.(Getenv|LookupEnv|Open|OpenFile|ReadFile|ReadDir|Stat|Lstat|QueryRow|Query|Exec|Begin|Ping|Dial|DialContext|Listen|ListenPacket|Connect|Socket)\b|time\.|(^|[^[:alnum:]_])go[[:space:]]+|\.ListenAndServe|\b(readiness|ready|database|river|queue|settings|uptime|hostname|version)\b'
scan_status=0
rg -ni "$forbidden" "${source_files[@]}" || scan_status=$?
(( scan_status == 1 )) || fail "forbidden dependency, side effect, readiness claim, or scan error found"

rg -Uq 'type[[:space:]]+HealthHandler[[:space:]]+struct[[:space:]]*\{[[:space:]]*\}' "$implementation" || {
  echo "p0-s02-static: HealthHandler must be stateless" >&2
  exit 1
}
rg -q 'var[[:space:]]+_[[:space:]]+generated\.StrictServerInterface[[:space:]]*=[[:space:]]*\(\*HealthHandler\)\(nil\)' "$implementation" || {
  echo "p0-s02-static: missing strict-interface compile-time assertion" >&2
  exit 1
}

for required in \
  'generated.StrictServerInterface' \
  'generated.GetHealthz200JSONResponse' \
  'generated.Ok'; do
  rg -Fq "$required" "$implementation" || {
    echo "p0-s02-static: missing frozen generated contract reference: $required" >&2
    exit 1
  }
done

rg -Uq 'return[[:space:]]+generated\.GetHealthz200JSONResponse[[:space:]]*\{[[:space:]]*Status:[[:space:]]*generated\.Ok[[:space:]]*\}[[:space:]]*,[[:space:]]*nil' "$implementation" || {
  echo "p0-s02-static: handler must return generated OK response and nil error" >&2
  exit 1
}

GOFLAGS=-mod=readonly go test -race -timeout=15s ./internal/platform/http

echo "p0-s02-static: PASS"
