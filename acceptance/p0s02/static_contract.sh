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
contract_file="internal/platform/http/contract.go"
source_files=("$implementation" "$unit_test")
mode_of() {
  local path="$1" mode
  mode="$(stat -f '%Lp' "$path" 2>/dev/null || true)"
  if [[ "$mode" =~ ^[0-7]{3,4}$ ]]; then
    printf '%s\n' "$mode"
    return
  fi
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || return 1
  printf '%s\n' "$mode"
}
for directory in internal internal/platform internal/platform/http; do
  [[ -d "$directory" && ! -L "$directory" ]] || fail "repository directory must be real: $directory"
done
[[ -f "$contract_file" && ! -L "$contract_file" ]] || fail "contract.go must be a regular non-symlink file: $contract_file"
[[ "$(mode_of "$contract_file")" =~ ^0?644$ ]] || fail "contract.go must have mode 0644: $contract_file"
inventory_file="$(mktemp -t p0s02-http.XXXXXX)" || fail "cannot create HTTP inventory"
trap 'rm -f "$inventory_file"' EXIT
if ! find internal/platform/http -mindepth 1 -maxdepth 1 -print0 >"$inventory_file"; then
  fail "cannot inspect HTTP package entries"
fi
while IFS= read -r -d '' entry; do
  case "$entry" in
    internal/platform/http/contract.go|"$implementation"|"$unit_test"|internal/platform/http/errors.go|internal/platform/http/gateway.go|internal/platform/http/gateway_test.go) ;;
    *) fail "unexpected HTTP package entry: $entry" ;;
  esac
  [[ -f "$entry" && ! -L "$entry" ]] || fail "HTTP package entry must be a regular non-symlink file: $entry"
done <"$inventory_file"
for source_file in "${source_files[@]}"; do
  [[ -f "$source_file" && ! -L "$source_file" ]] || fail "required Slice path must be a regular non-symlink file: $source_file"
  mode="$(mode_of "$source_file")" || fail "cannot read file mode: $source_file"
  permissions=$(( (8#$mode) & 07777 ))
  (( (permissions & 011) == 0 )) || fail "group/other execute bits are forbidden: $source_file"
done
line_count="$(awk 'END { print NR }' "$implementation")"
line_count=$((line_count + $(awk 'END { print NR }' "$unit_test")))
(( line_count <= 180 )) || fail "implementation files exceed 180 lines: $line_count"

export_checker_dir="$(mktemp -d -t p0s02-exports.XXXXXX)" || fail "cannot create AST checker directory"
export_checker="$export_checker_dir/check.go"
trap 'rm -f "$inventory_file" "$export_checker"; rmdir "$export_checker_dir"' EXIT
cat >"$export_checker" <<'EOF'
package main
import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)
var forbiddenSelectors = map[string]bool{
	"Getenv": true, "LookupEnv": true, "Open": true, "OpenFile": true, "ReadFile": true,
	"ReadDir": true, "Stat": true, "Lstat": true, "QueryRow": true, "Query": true,
	"Exec": true, "Begin": true, "Ping": true, "Dial": true, "DialContext": true,
	"Listen": true, "ListenPacket": true, "Connect": true, "Socket": true, "ListenAndServe": true,
}
func fail(format string, args ...interface{}) { fmt.Fprintf(os.Stderr, "p0-s02-static: "+format+"\n", args...); os.Exit(1) }
func generatedSelector(expr ast.Expr, name string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name { return false }
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "generated"
}
func healthHandlerPointerNil(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 { return false }
	nilIdent, nilOK := call.Args[0].(*ast.Ident)
	paren, parenOK := call.Fun.(*ast.ParenExpr)
	if !nilOK || nilIdent.Name != "nil" || !parenOK { return false }
	pointer, ok := paren.X.(*ast.StarExpr)
	if !ok { return false }
	ident, ok := pointer.X.(*ast.Ident)
	return ok && ident.Name == "HealthHandler"
}
func generatedOKResponse(expr ast.Expr) bool {
	response, ok := expr.(*ast.CompositeLit)
	if !ok || !generatedSelector(response.Type, "GetHealthz200JSONResponse") || len(response.Elts) != 1 { return false }
	field, ok := response.Elts[0].(*ast.KeyValueExpr)
	if !ok { return false }
	name, ok := field.Key.(*ast.Ident)
	return ok && name.Name == "Status" && generatedSelector(field.Value, "Ok")
}
func forbiddenImport(path string) bool {
	switch path {
	case "encoding/json", "database/sql", "path/filepath", "syscall", "time", "log", "embed":
		return true
	}
	return path == "net" || strings.HasPrefix(path, "net/") ||
		path == "os" || strings.HasPrefix(path, "os/") ||
		path == "io" || strings.HasPrefix(path, "io/") ||
		strings.HasPrefix(path, "golang.org/x/sys") ||
		strings.HasPrefix(path, "github.com/jackc/pgx") ||
		strings.HasPrefix(path, "github.com/riverqueue/river")
}
func allowExport(path, kind, name string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return kind == "func" && strings.HasPrefix(name, "Test")
	}
	return (kind == "type" && name == "HealthHandler") ||
		(kind == "func" && (name == "NewHealthHandler" || name == "GetHealthz"))
}
func main() {
	healthHandler := false
	newHealthHandler := false
	strictAssertion := false
	okResponse := false
	for _, path := range os.Args[2:] {
		testFile := strings.HasSuffix(path, "_test.go")
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil { fail("cannot parse %s: %v", path, err) }
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || forbiddenImport(importPath) { fail("forbidden import in %s", path) }
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch item := node.(type) {
			case *ast.GoStmt:
				fail("goroutines are forbidden in %s", path)
			case *ast.SelectorExpr:
				if forbiddenSelectors[item.Sel.Name] { fail("forbidden side effect in %s", path) }
				if ident, ok := item.X.(*ast.Ident); ok && ident.Name == "time" { fail("time use is forbidden in %s", path) }
			}
			return true
		})
		for _, declaration := range file.Decls {
			switch item := declaration.(type) {
			case *ast.GenDecl:
				for _, spec := range item.Specs {
					switch value := spec.(type) {
					case *ast.TypeSpec:
						if value.Name.IsExported() && !allowExport(path, "type", value.Name.Name) { fail("forbidden exported type %s in %s", value.Name.Name, path) }
						if !testFile && value.Name.Name == "HealthHandler" {
							structType, ok := value.Type.(*ast.StructType)
							if !ok || len(structType.Fields.List) != 0 { fail("HealthHandler must be stateless") }
							healthHandler = true
						}
					case *ast.ValueSpec:
						for _, name := range value.Names {
							if name.IsExported() && !allowExport(path, "value", name.Name) { fail("forbidden exported value %s in %s", name.Name, path) }
						}
						if !testFile && len(value.Names) == 1 && value.Names[0].Name == "_" &&
							generatedSelector(value.Type, "StrictServerInterface") && len(value.Values) == 1 &&
							healthHandlerPointerNil(value.Values[0]) {
							strictAssertion = true
						}
					}
				}
			case *ast.FuncDecl:
				if item.Name.IsExported() && !allowExport(path, "func", item.Name.Name) { fail("forbidden exported func %s in %s", item.Name.Name, path) }
				if !testFile && item.Name.Name == "NewHealthHandler" { newHealthHandler = true }
				if !testFile && item.Name.Name == "GetHealthz" && item.Body != nil {
					ast.Inspect(item.Body, func(node ast.Node) bool {
						returned, ok := node.(*ast.ReturnStmt)
						if !ok || len(returned.Results) != 2 || !generatedOKResponse(returned.Results[0]) { return true }
						nilIdent, ok := returned.Results[1].(*ast.Ident)
						if ok && nilIdent.Name == "nil" { okResponse = true }
						return true
					})
				}
			}
		}
	}
	if !healthHandler { fail("missing HealthHandler") }
	if !newHealthHandler { fail("missing NewHealthHandler") }
	if !strictAssertion { fail("missing strict-interface compile-time assertion") }
	if !okResponse { fail("handler must return generated OK response and nil error") }
}
EOF
GOFLAGS=-mod=readonly go run "$export_checker" -- "${source_files[@]}"

if ! awk '
  {
    line = tolower($0)
    if (line ~ /"encoding\/json"|"net(\/[^"[:space:]]*)?"|"database\/sql"|"os(\/[^"[:space:]]*)?"|"io(\/[^"[:space:]]*)?"|"path\/filepath"|"syscall"|"time"|"log"|"embed"|golang\.org\/x\/sys|github\.com\/jackc\/pgx|github\.com\/riverqueue\/river/) exit 1
    if (line ~ /\.(getenv|lookupenv|open|openfile|readfile|readdir|stat|lstat|queryrow|query|exec|begin|ping|dial|dialcontext|listen|listenpacket|connect|socket)([^[:alnum:]_]|$)|time\.|\.listenandserve|(^|[^[:alnum:]_])go[[:space:]]+/) exit 1
    gsub(/[^[:alnum:]_]/, " ", line)
    count = split(line, words, /[[:space:]]+/)
    for (i = 1; i <= count; i++) {
      if (words[i] == "readiness" || words[i] == "ready" || words[i] == "database" || words[i] == "river" || words[i] == "queue" || words[i] == "settings" || words[i] == "uptime" || words[i] == "hostname" || words[i] == "version") exit 1
    }
  }
' "${source_files[@]}"; then
  fail "forbidden dependency, side effect, readiness claim, or scan error found"
fi

GOFLAGS=-mod=readonly go test -race -timeout=15s ./internal/platform/http

echo "p0-s02-static: PASS"
