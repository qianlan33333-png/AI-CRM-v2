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
	sqlPattern  = regexp.MustCompile(`(?i)\b(select\s+|insert\s+into\s+|update\s+|delete\s+from\s+|merge\s+into\s+|copy\s+|truncate\s+)`)
	cronPattern = regexp.MustCompile(`(^|/)cron(/|$)`)
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
	return len(parts) == 4 && parts[0] == "internal" && parts[1] == "api" && parts[2] == "generated" || len(parts) == 5 && parts[0] == "internal" && parts[2] == "store" && parts[3] == "generated" && strings.HasSuffix(parts[4], ".go")
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
				if value, err := strconv.Unquote(item.Value); err == nil && sqlPattern.MatchString(value) {
					result = fmt.Errorf("handwritten SQL forbidden in %s", rel)
				}
			}
		case *ast.BinaryExpr:
			if value, ok := constantString(item); ok && sqlPattern.MatchString(value) {
				result = fmt.Errorf("constructed SQL forbidden in %s", rel)
			}
		case *ast.SelectorExpr:
			result = checkSelector(item, aliases, module, rel)
		}
		return result == nil
	})
	return result
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

func checkSelector(selector *ast.SelectorExpr, aliases map[string]string, module, rel string) error {
	if receiver, ok := selector.X.(*ast.Ident); ok {
		path := aliases[receiver.Name]
		if (path == "os" || path == "syscall") && map[string]bool{"Getenv": true, "LookupEnv": true, "Environ": true, "ExpandEnv": true}[selector.Sel.Name] && module != "config" {
			return fmt.Errorf("environment read forbidden outside config in %s: %s", rel, selector.Sel.Name)
		}
		if path == "time" && map[string]bool{"NewTicker": true, "Tick": true, "AfterFunc": true}[selector.Sel.Name] {
			return fmt.Errorf("business timer forbidden in %s: time.%s", rel, selector.Sel.Name)
		}
	}
	if map[string]bool{"Exec": true, "ExecContext": true, "Query": true, "QueryContext": true, "QueryRow": true, "QueryRowContext": true, "Prepare": true, "PrepareContext": true, "CopyFrom": true, "SendBatch": true}[selector.Sel.Name] {
		return fmt.Errorf("direct database call forbidden in %s: %s", rel, selector.Sel.Name)
	}
	return nil
}
