//go:build ignore

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strconv"
	"strings"
)

const prefix = "p0-s04-source: "

func fail(message string) { panic(prefix + message) }
func must(ok bool, message string) {
	if !ok {
		fail(message)
	}
}

func parse(path string) *ast.File {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
	must(err == nil, "cannot parse "+path)
	return file
}

func imports(file *ast.File, allowed map[string]bool, exact bool) {
	seen := map[string]bool{}
	for _, spec := range file.Imports {
		must(spec.Name == nil, "import aliases, dot imports, and blank imports are forbidden")
		value, err := strconv.Unquote(spec.Path.Value)
		must(err == nil && allowed[value] && !seen[value], "import is not allowed: "+spec.Path.Value)
		seen[value] = true
	}
	must(!exact || len(seen) == len(allowed), "required imports are incomplete")
}

func ident(expression ast.Expr, name string) bool {
	value, ok := expression.(*ast.Ident)
	return ok && value.Name == name
}

func selector(expression ast.Expr, left, right string) bool {
	value, ok := expression.(*ast.SelectorExpr)
	return ok && ident(value.X, left) && value.Sel.Name == right
}

func call(expression ast.Expr, left, right string) (*ast.CallExpr, bool) {
	value, ok := expression.(*ast.CallExpr)
	return value, ok && selector(value.Fun, left, right)
}

func lifecycleCall(value *ast.CallExpr, name string) bool {
	selectorValue, ok := value.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	owner, ok := selectorValue.X.(*ast.SelectorExpr)
	return ok && selectorValue.Sel.Name == name && ident(owner.X, "r") && owner.Sel.Name == "lifecycle"
}

func stopCall(value *ast.CallExpr) bool { return selector(value.Fun, "r", "stop") }

func withoutCancel(expression ast.Expr) bool {
	value, ok := call(expression, "context", "WithoutCancel")
	return ok && len(value.Args) == 1 && ident(value.Args[0], "parent")
}

func receiver(function *ast.FuncDecl, name string) bool {
	if function == nil || function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 || function.Recv.List[0].Names[0].Name != "r" {
		return false
	}
	pointer, ok := function.Recv.List[0].Type.(*ast.StarExpr)
	return ok && ident(pointer.X, name)
}

func declarations(file *ast.File, allowed map[string]bool, tests bool) {
	seen := map[string]bool{}
	for _, declaration := range file.Decls {
		if tests {
			if value, ok := declaration.(*ast.FuncDecl); ok && value.Recv == nil && value.Name.IsExported() && !strings.HasPrefix(value.Name.Name, "Test") {
				fail("unexpected exported symbol: " + value.Name.Name)
			}
			continue
		}
		switch value := declaration.(type) {
		case *ast.GenDecl:
			if value.Tok == token.IMPORT {
				continue
			}
			for _, specification := range value.Specs {
				item, ok := specification.(*ast.TypeSpec)
				must(ok && allowed[item.Name.Name] && !seen[item.Name.Name], "unexpected package declaration")
				seen[item.Name.Name] = true
			}
		case *ast.FuncDecl:
			must(allowed[value.Name.Name] && !seen[value.Name.Name], "unexpected package declaration")
			if value.Name.Name == "Run" || value.Name.Name == "stop" {
				must(receiver(value, "Runtime"), "unexpected Runtime method")
			} else if value.Name.Name == "Error" || value.Name.Name == "Unwrap" {
				must(value.Recv != nil && len(value.Recv.List) == 1 && ident(value.Recv.List[0].Type, "invalidDirectionError"), "unexpected invalid direction method")
			} else {
				must(value.Recv == nil, "unexpected receiver method")
			}
			seen[value.Name.Name] = true
		default:
			fail("unexpected package declaration")
		}
	}
}

func receive(expression ast.Expr, owner, method string) bool {
	value, ok := expression.(*ast.UnaryExpr)
	if !ok || value.Op != token.ARROW {
		return false
	}
	callValue, ok := value.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	if method == "Done" {
		return selector(callValue.Fun, owner, method)
	}
	return lifecycleCall(callValue, method)
}

func clauses(selectValue *ast.SelectStmt, owner string) (cancel, stopped, fallback *ast.CommClause) {
	for _, statement := range selectValue.Body.List {
		item, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		if item.Comm == nil {
			fallback = item
			continue
		}
		expression, ok := item.Comm.(*ast.ExprStmt)
		if !ok {
			continue
		}
		value := expression.X
		if receive(value, owner, "Done") {
			cancel = item
		}
		if receive(value, "", "Stopped") {
			stopped = item
		}
	}
	return
}

func containsCall(nodes []ast.Stmt, predicate func(*ast.CallExpr) bool) bool {
	found := false
	for _, node := range nodes {
		ast.Inspect(node, func(value ast.Node) bool {
			if item, ok := value.(*ast.CallExpr); ok && predicate(item) {
				found = true
			}
			return true
		})
	}
	return found
}

func containsSelector(nodes []ast.Stmt, left, right string) bool {
	found := false
	for _, node := range nodes {
		ast.Inspect(node, func(value ast.Node) bool {
			if item, ok := value.(*ast.SelectorExpr); ok && selector(item, left, right) {
				found = true
			}
			return true
		})
	}
	return found
}

func checkRuntime(file *ast.File) {
	var runtimeType *ast.StructType
	var constructor, run, stop *ast.FuncDecl
	for _, declaration := range file.Decls {
		switch item := declaration.(type) {
		case *ast.GenDecl:
			for _, specification := range item.Specs {
				if value, ok := specification.(*ast.TypeSpec); ok && value.Name.Name == "Runtime" {
					runtimeType, _ = value.Type.(*ast.StructType)
				}
			}
		case *ast.FuncDecl:
			switch item.Name.Name {
			case "NewRuntime":
				constructor = item
			case "Run":
				run = item
			case "stop":
				stop = item
			}
		}
	}
	must(runtimeType != nil && len(runtimeType.Fields.List) == 1 && len(runtimeType.Fields.List[0].Names) == 1 && runtimeType.Fields.List[0].Names[0].Name == "lifecycle" && ident(runtimeType.Fields.List[0].Type, "Lifecycle"), "Runtime must hold only Lifecycle")
	must(constructor != nil && constructor.Recv == nil && len(constructor.Type.Params.List) == 1 && len(constructor.Type.Params.List[0].Names) == 1 && constructor.Type.Params.List[0].Names[0].Name == "lifecycle" && ident(constructor.Type.Params.List[0].Type, "Lifecycle") && len(constructor.Type.Results.List) == 1, "NewRuntime signature is invalid")
	constructorType, constructorOK := constructor.Type.Results.List[0].Type.(*ast.StarExpr)
	must(constructorOK && ident(constructorType.X, "Runtime") && len(constructor.Body.List) == 1, "NewRuntime must return one Runtime")
	constructorResult, ok := constructor.Body.List[0].(*ast.ReturnStmt)
	must(ok && len(constructorResult.Results) == 1, "NewRuntime must return one Runtime")
	address, addressOK := constructorResult.Results[0].(*ast.UnaryExpr)
	must(addressOK && address.Op == token.AND, "NewRuntime must allocate Runtime")
	literal, literalOK := address.X.(*ast.CompositeLit)
	must(literalOK && ident(literal.Type, "Runtime") && len(literal.Elts) == 1, "NewRuntime must initialize only lifecycle")
	field, fieldOK := literal.Elts[0].(*ast.KeyValueExpr)
	must(fieldOK && ident(field.Key, "lifecycle") && ident(field.Value, "lifecycle"), "NewRuntime must retain Lifecycle")
	must(run != nil && receiver(run, "Runtime") && len(run.Type.Params.List) == 1 && len(run.Type.Params.List[0].Names) == 1 && run.Type.Params.List[0].Names[0].Name == "parent" && selector(run.Type.Params.List[0].Type, "context", "Context") && len(run.Type.Results.List) == 1 && ident(run.Type.Results.List[0].Type, "error"), "Run signature is invalid")
	must(stop != nil && receiver(stop, "Runtime") && len(stop.Type.Params.List) == 1 && len(stop.Type.Params.List[0].Names) == 1 && stop.Type.Params.List[0].Names[0].Name == "parent" && len(stop.Type.Results.List) == 1 && ident(stop.Type.Results.List[0].Type, "error"), "stop signature is invalid")
	must(len(run.Body.List) > 0, "Run must preserve Start errors")
	startGuard, ok := run.Body.List[0].(*ast.IfStmt)
	must(ok && startGuard.Init != nil && len(startGuard.Body.List) == 1, "Run must preserve Start errors")
	assignment, ok := startGuard.Init.(*ast.AssignStmt)
	must(ok && len(assignment.Lhs) == 1 && ident(assignment.Lhs[0], "err") && len(assignment.Rhs) == 1, "Run must preserve Start errors")
	startValue, startOK := assignment.Rhs[0].(*ast.CallExpr)
	condition, conditionOK := startGuard.Cond.(*ast.BinaryExpr)
	result, returnOK := startGuard.Body.List[0].(*ast.ReturnStmt)
	must(startOK && conditionOK && returnOK && lifecycleCall(startValue, "Start") && condition.Op == token.NEQ && ident(condition.X, "err") && ident(condition.Y, "nil") && len(result.Results) == 1 && ident(result.Results[0], "err"), "Run must return the original Start error")
	must(len(run.Body.List) == 2, "Run must contain only Start guard and select")
	outer, outerOK := run.Body.List[1].(*ast.SelectStmt)
	must(outerOK, "Run must contain only Start guard and select")
	cancel, stoppedCase, fallback := clauses(outer, "parent")
	must(len(outer.Body.List) == 2 && cancel != nil && stoppedCase != nil && fallback == nil, "Run requires exact cancellation and Stopped select")
	starts, stopped, stopCalls := 0, 0, 0
	ast.Inspect(run.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			fail("Run side effects are forbidden")
		case *ast.CallExpr:
			if lifecycleCall(value, "Start") {
				starts++
				must(len(value.Args) == 1 && withoutCancel(value.Args[0]), "Start must use context.WithoutCancel(parent)")
			} else if lifecycleCall(value, "Stopped") {
				stopped++
			} else if stopCall(value) {
				stopCalls++
				must(len(value.Args) == 1 && ident(value.Args[0], "parent"), "Stop must retain the parent value context")
			} else if item, ok := call(value, "context", "WithoutCancel"); ok {
				must(len(item.Args) == 1 && ident(item.Args[0], "parent"), "WithoutCancel must retain parent values")
			} else if !selector(value.Fun, "parent", "Done") {
				fail("runtime call is not allowed")
			}
		}
		return true
	})
	must(containsCall(cancel.Body, stopCall), "parent cancellation must Stop")
	var nested *ast.SelectStmt
	for _, statement := range stoppedCase.Body {
		if value, ok := statement.(*ast.SelectStmt); ok {
			nested = value
		}
	}
	must(nested != nil, "Stopped branch must recheck parent cancellation")
	recheck, _, nestedFallback := clauses(nested, "parent")
	must(len(nested.Body.List) == 2 && recheck != nil && nestedFallback != nil && containsCall(recheck.Body, stopCall) && containsSelector(nestedFallback.Body, "runtime", "ErrUnexpectedStop"), "Stopped branch must prefer concurrent cancellation")
	must(len(stop.Body.List) == 3, "stop must have exactly three statements")
	shutdown, ok := stop.Body.List[0].(*ast.AssignStmt)
	must(ok && shutdown.Tok == token.DEFINE && len(shutdown.Lhs) == 2 && ident(shutdown.Lhs[0], "shutdown") && ident(shutdown.Lhs[1], "cancel") && len(shutdown.Rhs) == 1, "stop must bind shutdown and cancel")
	timeout, ok := shutdown.Rhs[0].(*ast.CallExpr)
	must(ok && selector(timeout.Fun, "context", "WithTimeout") && len(timeout.Args) == 2 && withoutCancel(timeout.Args[0]) && selector(timeout.Args[1], "runtime", "ShutdownGrace"), "stop must use bounded live shutdown context")
	deferred, ok := stop.Body.List[1].(*ast.DeferStmt)
	must(ok && ident(deferred.Call.Fun, "cancel") && len(deferred.Call.Args) == 0, "stop must defer cancel")
	stopResult, ok := stop.Body.List[2].(*ast.ReturnStmt)
	must(ok && len(stopResult.Results) == 1, "stop must return Stop")
	stoppedCall, ok := stopResult.Results[0].(*ast.CallExpr)
	must(ok && lifecycleCall(stoppedCall, "Stop") && len(stoppedCall.Args) == 1 && ident(stoppedCall.Args[0], "shutdown"), "Stop must return its original error")
	must(starts == 1 && stopped == 1 && stopCalls == 2, "runtime lifecycle calls are incomplete or duplicated")
}

func invalidCondition(expression ast.Expr) bool {
	value, ok := expression.(*ast.BinaryExpr)
	if !ok || value.Op != token.LAND {
		return false
	}
	match := func(item ast.Expr, direction string) bool {
		comparison, ok := item.(*ast.BinaryExpr)
		return ok && comparison.Op == token.NEQ && ident(comparison.X, "direction") && ident(comparison.Y, direction)
	}
	return match(value.X, "DirectionUp") && match(value.Y, "DirectionDown")
}

func checkMigrate(file *ast.File) {
	var migrate *ast.FuncDecl
	invalidType, errorText, unwrap := false, false, false
	for _, declaration := range file.Decls {
		if value, ok := declaration.(*ast.GenDecl); ok {
			for _, specification := range value.Specs {
				if item, ok := specification.(*ast.TypeSpec); ok && item.Name.Name == "invalidDirectionError" && ident(item.Type, "Direction") {
					invalidType = true
				}
			}
		}
		if value, ok := declaration.(*ast.FuncDecl); ok {
			if value.Name.Name == "Migrate" && value.Recv == nil {
				migrate = value
			}
			if value.Recv != nil && len(value.Recv.List) == 1 && ident(value.Recv.List[0].Type, "invalidDirectionError") {
				if value.Name.Name == "Error" {
					must(len(value.Recv.List[0].Names) == 1 && value.Recv.List[0].Names[0].Name == "direction" && len(value.Body.List) == 1, "invalid direction Error must have one exact return")
					result, ok := value.Body.List[0].(*ast.ReturnStmt)
					must(ok && len(result.Results) == 1, "invalid direction Error must have one exact return")
					var source bytes.Buffer
					must(printer.Fprint(&source, token.NewFileSet(), result.Results[0]) == nil && source.String() == "`platform river migration: invalid direction \"` + string(direction) + `\"`", "invalid direction Error text is not exact")
					errorText = true
				}
				if value.Name.Name == "Unwrap" && len(value.Body.List) == 1 {
					if result, ok := value.Body.List[0].(*ast.ReturnStmt); ok && len(result.Results) == 1 && ident(result.Results[0], "ErrInvalidDirection") {
						unwrap = true
					}
				}
			}
		}
	}
	must(invalidType && errorText && unwrap, "invalid direction must preserve exact text and ErrInvalidDirection")
	must(migrate != nil && len(migrate.Type.Params.List) == 4 && len(migrate.Type.Results.List) == 1 && ident(migrate.Type.Results.List[0].Type, "error"), "Migrate signature is invalid")
	parameters := migrate.Type.Params.List
	must(len(parameters[0].Names) == 1 && parameters[0].Names[0].Name == "ctx" && selector(parameters[0].Type, "context", "Context") && len(parameters[1].Names) == 1 && parameters[1].Names[0].Name == "pool" && len(parameters[2].Names) == 1 && parameters[2].Names[0].Name == "direction" && ident(parameters[2].Type, "Direction") && len(parameters[3].Names) == 1 && parameters[3].Names[0].Name == "options", "Migrate parameters are invalid")
	poolType, poolOK := parameters[1].Type.(*ast.StarExpr)
	optionsType, optionsOK := parameters[3].Type.(*ast.StarExpr)
	must(poolOK && selector(poolType.X, "pgxpool", "Pool") && optionsOK && ident(optionsType.X, "MigrateOptions"), "Migrate pointer parameters are invalid")
	must(len(migrate.Body.List) > 0, "Migrate must validate direction")
	validation, ok := migrate.Body.List[0].(*ast.IfStmt)
	must(ok && invalidCondition(validation.Cond) && len(validation.Body.List) == 1, "invalid direction validation must precede driver or pool access")
	returnValue, ok := validation.Body.List[0].(*ast.ReturnStmt)
	must(ok && len(returnValue.Results) == 1, "invalid direction must return a sentinel wrapper")
	value, ok := returnValue.Results[0].(*ast.CallExpr)
	must(ok && ident(value.Fun, "invalidDirectionError") && len(value.Args) == 1 && ident(value.Args[0], "direction"), "invalid direction must fail closed")
	ast.Inspect(migrate.Body, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			fail("Migrate side effects are forbidden")
		case *ast.BasicLit:
			must(item.Kind != token.STRING, "Migrate string literals are forbidden")
		case *ast.SelectorExpr:
			must(item.Sel.Name != "Exec" && item.Sel.Name != "Query" && item.Sel.Name != "QueryRow", "pool Exec/Query is forbidden")
		}
		return true
	})
	driver, migration, migrationCall := 0, 0, 0
	ast.Inspect(migrate.Body, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.CallExpr:
			if selector(item.Fun, "riverpgxv5", "New") {
				driver++
				must(item.Pos() > validation.Pos(), "invalid direction validation must precede driver or pool access")
			} else if selector(item.Fun, "rivermigrate", "New") {
				migration++
				must(item.Pos() > validation.Pos(), "invalid direction validation must precede driver or pool access")
			} else if selector(item.Fun, "migrator", "Migrate") {
				migrationCall++
			} else if ident(item.Fun, "invalidDirectionError") || ident(item.Fun, "string") {
			} else {
				fail("migration call is not allowed")
			}
		case *ast.CompositeLit:
			if selector(item.Type, "rivermigrate", "MigrateOpts") {
				must(len(item.Elts) == 1, "only TargetVersion may be forwarded")
				field, ok := item.Elts[0].(*ast.KeyValueExpr)
				must(ok && ident(field.Key, "TargetVersion"), "only TargetVersion may be forwarded")
			}
		}
		return true
	})
	must(driver == 1 && migration == 1 && migrationCall == 1, "official River migration calls are incomplete or duplicated")
}

func main() {
	arguments := os.Args[1:]
	if len(arguments) > 0 && arguments[0] == "--" {
		arguments = arguments[1:]
	}
	must(len(arguments) == 3, "usage: source_contract.go runtime.go migrate.go runtime_test.go")
	runtimeFile, migrateFile, testFile := parse(arguments[0]), parse(arguments[1]), parse(arguments[2])
	for _, file := range []*ast.File{runtimeFile, migrateFile, testFile} {
		must(file.Name.Name == "platformriver", "only package platformriver is allowed")
	}
	imports(runtimeFile, map[string]bool{"context": true, "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime": true}, true)
	imports(migrateFile, map[string]bool{"context": true, "github.com/jackc/pgx/v5/pgxpool": true, "github.com/riverqueue/river/riverdriver/riverpgxv5": true, "github.com/riverqueue/river/rivermigrate": true}, true)
	imports(testFile, map[string]bool{"context": true, "errors": true, "testing": true, "time": true}, false)
	declarations(runtimeFile, map[string]bool{"Runtime": true, "NewRuntime": true, "Run": true, "stop": true}, false)
	declarations(migrateFile, map[string]bool{"invalidDirectionError": true, "Error": true, "Unwrap": true, "Migrate": true}, false)
	declarations(testFile, map[string]bool{}, true)
	checkRuntime(runtimeFile)
	checkMigrate(migrateFile)
	println("p0-s04-source: PASS")
}
