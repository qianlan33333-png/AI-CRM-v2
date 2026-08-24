package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	baselineRulePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]*=[1-9][0-9]*(?:,[a-z][a-z0-9_]*=[1-9][0-9]*)*$`)
	baselineDigestPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const (
	baselineHeader = "# source-policy-baseline-v1: path<TAB>sorted_rule_counts<TAB>ordered_syntax_sha256"
	baselinePath   = "scripts/sourcepolicy/baseline.tsv"
)

type finding struct {
	position token.Pos
	rule     string
	context  string
	syntax   string
	message  string
}

type baselineEntry struct {
	path        string
	rules       string
	fingerprint string
	message     string
}

func (entry baselineEntry) line() string {
	return strings.Join([]string{entry.path, entry.rules, entry.fingerprint}, "\t")
}

func main() {
	root := flag.String("root", ".", "repository root")
	printBaseline := flag.Bool("print-baseline", false, "print the exact current debt inventory without changing files")
	flag.Parse()
	if flag.NArg() != 0 {
		fatalf("unexpected arguments")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}
	entries, err := scanRepository(absRoot)
	if err != nil {
		fatalf("%v", err)
	}
	if *printBaseline {
		fmt.Println(baselineHeader)
		for _, entry := range entries {
			fmt.Println(entry.line())
		}
		return
	}
	expected, err := readBaseline(absRoot)
	if err != nil {
		fatalf("%v", err)
	}
	if err = compareBaseline(expected, entries); err != nil {
		fatalf("%v", err)
	}
	fmt.Printf("source-policy-lint: PASS_WITH_BASELINE(%d)\n", len(expected))
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "source-policy-lint: "+format+"\n", args...)
	os.Exit(1)
}

func scanRepository(root string) ([]baselineEntry, error) {
	findings := make(map[string][]finding)
	for _, tree := range []string{"cmd", "internal"} {
		if err := walkTree(root, tree, findings); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(findings))
	for path := range findings {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	entries := make([]baselineEntry, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, makeBaselineEntry(path, findings[path]))
	}
	return entries, nil
}

func walkTree(root, tree string, findings map[string][]finding) error {
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
				contents, readErr := os.ReadFile(path)
				if readErr != nil {
					return fmt.Errorf("read %s: %w", rel, readErr)
				}
				findings[rel] = append(findings[rel], finding{
					rule: "sql_source_outside_store_queries", context: "file", syntax: string(contents),
					message: fmt.Sprintf("SQL source outside store/queries: %s", rel),
				})
			}
		case ".go":
			if !generatedGoPath(rel) {
				fileFindings, checkErr := checkGo(path, rel)
				if checkErr != nil {
					return checkErr
				}
				if len(fileFindings) > 0 {
					findings[rel] = append(findings[rel], fileFindings...)
				}
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

func checkGo(path, rel string) ([]finding, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	module := sourceModule(rel)
	allowDirectDatabase := performanceAcceptanceCommand(rel)
	allowedCustomerQuerySelectors := customerQueryPlanSelectors(file, rel)
	allowedMemberGridQuerySelectors := memberGridHTTPQuerySelectors(file, rel)
	allowedMemberGridServiceTestQuerySelectors := memberGridServiceTestQuerySelectors(file, rel)
	aliases := map[string]string{}
	result := make([]finding, 0)
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(value)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if (value == "os" || value == "syscall" || value == "time") && name == "." {
			result = append(result, findingAt(spec.Pos(), "dot_import", "import", name+":"+value,
				fmt.Sprintf("dot import of source-policy package forbidden in %s: %s", rel, value)))
		}
		aliases[name] = value
		lower := strings.ToLower(value)
		if cronPattern.MatchString(lower) {
			result = append(result, findingAt(spec.Pos(), "third_party_cron", "import", name+":"+value,
				fmt.Sprintf("third-party cron forbidden in %s: %s", rel, value)))
		}
		if module != "config" && (strings.Contains(lower, "envconfig") || strings.Contains(lower, "godotenv") || strings.Contains(lower, "viper")) {
			result = append(result, findingAt(spec.Pos(), "environment_loader", "import", name+":"+value,
				fmt.Sprintf("environment loader forbidden outside config in %s: %s", rel, value)))
		}
		if strings.Contains(lower, "gorm.io/gorm") || strings.Contains(lower, "jmoiron/sqlx") || strings.Contains(lower, "masterminds/squirrel") || strings.Contains(lower, "doug-martin/goqu") {
			result = append(result, findingAt(spec.Pos(), "dynamic_sql_library", "import", name+":"+value,
				fmt.Sprintf("dynamic SQL library forbidden in %s: %s", rel, value)))
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.BasicLit:
			if item.Kind == token.STRING {
				if value, err := strconv.Unquote(item.Value); err == nil && containsHandwrittenSQL(value) && !allowDirectDatabase {
					result = append(result, findingAt(item.Pos(), "handwritten_sql", syntaxContext(file, item.Pos()), value,
						fmt.Sprintf("handwritten SQL forbidden in %s", rel)))
				}
			}
		case *ast.BinaryExpr:
			if value, ok := constantString(item); ok && containsHandwrittenSQL(value) && !allowDirectDatabase {
				result = append(result, findingAt(item.Pos(), "constructed_sql", syntaxContext(file, item.Pos()), value,
					fmt.Sprintf("constructed SQL forbidden in %s", rel)))
				return false
			}
		case *ast.SelectorExpr:
			if found, ok := selectorFinding(fileSet, file, item, aliases, module, rel, allowDirectDatabase,
				allowedCustomerQuerySelectors[item.Pos()] || allowedMemberGridQuerySelectors[item.Pos()] ||
					allowedMemberGridServiceTestQuerySelectors[item.Pos()]); ok {
				result = append(result, found)
			}
		}
		return true
	})
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].position != result[right].position {
			return result[left].position < result[right].position
		}
		if result[left].rule != result[right].rule {
			return result[left].rule < result[right].rule
		}
		return result[left].syntax < result[right].syntax
	})
	return result, nil
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

func selectorFinding(fileSet *token.FileSet, file *ast.File, selector *ast.SelectorExpr, aliases map[string]string, module, rel string, allowDirectDatabase, allowCustomerQueryPlan bool) (finding, bool) {
	context := syntaxContext(file, selector.Pos())
	syntax := formattedNode(fileSet, selector)
	if receiver, ok := selector.X.(*ast.Ident); ok {
		path := aliases[receiver.Name]
		if (path == "os" || path == "syscall") && map[string]bool{"Getenv": true, "LookupEnv": true, "Environ": true, "ExpandEnv": true}[selector.Sel.Name] && module != "config" {
			return findingAt(selector.Pos(), "environment_read", context, syntax,
				fmt.Sprintf("environment read forbidden outside config in %s: %s", rel, selector.Sel.Name)), true
		}
		if path == "time" && map[string]bool{"NewTicker": true, "Tick": true, "AfterFunc": true}[selector.Sel.Name] {
			return findingAt(selector.Pos(), "business_timer", context, syntax,
				fmt.Sprintf("business timer forbidden in %s: time.%s", rel, selector.Sel.Name)), true
		}
	}
	if receiver, ok := selector.X.(*ast.SelectorExpr); ok && receiver.Sel.Name == "URL" && selector.Sel.Name == "Query" {
		return finding{}, false
	}
	if map[string]bool{"Exec": true, "ExecContext": true, "Query": true, "QueryContext": true, "QueryRow": true, "QueryRowContext": true, "Prepare": true, "PrepareContext": true, "CopyFrom": true, "SendBatch": true}[selector.Sel.Name] {
		if allowDirectDatabase || allowCustomerQueryPlan {
			return finding{}, false
		}
		return findingAt(selector.Pos(), "direct_database_call", context, syntax,
			fmt.Sprintf("direct database call forbidden in %s: %s", rel, selector.Sel.Name)), true
	}
	return finding{}, false
}

func findingAt(position token.Pos, rule, context, syntax, message string) finding {
	return finding{position: position, rule: rule, context: context, syntax: syntax, message: message}
}

func syntaxContext(file *ast.File, position token.Pos) string {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || position < function.Pos() || position > function.End() {
			continue
		}
		name := function.Name.Name
		if function.Recv == nil || len(function.Recv.List) != 1 {
			return "func:" + name
		}
		receiver := function.Recv.List[0].Type
		if pointer, ok := receiver.(*ast.StarExpr); ok {
			receiver = pointer.X
		}
		if identifier, ok := receiver.(*ast.Ident); ok {
			return "method:" + identifier.Name + "." + name
		}
		return "method:" + name
	}
	return "package"
}

func formattedNode(fileSet *token.FileSet, node any) string {
	var output bytes.Buffer
	if err := format.Node(&output, fileSet, node); err != nil {
		return fmt.Sprintf("%T", node)
	}
	return output.String()
}

func makeBaselineEntry(path string, findings []finding) baselineEntry {
	counts := make(map[string]int)
	signatures := make([]string, 0, len(findings))
	firstMessage := ""
	for _, item := range findings {
		counts[item.rule]++
		signatures = append(signatures, strings.Join([]string{item.rule, item.context, item.syntax}, "\x00"))
		if firstMessage == "" {
			firstMessage = item.message
		}
	}
	rules := make([]string, 0, len(counts))
	for rule, count := range counts {
		rules = append(rules, fmt.Sprintf("%s=%d", rule, count))
	}
	sort.Strings(rules)
	// One line represents one debt file. The digest binds every finding's rule,
	// enclosing declaration, normalized syntax and lexical order, so a new,
	// changed or moved finding cannot hide behind another debt in the same file.
	digest := sha256.Sum256([]byte(strings.Join(signatures, "\x1e")))
	return baselineEntry{
		path: path, rules: strings.Join(rules, ","), fingerprint: fmt.Sprintf("%x", digest[:]), message: firstMessage,
	}
}

func readBaseline(root string) ([]baselineEntry, error) {
	path := filepath.Join(root, filepath.FromSlash(baselinePath))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("baseline file required: %s", baselinePath)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("regular baseline file required: %s", baselinePath)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open baseline: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	previousPath := ""
	entries := make([]baselineEntry, 0)
	seen := make(map[string]bool)
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if lineNumber == 1 {
			if line != baselineHeader {
				return nil, fmt.Errorf("malformed baseline header")
			}
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || !validBaselinePath(parts[0]) || !validBaselineRules(parts[1]) || !baselineDigestPattern.MatchString(parts[2]) {
			return nil, fmt.Errorf("malformed baseline entry at line %d", lineNumber)
		}
		if seen[parts[0]] {
			return nil, fmt.Errorf("duplicate baseline entry for %s", parts[0])
		}
		if previousPath != "" && parts[0] < previousPath {
			return nil, fmt.Errorf("malformed baseline entry order at line %d", lineNumber)
		}
		seen[parts[0]] = true
		previousPath = parts[0]
		entries = append(entries, baselineEntry{path: parts[0], rules: parts[1], fingerprint: parts[2]})
	}
	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("read baseline: %w", err)
	}
	if lineNumber == 0 {
		return nil, fmt.Errorf("malformed baseline header")
	}
	return entries, nil
}

func validBaselinePath(path string) bool {
	if path == "" || strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path {
		return false
	}
	return (strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/")) &&
		(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sql"))
}

func validBaselineRules(value string) bool {
	if !baselineRulePattern.MatchString(value) {
		return false
	}
	previous := ""
	for _, item := range strings.Split(value, ",") {
		rule := strings.SplitN(item, "=", 2)[0]
		if previous != "" && rule <= previous {
			return false
		}
		previous = rule
	}
	return true
}

func compareBaseline(expected, actual []baselineEntry) error {
	expectedByPath := make(map[string]baselineEntry, len(expected))
	for _, entry := range expected {
		expectedByPath[entry.path] = entry
	}
	actualByPath := make(map[string]baselineEntry, len(actual))
	for _, entry := range actual {
		actualByPath[entry.path] = entry
		wanted, ok := expectedByPath[entry.path]
		if !ok {
			return fmt.Errorf("unexpected baseline debt: %s: %s", entry.path, entry.message)
		}
		if wanted.line() != entry.line() {
			return fmt.Errorf("unexpected baseline debt fingerprint for %s: %s; stale baseline debt fingerprint remains", entry.path, entry.message)
		}
	}
	for _, entry := range expected {
		if _, ok := actualByPath[entry.path]; !ok {
			return fmt.Errorf("stale baseline debt: %s", entry.path)
		}
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
