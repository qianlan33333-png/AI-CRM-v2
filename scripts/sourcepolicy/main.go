package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	sqlPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?is)(?:^|[;\r\n])[ \t]*select[ \t]+(?:[^;]*?\b(?:from|where|join|union|intersect|except|for[ \t]+(?:update|share))\b|(?:[0-9]+|\$[0-9]+|true|false|null|[a-z_][a-z0-9_]*[ \t]*\([^;]*\))[ \t]*(?:;|$))`),
		regexp.MustCompile(`(?im)(?:^|[;\r\n])[ \t]*insert[ \t]+into[ \t]+[a-z0-9_.$"]+`),
		regexp.MustCompile(`(?is)(?:^|[;\r\n])[ \t]*update[ \t]+(?:only[ \t]+)?[a-z0-9_.$"]+[^;]*?\bset\b`),
		regexp.MustCompile(`(?im)(?:^|[;\r\n])[ \t]*delete[ \t]+from[ \t]+(?:only[ \t]+)?[a-z0-9_.$"]+`),
		regexp.MustCompile(`(?im)(?:^|[;\r\n])[ \t]*merge[ \t]+into[ \t]+[a-z0-9_.$"]+`),
		regexp.MustCompile(`(?is)(?:^|[;\r\n])[ \t]*copy[ \t]+[a-z0-9_.$"]+(?:[ \t]*\([^;]*?\))?[ \t\r\n]+from\b`),
		regexp.MustCompile(`(?im)(?:^|[;\r\n])[ \t]*truncate[ \t]+(?:table[ \t]+)?(?:only[ \t]+)?[a-z0-9_.$"]+`),
	}
	ctePattern              = regexp.MustCompile(`(?is)(?:^|[;\r\n])[ \t]*with(?:[ \t\r\n]+recursive)?[ \t\r\n]+(?:"(?:[^"]|"")*"|[a-z_][a-z0-9_$]*)[ \t\r\n]*(?:\([^;]*?\))?[ \t\r\n]+as\b[ \t\r\n]*(?:(?:not[ \t\r\n]+)?materialized[ \t\r\n]*)?\(`)
	statementKeywordPattern = regexp.MustCompile(`(?i)\b(?:select|insert|update|delete|merge|copy|truncate|with)\b`)
	cronPattern             = regexp.MustCompile(`(^|/)cron(/|$)`)
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if flag.NArg() != 0 {
		fatalf("unexpected arguments")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}
	for _, tree := range []string{"cmd", "internal"} {
		if err := walkTree(absRoot, tree); err != nil {
			fatalf("%v", err)
		}
	}
	fmt.Println("source-policy-lint: PASS")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "source-policy-lint: "+format+"\n", args...)
	os.Exit(1)
}

func walkTree(root, tree string) error {
	treeRoot := filepath.Join(root, tree)
	info, err := os.Lstat(treeRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("regular directory required: %s", tree)
	}
	return filepath.WalkDir(treeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("symlink or special path forbidden: %s", rel)
		}
		if info.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".sql":
			if !allowedSQLPath(rel) {
				return fmt.Errorf("SQL source outside store/queries: %s", rel)
			}
		case ".go":
			if !generatedGoPath(rel) {
				return checkGo(path, rel)
			}
		}
		return nil
	})
}

func generatedGoPath(rel string) bool {
	parts := strings.Split(rel, "/")
	return len(parts) >= 3 && parts[0] == "internal" && parts[len(parts)-2] == "generated" && strings.HasSuffix(parts[len(parts)-1], ".go")
}

func allowedSQLPath(rel string) bool {
	parts := strings.Split(rel, "/")
	return len(parts) == 5 && parts[0] == "internal" && parts[2] == "store" && parts[3] == "queries" && strings.HasSuffix(parts[4], ".sql")
}

func checkGo(path, rel string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	module := sourceModule(rel)
	allowDirectDatabase := performanceAcceptanceCommand(rel)
	allowedCustomerQuerySelectors := customerQueryPlanSelectors(file, rel)
	allowedMemberGridQuerySelectors := memberGridHTTPQuerySelectors(file, rel)
	allowedMemberGridServiceTestQuerySelectors := memberGridServiceTestQuerySelectors(file, rel)
	aliases := map[string]string{}
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return err
		}
		name := filepath.Base(value)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if (value == "os" || value == "syscall" || value == "time") && name == "." {
			return fmt.Errorf("dot import of source-policy package forbidden in %s: %s", rel, value)
		}
		aliases[name] = value
		lower := strings.ToLower(value)
		if cronPattern.MatchString(lower) {
			return fmt.Errorf("third-party cron forbidden in %s: %s", rel, value)
		}
		if module != "config" && (strings.Contains(lower, "envconfig") || strings.Contains(lower, "godotenv") || strings.Contains(lower, "viper")) {
			return fmt.Errorf("environment loader forbidden outside config in %s: %s", rel, value)
		}
		if strings.Contains(lower, "gorm.io/gorm") || strings.Contains(lower, "jmoiron/sqlx") || strings.Contains(lower, "masterminds/squirrel") || strings.Contains(lower, "doug-martin/goqu") {
			return fmt.Errorf("dynamic SQL library forbidden in %s: %s", rel, value)
		}
	}
	var result error
	ast.Inspect(file, func(node ast.Node) bool {
		if result != nil {
			return false
		}
		switch item := node.(type) {
		case *ast.BasicLit:
			if item.Kind == token.STRING {
				if value, err := strconv.Unquote(item.Value); err == nil && containsHandwrittenSQL(value) && !allowDirectDatabase {
					result = fmt.Errorf("handwritten SQL forbidden in %s", rel)
				}
			}
		case *ast.BinaryExpr:
			if value, ok := constantString(item); ok && containsHandwrittenSQL(value) && !allowDirectDatabase {
				result = fmt.Errorf("constructed SQL forbidden in %s", rel)
			}
		case *ast.SelectorExpr:
			result = checkSelector(item, aliases, module, rel, allowDirectDatabase,
				allowedCustomerQuerySelectors[item.Pos()] || allowedMemberGridQuerySelectors[item.Pos()] ||
					allowedMemberGridServiceTestQuerySelectors[item.Pos()])
		}
		return result == nil
	})
	return result
}

func containsHandwrittenSQL(value string) bool {
	value = trimLeadingSQLComments(value)
	if ctePattern.MatchString(value) {
		return true
	}
	for _, pattern := range sqlPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func trimLeadingSQLComments(value string) string {
	value = strings.TrimSpace(value)
	for {
		switch {
		case strings.HasPrefix(value, "--"):
			end := strings.IndexAny(value, "\r\n")
			comment := value[2:]
			if end >= 0 {
				comment = value[2:end]
			}
			if start := statementKeywordPattern.FindStringIndex(comment); start != nil {
				return strings.TrimSpace(comment[start[0]:])
			}
			if end < 0 {
				return ""
			}
			value = strings.TrimSpace(value[end+1:])
		case strings.HasPrefix(value, "/*"):
			end := strings.Index(value[2:], "*/")
			if end < 0 {
				return ""
			}
			value = strings.TrimSpace(value[end+4:])
		default:
			return value
		}
	}
}

func performanceAcceptanceCommand(rel string) bool {
	return rel == "cmd/aicrm-contact-perf-data/main.go" || rel == "cmd/aicrm-contact-perf/main.go"
}

func sourceModule(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) >= 2 && parts[0] == "internal" {
		return parts[1]
	}
	return ""
}

func constantString(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(value.Value)
		return text, err == nil
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := constantString(value.X)
		right, rightOK := constantString(value.Y)
		return left + right, leftOK && rightOK
	case *ast.ParenExpr:
		return constantString(value.X)
	}
	return "", false
}

func checkSelector(selector *ast.SelectorExpr, aliases map[string]string, module, rel string, allowDirectDatabase, allowCustomerQueryPlan bool) error {
	if receiver, ok := selector.X.(*ast.Ident); ok {
		path := aliases[receiver.Name]
		if (path == "os" || path == "syscall") && map[string]bool{"Getenv": true, "LookupEnv": true, "Environ": true, "ExpandEnv": true}[selector.Sel.Name] && module != "config" {
			return fmt.Errorf("environment read forbidden outside config in %s: %s", rel, selector.Sel.Name)
		}
		if path == "time" && map[string]bool{"NewTicker": true, "Tick": true, "AfterFunc": true}[selector.Sel.Name] {
			return fmt.Errorf("business timer forbidden in %s: time.%s", rel, selector.Sel.Name)
		}
	}
	if receiver, ok := selector.X.(*ast.SelectorExpr); ok && receiver.Sel.Name == "URL" && selector.Sel.Name == "Query" {
		return nil
	}
	if map[string]bool{"Exec": true, "ExecContext": true, "Query": true, "QueryContext": true, "QueryRow": true, "QueryRowContext": true, "Prepare": true, "PrepareContext": true, "CopyFrom": true, "SendBatch": true}[selector.Sel.Name] {
		if allowDirectDatabase || allowCustomerQueryPlan {
			return nil
		}
		return fmt.Errorf("direct database call forbidden in %s: %s", rel, selector.Sel.Name)
	}
	return nil
}

func customerQueryPlanSelectors(file *ast.File, rel string) map[token.Pos]bool {
	result := map[token.Pos]bool{}
	if rel != "internal/contact/store/customer_query_repository.go" {
		return result
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || len(function.Recv.List) != 1 || function.Name == nil ||
			(function.Name.Name != "Query" && function.Name.Name != "QueryRow") {
			continue
		}
		receiver := function.Recv.List[0]
		receiverType, ok := receiver.Type.(*ast.Ident)
		if !ok || receiverType.Name != "customerQueryDBTX" || len(receiver.Names) != 1 || receiver.Names[0].Name != "db" {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != function.Name.Name {
				return true
			}
			embedded, ok := selector.X.(*ast.SelectorExpr)
			if !ok || embedded.Sel.Name != "Tx" {
				return true
			}
			identifier, ok := embedded.X.(*ast.Ident)
			if ok && identifier.Obj != nil && identifier.Obj == receiver.Names[0].Obj {
				result[selector.Pos()] = true
			}
			return true
		})
	}
	return result
}

func memberGridHTTPQuerySelectors(file *ast.File, rel string) map[token.Pos]bool {
	result := map[token.Pos]bool{}
	if rel != "internal/product/membergrid/http.go" {
		return result
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || len(function.Recv.List) != 1 || function.Name == nil {
			continue
		}
		receiver := function.Recv.List[0]
		if len(receiver.Names) != 1 {
			continue
		}
		receiverType := receiver.Type
		if pointer, ok := receiverType.(*ast.StarExpr); ok {
			receiverType = pointer.X
		}
		typeName, ok := receiverType.(*ast.Ident)
		if !ok {
			continue
		}
		field := ""
		switch {
		case typeName.Name == "Handler" && function.Name.Name == "Query":
			field = "application"
		case typeName.Name == "routeFragment" && function.Name.Name == "ServeHTTP":
			field = "handler"
		default:
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Query" {
				return true
			}
			embedded, ok := selector.X.(*ast.SelectorExpr)
			if !ok || embedded.Sel.Name != field {
				return true
			}
			identifier, ok := embedded.X.(*ast.Ident)
			if ok && identifier.Obj != nil && identifier.Obj == receiver.Names[0].Obj {
				result[selector.Pos()] = true
			}
			return true
		})
	}
	return result
}

func memberGridServiceTestQuerySelectors(file *ast.File, rel string) map[token.Pos]bool {
	result := map[token.Pos]bool{}
	if rel != "internal/product/membergrid/service_test.go" {
		return result
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Query" {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok || identifier.Obj == nil {
			return true
		}
		assignment, ok := identifier.Obj.Decl.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, expression := range assignment.Rhs {
			call, ok := expression.(*ast.CallExpr)
			if !ok {
				continue
			}
			constructor, ok := call.Fun.(*ast.Ident)
			if ok && constructor.Name == "newTestService" {
				result[selector.Pos()] = true
				break
			}
		}
		return true
	})
	return result
}
