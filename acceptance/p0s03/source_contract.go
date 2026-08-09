//go:build ignore

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const generated = "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store/generated"
const unexpected = "platform store ping: unexpected value %d"
const canonicalPingControlFlow = "Ping must contain exactly the canonical query/error/success/failure control flow"

var testPackages = map[string]bool{"context": true, "errors": true, "testing": true}
var testSelectors = map[string]bool{"context.Background": true, "context.Context": true, "errors.Is": true, "errors.New": true, "testing.T": true}
var deniedTestSelectors = map[string]bool{"Setenv": true, "TempDir": true, "Chdir": true, "Parallel": true}

func must(ok bool, message string) {
	if !ok {
		panic("p0-s03-source: " + message)
	}
}

func parse(path string) *ast.File {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
	must(err == nil, "cannot parse source")
	return file
}

func checkImports(file *ast.File, allowed map[string]string, exact bool) {
	seen := map[string]bool{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		must(err == nil, "invalid import")
		alias := ""
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		want, ok := allowed[path]
		must(ok && alias == want && !seen[path], "import is not allowed: "+path)
		seen[path] = true
	}
	must(!exact || len(seen) == len(allowed), "implementation imports are incomplete")
}

func directIdent(expr ast.Expr, name string) bool {
	identifier, ok := expr.(*ast.Ident)
	return ok && identifier.Name == name
}

func allowedSelector(selector *ast.SelectorExpr) (string, string, bool) {
	if target, ok := selector.X.(*ast.Ident); ok {
		switch target.Name + "." + selector.Sel.Name {
		case "dbgen.New":
			return "dbgen.New", "NewPingStore", true
		case "store.querier":
			return "store.querier", "Ping", true
		case "fmt.Errorf":
			return "fmt.Errorf", "Ping", true
		}
	}
	if target, ok := selector.X.(*ast.SelectorExpr); ok {
		receiver, ok := target.X.(*ast.Ident)
		if ok && receiver.Name == "store" && target.Sel.Name == "querier" && selector.Sel.Name == "Ping" {
			return "store.querier.Ping", "Ping", true
		}
	}
	return "", "", false
}

func allowedCall(call *ast.CallExpr) (string, bool) {
	if call == nil {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || call.Ellipsis != token.NoPos {
		return "", false
	}
	if target, ok := selector.X.(*ast.Ident); ok {
		if target.Name == "dbgen" && selector.Sel.Name == "New" {
			return "NewPingStore", len(call.Args) == 1 && directIdent(call.Args[0], "db")
		}
		if target.Name == "fmt" && selector.Sel.Name == "Errorf" && len(call.Args) == 2 {
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return "format", false
			}
			text, err := strconv.Unquote(literal.Value)
			return "format", literal.Kind == token.STRING && err == nil && text == unexpected && directIdent(call.Args[1], "value")
		}
	}
	if target, ok := selector.X.(*ast.SelectorExpr); ok {
		receiver, ok := target.X.(*ast.Ident)
		if ok && receiver.Name == "store" && target.Sel.Name == "querier" && selector.Sel.Name == "Ping" {
			return "Ping", len(call.Args) == 1 && directIdent(call.Args[0], "ctx")
		}
	}
	return "", false
}

func directInt(expr ast.Expr, value string) bool {
	literal, ok := expr.(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == value
}

func returnIdent(statement ast.Stmt, name string) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && directIdent(returned.Results[0], name)
}

func ifReturnsIdent(statement ast.Stmt, left string, operator token.Token, right, result string) bool {
	conditional, ok := statement.(*ast.IfStmt)
	if !ok || conditional.Init != nil || conditional.Else != nil || conditional.Body == nil || len(conditional.Body.List) != 1 {
		return false
	}
	comparison, ok := conditional.Cond.(*ast.BinaryExpr)
	return ok && comparison.Op == operator && directIdent(comparison.X, left) && directIdent(comparison.Y, right) && returnIdent(conditional.Body.List[0], result)
}

func checkCanonicalPing(function *ast.FuncDecl) {
	must(function != nil && function.Body != nil && len(function.Body.List) == 4, canonicalPingControlFlow)

	query, ok := function.Body.List[0].(*ast.AssignStmt)
	must(ok && query.Tok == token.DEFINE && len(query.Lhs) == 2 && len(query.Rhs) == 1 && directIdent(query.Lhs[0], "value") && directIdent(query.Lhs[1], "err"), canonicalPingControlFlow)
	call, ok := query.Rhs[0].(*ast.CallExpr)
	owner, allowed := allowedCall(call)
	must(ok && allowed && owner == "Ping", canonicalPingControlFlow)

	must(ifReturnsIdent(function.Body.List[1], "err", token.NEQ, "nil", "err"), canonicalPingControlFlow)

	success, ok := function.Body.List[2].(*ast.IfStmt)
	must(ok && success.Init == nil && success.Else == nil && success.Body != nil && len(success.Body.List) == 1, canonicalPingControlFlow)
	comparison, ok := success.Cond.(*ast.BinaryExpr)
	must(ok && comparison.Op == token.EQL && directIdent(comparison.X, "value") && directInt(comparison.Y, "1") && returnIdent(success.Body.List[0], "nil"), canonicalPingControlFlow)

	failure, ok := function.Body.List[3].(*ast.ReturnStmt)
	must(ok && len(failure.Results) == 1, canonicalPingControlFlow)
	call, ok = failure.Results[0].(*ast.CallExpr)
	owner, allowed = allowedCall(call)
	must(ok && allowed && owner == "format", canonicalPingControlFlow)
}

func isTest(function *ast.FuncDecl) bool {
	name := function.Name.Name
	actualTest := name == "Test"
	if strings.HasPrefix(name, "Test") && len(name) > 4 {
		value, _ := utf8.DecodeRuneInString(name[4:])
		actualTest = !unicode.IsLower(value)
	}
	if function.Recv != nil || function.Body == nil || !actualTest || function.Type.Results != nil || len(function.Type.Params.List) != 1 {
		return false
	}
	pointer, ok := function.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Name == "testing" && selector.Sel.Name == "T"
}

func main() {
	arguments := os.Args[1:]
	if len(arguments) > 0 && arguments[0] == "--" {
		arguments = arguments[1:]
	}
	must(len(arguments) == 2, "usage: source_contract.go ping.go ping_test.go")
	implementation, tests := parse(arguments[0]), parse(arguments[1])
	checkImports(implementation, map[string]string{"context": "", "fmt": "", generated: "dbgen"}, true)
	checkImports(tests, map[string]string{"context": "", "errors": "", "testing": ""}, false)
	counts, selectorCounts, stringsSeen := map[string]int{}, map[string]int{}, 0
	var ping *ast.FuncDecl
	for _, declaration := range implementation.Decls {
		if general, ok := declaration.(*ast.GenDecl); ok && general.Tok == token.IMPORT {
			continue
		}
		function, _ := declaration.(*ast.FuncDecl)
		if function != nil && function.Name.Name == "Ping" {
			must(ping == nil, canonicalPingControlFlow)
			ping = function
		}
		ast.Inspect(declaration, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CallExpr:
				owner, allowed := allowedCall(value)
				must(allowed, "implementation call is not allowed")
				counts[owner]++
				want := owner
				if owner == "format" {
					want = "Ping"
				}
				must(function != nil && function.Name.Name == want, "allowed call is in the wrong function")
			case *ast.BasicLit:
				if value.Kind == token.STRING {
					text, err := strconv.Unquote(value.Value)
					must(err == nil && text == unexpected && function != nil && function.Name.Name == "Ping", "implementation string literal is not allowed")
					stringsSeen++
				}
			case *ast.BinaryExpr:
				must(value.Op != token.ADD, "implementation string construction is forbidden")
			case *ast.GoStmt, *ast.DeferStmt:
				must(false, "go and defer statements are forbidden")
			case *ast.FuncLit:
				must(false, "function literals are forbidden")
			}
			return true
		})
		if function != nil {
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if selector, ok := node.(*ast.SelectorExpr); ok {
					name, owner, allowed := allowedSelector(selector)
					must(allowed, "implementation body selector is not allowed")
					must(function.Name.Name == owner, "implementation selector is in the wrong function")
					selectorCounts[name]++
				}
				return true
			})
		}
	}
	must(counts["NewPingStore"] == 1 && counts["Ping"] == 1 && counts["format"] == 1 && stringsSeen == 1, "required calls and error literal must each appear exactly once")
	must(selectorCounts["dbgen.New"] == 1 && selectorCounts["store.querier"] == 1 && selectorCounts["store.querier.Ping"] == 1 && selectorCounts["fmt.Errorf"] == 1, "allowed implementation selectors must each appear exactly once")
	checkCanonicalPing(ping)
	testCount := 0
	for _, declaration := range tests.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && isTest(function) {
			testCount++
		}
	}
	ast.Inspect(tests, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.SelectorExpr:
			must(!deniedTestSelectors[value.Sel.Name], "test method is not allowed: "+value.Sel.Name)
			if target, ok := value.X.(*ast.Ident); ok && testPackages[target.Name] {
				must(testSelectors[target.Name+"."+value.Sel.Name], "test package call is not allowed")
			}
		case *ast.GoStmt, *ast.DeferStmt:
			must(false, "go and defer statements are forbidden")
		}
		return true
	})
	must(testCount > 0, "at least one Test function is required")
	println("p0-s03-source: PASS")
}
